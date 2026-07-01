package cfg

import (
	"fmt"
	"os"
	"path/filepath"
)

// Loaded is a service cfg read from disk, keeping where it came from — the
// service folder is the unit ("a service is a folder"), so Dir is the root its
// scripts resolve against.
type Loaded struct {
	Service *Service
	Dir     string // the service folder (absolute or as given)
	Path    string // the cfg file itself
}

// LoadDir reads every service under root — one folder per service, each holding
// a cfg file (cfg.yml, cfg.yaml, or service.yml) — parsing and validating each
// against vocab. Folders without a cfg file are skipped (they may be scripts-only
// scaffolding); an invalid cfg fails the whole load, so a boot never half-loads.
func LoadDir(root string, vocab Vocabulary) ([]Loaded, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("cfg: read services dir: %w", err)
	}

	var out []Loaded
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		path, ok := findCfg(dir)
		if !ok {
			continue
		}
		data, err := os.ReadFile(path) // #nosec G304 -- operator-provided services dir
		if err != nil {
			return nil, fmt.Errorf("cfg: read %s: %w", path, err)
		}
		svc, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("cfg: %s: %w", path, err)
		}
		if svc.Name == "" {
			svc.Name = e.Name() // the folder names the service unless the cfg says otherwise
		}
		if err := svc.Validate(vocab); err != nil {
			return nil, fmt.Errorf("cfg: %s: %w", path, err)
		}
		out = append(out, Loaded{Service: svc, Dir: dir, Path: path})
	}
	return out, nil
}

// findCfg locates the service's cfg file in its folder, by convention.
func findCfg(dir string) (string, bool) {
	for _, name := range []string{"cfg.yml", "cfg.yaml", "service.yml", "service.yaml"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	return "", false
}
