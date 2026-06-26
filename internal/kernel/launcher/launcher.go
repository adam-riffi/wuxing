// Package launcher drives the container engine to run a service's image with its
// resource envelope: it provisions the scratch space, injects the service's
// environment, applies the memory limit, and tracks the running container.
//
// The engine is an interface — Docker in production, a fake in tests. The real
// Docker driver (github.com/docker/docker) and the in-container bus transport
// are wired at the kernel integration step; this package owns the launch logic.
//
// Scratch is per-run ephemeral churn: provisioned on Launch, swept on Stop.
package launcher

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// RunSpec is what the container engine needs to run one service container.
type RunSpec struct {
	Name        string
	Image       string
	Env         map[string]string
	MemoryLimit int64  // bytes; the scheduler's limit applied to the container
	ScratchDir  string // host dir mounted as the container's scratch space
}

// Engine drives a container runtime. Docker implements it in production; tests
// use a fake.
type Engine interface {
	Run(ctx context.Context, spec RunSpec) (containerID string, err error)
	Stop(ctx context.Context, containerID string) error
}

// ServiceSpec describes a service to launch.
type ServiceSpec struct {
	Name        string
	Image       string
	Env         map[string]string
	MemoryLimit int64
}

// Instance is a launched service container.
type Instance struct {
	Service     string
	ContainerID string
	ScratchDir  string
}

// Launcher provisions scratch, builds the run spec, and drives the engine. It is
// safe for concurrent use.
type Launcher struct {
	engine      Engine
	scratchRoot string

	mu      sync.Mutex
	running map[string]Instance // containerID -> instance
}

// New returns a Launcher that runs containers via engine and provisions scratch
// directories under scratchRoot (which must exist).
func New(engine Engine, scratchRoot string) *Launcher {
	return &Launcher{
		engine:      engine,
		scratchRoot: scratchRoot,
		running:     make(map[string]Instance),
	}
}

// Launch provisions a scratch directory, builds the run spec from the service's
// envelope, and runs the container. On engine failure the scratch is swept so
// nothing leaks.
func (l *Launcher) Launch(ctx context.Context, svc ServiceSpec) (Instance, error) {
	if svc.Image == "" {
		return Instance{}, fmt.Errorf("launcher: service %q has no image", svc.Name)
	}

	scratch, err := os.MkdirTemp(l.scratchRoot, "scratch-"+svc.Name+"-")
	if err != nil {
		return Instance{}, fmt.Errorf("launcher: provision scratch for %q: %w", svc.Name, err)
	}

	id, err := l.engine.Run(ctx, RunSpec{
		Name:        svc.Name,
		Image:       svc.Image,
		Env:         svc.Env,
		MemoryLimit: svc.MemoryLimit,
		ScratchDir:  scratch,
	})
	if err != nil {
		_ = os.RemoveAll(scratch)
		return Instance{}, fmt.Errorf("launcher: run %q: %w", svc.Name, err)
	}

	inst := Instance{Service: svc.Name, ContainerID: id, ScratchDir: scratch}
	l.mu.Lock()
	l.running[id] = inst
	l.mu.Unlock()
	return inst, nil
}

// Stop tears the container down and sweeps its scratch space (regardless of the
// engine's result — the play area dies each run).
func (l *Launcher) Stop(ctx context.Context, containerID string) error {
	l.mu.Lock()
	inst, ok := l.running[containerID]
	if ok {
		delete(l.running, containerID)
	}
	l.mu.Unlock()
	if !ok {
		return fmt.Errorf("launcher: no running container %q", containerID)
	}

	err := l.engine.Stop(ctx, containerID)
	_ = os.RemoveAll(inst.ScratchDir)
	return err
}

// Running returns a snapshot of the currently-running instances.
func (l *Launcher) Running() []Instance {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Instance, 0, len(l.running))
	for _, inst := range l.running {
		out = append(out, inst)
	}
	return out
}
