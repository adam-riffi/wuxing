package main

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adam-riffi/wuxing/internal/control"
)

var errUnknown = errors.New("library: unknown service \"ghost\"")

func TestStateCmd(t *testing.T) {
	state := control.State{
		Memory:      control.Resource{Capacity: 2 << 30, Used: 64 << 20, Free: (2 << 30) - (64 << 20)},
		Window:      control.Resource{Capacity: 100, Used: 10, Free: 90},
		RunningJobs: []string{"runAAAA1111"},
		Queue:       []string{"runBBBB2222"},
		Sessions:    []control.SessionInfo{{Session: "sessAAAA111", Run: "runAAAA1111", Service: "mtg"}},
	}
	srv := httptest.NewServer(control.Handler(func() control.State { return state }, func(control.RunRequest) (control.RunStarted, error) {
		return control.RunStarted{}, nil
	}))
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

func TestRunCmd(t *testing.T) {
	var gotService string
	srv := httptest.NewServer(control.Handler(
		func() control.State { return control.State{} },
		func(req control.RunRequest) (control.RunStarted, error) {
			gotService = req.Service
			return control.RunStarted{Service: req.Service, Sequence: "seqAAAA1111", Run: "runBBBB2222"}, nil
		}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	out := runCmd(t, "run", "mtg", "--api", addr)
	if gotService != "mtg" {
		t.Errorf("service not sent: %q", gotService)
	}
	for _, want := range []string{"fired mtg", "seqAAAA1111", "runBBBB2222", "wxg show seqAAAA1"} {
		if !strings.Contains(out, want) {
			t.Errorf("run output missing %q:\n%s", want, out)
		}
	}
}

func TestRunCmd_UnknownService(t *testing.T) {
	srv := httptest.NewServer(control.Handler(
		func() control.State { return control.State{} },
		func(control.RunRequest) (control.RunStarted, error) {
			return control.RunStarted{}, errUnknown
		}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	root := newRootCmd()
	root.SetArgs([]string{"run", "ghost", "--api", addr})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("expected a refusal error, got %v", err)
	}
}
