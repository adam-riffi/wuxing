// Package manifest loads the boot manifest — the source-level registry of tools
// the kernel loads at boot. It is one of wuxing's three registries (boot
// manifest · library index · running registry) and the only one edited in
// development rather than at runtime.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ToolStatus describes how complete a tool's implementation is. During the
// walking-skeleton build most tools are stubs; this lets the daemon boot the
// full manifest before every tool is real.
type ToolStatus string

// Tool implementation statuses, least to most complete.
const (
	StatusStub  ToolStatus = "stub"
	StatusAlpha ToolStatus = "alpha"
	StatusLive  ToolStatus = "live"
)

// Tool is a single entry in the boot manifest.
type Tool struct {
	Name    string     `yaml:"name"`
	Package string     `yaml:"package"`
	Status  ToolStatus `yaml:"status"`
}

// Manifest is the parsed boot manifest.
type Manifest struct {
	Version int    `yaml:"version"`
	Tools   []Tool `yaml:"tools"`
}

// Load reads and validates the boot manifest at path.
func Load(path string) (*Manifest, error) {
	// The manifest path is operator-supplied (a CLI flag / boot config), not
	// attacker-controlled input; reading it is the whole point of this function.
	raw, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("manifest: read %q: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse %q: %w", path, err)
	}

	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("manifest: %q: %w", path, err)
	}

	return &m, nil
}

func (m *Manifest) validate() error {
	if len(m.Tools) == 0 {
		return fmt.Errorf("no tools declared")
	}

	seen := make(map[string]struct{}, len(m.Tools))
	for i, t := range m.Tools {
		if t.Name == "" {
			return fmt.Errorf("tool %d: missing name", i)
		}
		if t.Package == "" {
			return fmt.Errorf("tool %q: missing package", t.Name)
		}
		if _, dup := seen[t.Name]; dup {
			return fmt.Errorf("tool %q: declared more than once", t.Name)
		}
		seen[t.Name] = struct{}{}
	}

	return nil
}
