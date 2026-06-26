// Package library is the catalog tool: persist and serve service definitions. A
// registry, not a runtime — it stores and serves; it never interprets, launches,
// or evaluates. It is consulted by the core (runtime) and the operator (CLI),
// not called as a workflow verb. This is the in-memory Catalog; a hard-saved
// canonical snapshot on disk follows.
package library

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
)

// Catalog stores service definitions and serves their parts. Safe for concurrent
// use. It does not parse or evaluate the definitions — that is the core's job.
type Catalog struct {
	mu   sync.RWMutex
	defs map[string]*cfg.Service
}

// New returns an empty catalog.
func New() *Catalog {
	return &Catalog{defs: make(map[string]*cfg.Service)}
}

// Register snapshots a service definition into the catalog (draft → live). It
// errors on a missing name or a duplicate registration; reindexing a changed
// service is deregister-then-register (the core performs the drain check first).
func (c *Catalog) Register(svc *cfg.Service) error {
	if svc == nil || svc.Name == "" {
		return fmt.Errorf("library: service has no name")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, dup := c.defs[svc.Name]; dup {
		return fmt.Errorf("library: service %q already registered", svc.Name)
	}
	c.defs[svc.Name] = svc
	return nil
}

// Deregister removes a service's entry. The core performs the drain check (no
// running instances) before calling this.
func (c *Catalog) Deregister(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.defs[name]; !ok {
		return fmt.Errorf("library: service %q not registered", name)
	}
	delete(c.defs, name)
	return nil
}

// GetDefinition returns the stored definition the core needs to launch.
func (c *Catalog) GetDefinition(name string) (*cfg.Service, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	svc, ok := c.defs[name]
	if !ok {
		return nil, fmt.Errorf("library: service %q not registered", name)
	}
	return svc, nil
}

// GetSuccessors returns the stored successor declarations (returned, never evaluated).
func (c *Catalog) GetSuccessors(name string) ([]cfg.Successor, error) {
	svc, err := c.GetDefinition(name)
	if err != nil {
		return nil, err
	}
	return svc.Successors, nil
}

// GetTriggers returns the stored external-trigger declarations.
func (c *Catalog) GetTriggers(name string) ([]cfg.Trigger, error) {
	svc, err := c.GetDefinition(name)
	if err != nil {
		return nil, err
	}
	return svc.Triggers, nil
}

// List enumerates the registered service names.
func (c *Catalog) List() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.defs))
	for name := range c.defs {
		names = append(names, name)
	}
	return names
}

// Diff reports whether current differs from the registered canonical snapshot
// (drift). It errors if the service is not registered.
func (c *Catalog) Diff(name string, current *cfg.Service) (bool, error) {
	stored, err := c.GetDefinition(name)
	if err != nil {
		return false, err
	}
	return !reflect.DeepEqual(stored, current), nil
}
