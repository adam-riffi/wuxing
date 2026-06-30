package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adam-riffi/wuxing/internal/control"
)

func TestStateCmd(t *testing.T) {
	state := control.State{
		Memory:      control.Resource{Capacity: 2 << 30, Used: 64 << 20, Free: (2 << 30) - (64 << 20)},
		Window:      control.Resource{Capacity: 100, Used: 10, Free: 90},
		RunningJobs: []string{"runAAAA1111"},
		Queue:       []string{"runBBBB2222"},
		Sessions:    []control.SessionInfo{{Session: "sessAAAA111", Run: "runAAAA1111", Service: "mtg"}},
	}
	srv := httptest.NewServer(control.Handler(func() control.State { return state }))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	out := runCmd(t, "state", "--api", addr)
	for _, want := range []string{"resources", "memory", "running jobs (1)", "runAAAA1", "queued jobs (1)", "runBBBB2", "service=mtg"} {
		if !strings.Contains(out, want) {
			t.Errorf("state output missing %q:\n%s", want, out)
		}
	}
}

func TestStateCmd_DaemonDown(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"state", "--api", "127.0.0.1:0"})
	if err := root.Execute(); err == nil {
		t.Error("expected an error when the daemon is unreachable")
	}
}
