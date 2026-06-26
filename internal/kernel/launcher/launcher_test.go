package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
)

type fakeEngine struct {
	mu      sync.Mutex
	runs    []RunSpec
	stopped []string
	nextID  int
	failRun bool
}

func (f *fakeEngine) Run(_ context.Context, spec RunSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRun {
		return "", errors.New("engine: run failed")
	}
	f.runs = append(f.runs, spec)
	f.nextID++
	return fmt.Sprintf("c%d", f.nextID), nil
}

func (f *fakeEngine) Stop(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, id)
	return nil
}

func TestLauncher_LaunchBuildsSpec(t *testing.T) {
	eng := &fakeEngine{}
	l := New(eng, t.TempDir())

	inst, err := l.Launch(context.Background(), ServiceSpec{
		Name:        "mtg",
		Image:       "wuxing/mtg:dev",
		Env:         map[string]string{"SCRYFALL_KEY": "x"},
		MemoryLimit: 256 << 20,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if len(eng.runs) != 1 {
		t.Fatalf("engine.Run called %d times want 1", len(eng.runs))
	}
	spec := eng.runs[0]
	if spec.Image != "wuxing/mtg:dev" {
		t.Errorf("image: got %q", spec.Image)
	}
	if spec.Env["SCRYFALL_KEY"] != "x" {
		t.Errorf("env not injected: %v", spec.Env)
	}
	if spec.MemoryLimit != 256<<20 {
		t.Errorf("memory limit: got %d", spec.MemoryLimit)
	}
	if spec.ScratchDir == "" {
		t.Error("scratch dir not provisioned")
	}
	if _, err := os.Stat(inst.ScratchDir); err != nil {
		t.Errorf("scratch dir does not exist: %v", err)
	}
	if inst.ContainerID != "c1" {
		t.Errorf("container id: got %q want c1", inst.ContainerID)
	}
}

func TestLauncher_StopSweepsScratch(t *testing.T) {
	eng := &fakeEngine{}
	l := New(eng, t.TempDir())

	inst, err := l.Launch(context.Background(), ServiceSpec{Name: "svc", Image: "img"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(inst.ScratchDir); err != nil {
		t.Fatalf("scratch should exist after launch: %v", err)
	}

	if err := l.Stop(context.Background(), inst.ContainerID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := os.Stat(inst.ScratchDir); !os.IsNotExist(err) {
		t.Error("scratch dir should be swept on Stop")
	}
	if len(eng.stopped) != 1 || eng.stopped[0] != inst.ContainerID {
		t.Errorf("engine.Stop not called for %q: %v", inst.ContainerID, eng.stopped)
	}
	if len(l.Running()) != 0 {
		t.Error("instance should be removed from running set")
	}
}

func TestLauncher_NoImage(t *testing.T) {
	l := New(&fakeEngine{}, t.TempDir())
	if _, err := l.Launch(context.Background(), ServiceSpec{Name: "x"}); err == nil {
		t.Error("expected error launching a service with no image")
	}
}

func TestLauncher_RunFailureSweepsScratch(t *testing.T) {
	eng := &fakeEngine{failRun: true}
	root := t.TempDir()
	l := New(eng, root)

	if _, err := l.Launch(context.Background(), ServiceSpec{Name: "svc", Image: "img"}); err == nil {
		t.Fatal("expected Launch to fail when the engine fails")
	}
	// No scratch directory should be left behind under the root.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("scratch leaked on engine failure: %v", entries)
	}
}

func TestLauncher_StopUnknown(t *testing.T) {
	l := New(&fakeEngine{}, t.TempDir())
	if err := l.Stop(context.Background(), "ghost"); err == nil {
		t.Error("expected error stopping an unknown container")
	}
}
