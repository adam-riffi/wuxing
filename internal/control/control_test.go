package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_State(t *testing.T) {
	want := State{
		Memory:      Resource{Capacity: 100, Used: 30, Free: 70},
		Window:      Resource{Capacity: 10, Used: 4, Free: 6},
		RunningJobs: []string{"run-a"},
		Queue:       []string{"run-b", "run-c"},
		Sessions:    []SessionInfo{{Session: "s1", Run: "run-a", Service: "mtg"}},
	}
	srv := httptest.NewServer(Handler(func() State { return want }))
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

func TestHandler_Healthz(t *testing.T) {
	srv := httptest.NewServer(Handler(func() State { return State{} }))
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
