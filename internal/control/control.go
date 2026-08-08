// Package control is the daemon's local control channel: a loopback HTTP/JSON
// API that exposes the kernel's live runtime state (resources, queue, running
// sessions) to the wxg CLI. It is the read side today; the write side (drive
// runs, manage the catalog) extends the same surface later.
//
// The wire types live here so both the daemon and wxg share them without wxg
// importing the kernel. The handler takes a snapshot function, not the kernel,
// so this package stays a leaf.
package control

import (
	"encoding/json"
	"io"
	"net/http"
)

// DefaultAddr is the loopback address the daemon serves on and wxg dials.
const DefaultAddr = "127.0.0.1:7777"

// State is the live runtime snapshot returned by GET /state.
type State struct {
	Memory      Resource      `json:"memory"`
	Window      Resource      `json:"ai_window"`
	RunningJobs []string      `json:"running_jobs"`
	Queue       []string      `json:"queue"`
	Sessions    []SessionInfo `json:"sessions"`
}

// Resource is a dual-resource pool: capacity, used, free (bytes / window units).
type Resource struct {
	Capacity int64 `json:"capacity"`
	Used     int64 `json:"used"`
	Free     int64 `json:"free"`
}

// SessionInfo is a running unit of work.
type SessionInfo struct {
	Session string `json:"session"`
	Run     string `json:"run"`
	Service string `json:"service"`
}

// RunRequest asks the daemon to fire a service now — a manual external trigger,
// opening a new sequence. The minimal form; the run-control parameters
// (targeting, inputs, forced sequences — docs/cli-run-control.md) extend it.
type RunRequest struct {
	Service string `json:"service"`
}

// RunStarted reports a fired run's lineage ids, so the caller can follow it
// (`wxg show <sequence>`). Firing is decoupled from completion: the run may
// still be queued or executing when this returns.
type RunStarted struct {
	Service  string `json:"service"`
	Sequence string `json:"sequence"`
	Run      string `json:"run"`
}

type apiError struct {
	Error string `json:"error"`
}

// IndexRequest asks the daemon to register a service from its marshalled cfg.
type IndexRequest struct {
	CfgJSON string `json:"cfg_json"`
}

// DeindexRequest asks the daemon to deregister a service by name.
type DeindexRequest struct {
	Service string `json:"service"`
}

// CatalogEntry is one row of the registered service index.
type CatalogEntry struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	RegisteredAt string `json:"registered_at"`
}

// CatalogFuncs supplies catalog read/write operations to the Handler. The
// daemon passes closures over the live kernel + admin index; tests pass stubs.
type CatalogFuncs struct {
	Index   func(cfgJSON string) (CatalogEntry, error)
	Deindex func(name string) error
	List    func() []CatalogEntry
}

// Handler builds the control API over a snapshot function (read side), a fire
// function (write side), and catalog management functions.
// The daemon supplies functions over the live kernel; tests supply stubs.
func Handler(snapshot func() State, fire func(RunRequest) (RunStarted, error), catalog CatalogFuncs) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot())
	})
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(apiError{Error: "POST only"})
			return
		}
		var req RunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(apiError{Error: "body must be JSON with a non-empty \"service\""})
			return
		}
		started, err := fire(req)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(apiError{Error: err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(started)
	})
	mux.HandleFunc("/library", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		entries := catalog.List()
		if entries == nil {
			entries = []CatalogEntry{}
		}
		_ = json.NewEncoder(w).Encode(entries)
	})
	mux.HandleFunc("/library/index", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(apiError{Error: "POST only"})
			return
		}
		var req IndexRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CfgJSON == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(apiError{Error: "body must be JSON with a non-empty \"cfg_json\""})
			return
		}
		entry, err := catalog.Index(req.CfgJSON)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(apiError{Error: err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(entry)
	})
	mux.HandleFunc("/library/deindex", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(apiError{Error: "POST only"})
			return
		}
		var req DeindexRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(apiError{Error: "body must be JSON with a non-empty \"service\""})
			return
		}
		if err := catalog.Deindex(req.Service); err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(apiError{Error: err.Error()})
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
