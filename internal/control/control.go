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

// Handler builds the control API over a snapshot function. The daemon supplies a
// function that reads the live kernel; tests supply a stub.
func Handler(snapshot func() State) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snapshot())
	})
	return mux
}
