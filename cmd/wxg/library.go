package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/control"
)

// newLibraryIndexCmd registers a service from its cfg folder with the live kernel
// and persists it to the admin index.
func newLibraryIndexCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "index <folder>",
		Short: "register a service from its cfg folder (draft → live)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := loadServiceFolder(args[0])
			if err != nil {
				return err
			}
			cfgJSON, err := json.Marshal(svc)
			if err != nil {
				return fmt.Errorf("marshal cfg: %w", err)
			}
			body, _ := json.Marshal(control.IndexRequest{CfgJSON: string(cfgJSON)})
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Post("http://"+addr+"/library/index", "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("cannot reach the daemon at %s — is `wuxing` running? (%w)", addr, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				var apiErr struct {
					Error string `json:"error"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&apiErr)
				if apiErr.Error == "" {
					apiErr.Error = resp.Status
				}
				return fmt.Errorf("daemon refused index: %s", apiErr.Error)
			}
			var entry control.CatalogEntry
			if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
			ew := &errWriter{w: cmd.OutOrStdout()}
			ew.printf("indexed: %s\n", entry.Name)
			return ew.err
		},
	}
	cmd.Flags().StringVar(&addr, "api", control.DefaultAddr, "daemon control API address")
	return cmd
}

// newLibraryDeindexCmd removes a service from the live kernel and admin index.
func newLibraryDeindexCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "deindex <service>",
		Short: "deregister a service (drain first)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, _ := json.Marshal(control.DeindexRequest{Service: args[0]})
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Post("http://"+addr+"/library/deindex", "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("cannot reach the daemon at %s — is `wuxing` running? (%w)", addr, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				var apiErr struct {
					Error string `json:"error"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&apiErr)
				if apiErr.Error == "" {
					apiErr.Error = resp.Status
				}
				return fmt.Errorf("daemon refused deindex: %s", apiErr.Error)
			}
			ew := &errWriter{w: cmd.OutOrStdout()}
			ew.printf("deindexed: %s\n", args[0])
			return ew.err
		},
	}
	cmd.Flags().StringVar(&addr, "api", control.DefaultAddr, "daemon control API address")
	return cmd
}

// loadServiceFolder reads a service from a cfg folder: looks for a cfg file,
// parses and validates it, then sets the name from the folder if the cfg omits it.
func loadServiceFolder(folder string) (*cfg.Service, error) {
	for _, name := range []string{"cfg.yml", "cfg.yaml", "service.yml", "service.yaml"} {
		p := filepath.Join(folder, name)
		data, err := os.ReadFile(p) // #nosec G304 -- operator-provided path
		if err != nil {
			continue
		}
		svc, err := cfg.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		if svc.Name == "" {
			svc.Name = filepath.Base(folder)
		}
		if err := svc.Validate(cfg.DefaultVocabulary()); err != nil {
			return nil, err
		}
		return svc, nil
	}
	return nil, fmt.Errorf("no cfg file (cfg.yml, cfg.yaml, service.yml, service.yaml) found in %s", folder)
}
