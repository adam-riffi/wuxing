package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// noFire is a stub write side for read-side tests.
func noFire(RunRequest) (RunStarted, error) { return RunStarted{}, errors.New("not under test") }

func TestHandler_State(t *testing.T) {
	want := State{
		Memory:      Resource{Capacity: 100, Used: 30, Free: 70},
		Window:      Resource{Capacity: 10, Used: 4, Free: 6},
		RunningJobs: []string{"run-a"},
		Queue:       []string{"run-b", "run-c"},
		Sessions:    []SessionInfo{{Session: "s1", Run: "run-a", Service: "mtg"}},
	}
	srv := httptest.NewServer(Handler(func() State { return want }, noFire))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/state")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %s", resp.Status)
	}

	var got State
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Memory.Used != 30 || got.Window.Free != 6 {
		t.Errorf("resources: %+v", got)
	}
	if len(got.Queue) != 2 || got.Queue[0] != "run-b" {
		t.Errorf("queue: %+v", got.Queue)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Service != "mtg" {
		t.Errorf("sessions: %+v", got.Sessions)
	}
}

func TestHandler_Run(t *testing.T) {
	fire := func(req RunRequest) (RunStarted, error) {
		if req.Service == "ghost" {
			return RunStarted{}, errors.New("library: unknown service")
		}
		return RunStarted{Service: req.Service, Sequence: "seq1", Run: "run1"}, nil
	}
	srv := httptest.NewServer(Handler(func() State { return State{} }, fire))
	defer srv.Close()

	// Happy path.
	resp, err := http.Post(srv.URL+"/run", "application/json", strings.NewReader(`{"service":"mtg"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %s", resp.Status)
	}
	var started RunStarted
	if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	if started.Service != "mtg" || started.Sequence != "seq1" || started.Run != "run1" {
		t.Errorf("started: %+v", started)
	}

	// Unknown service surfaces the error.
	resp2, err := http.Post(srv.URL+"/run", "application/json", strings.NewReader(`{"service":"ghost"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unknown service status: %s", resp2.Status)
	}

	// Bad body / empty service.
	resp3, err := http.Post(srv.URL+"/run", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("empty service status: %s", resp3.Status)
	}

	// GET is refused.
	resp4, err := http.Get(srv.URL + "/run")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp4.Body.Close() }()
	if resp4.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /run status: %s", resp4.Status)
	}
}

func TestHandler_Healthz(t *testing.T) {
	srv := httptest.NewServer(Handler(func() State { return State{} }, noFire))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz status: %s", resp.Status)
	}
}
