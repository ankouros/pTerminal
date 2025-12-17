package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ankouros/pterminal/internal/model"
)

const (
	ConfigDirName  = "pterminal"
	ConfigFileName = "pterminal.json"

	ConfigVersionCurrent = 1
)

// -----------------------------
// Defaults
// -----------------------------

func DefaultConfig() model.AppConfig {
	return model.AppConfig{
		Version: ConfigVersionCurrent,
		Networks: []model.Network{
			{
				ID:   1,
				Name: "Default",
				Hosts: []model.Host{
					{
						ID:     1,
						Name:   "example",
						Host:   "192.168.11.90",
						Port:   22,
						User:   "root",
						Driver: model.DriverSSH,
						Auth: model.AuthConfig{
							Method: model.AuthPassword,
						},
						HostKey: model.HostKeyConfig{
							Mode: model.HostKeyKnownHosts,
						},
						SFTPEnabled: false, // placeholder
					},
				},
			},
		},
	}
}

// -----------------------------
// Paths
// -----------------------------

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", ConfigDirName, ConfigFileName), nil
}

func ensureDir() (string, error) {
	p, err := ConfigPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return p, nil
}

// -----------------------------
// Public API
// -----------------------------

func EnsureConfig() (model.AppConfig, string, error) {
	p, err := ConfigPath()
	if err != nil {
		return model.AppConfig{}, "", err
	}

	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		cfg := DefaultConfig()
		if err := Save(cfg); err != nil {
			return model.AppConfig{}, "", err
		}
		return cfg, p, nil
	}

	cfg, err := Load()
	return cfg, p, err
}

func Load() (model.AppConfig, error) {
	p, err := ConfigPath()
	if err != nil {
		return model.AppConfig{}, err
	}

	b, err := os.ReadFile(p)
	if err != nil {
		return model.AppConfig{}, err
	}

	var cfg model.AppConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return model.AppConfig{}, fmt.Errorf("invalid config JSON: %w", err)
	}

	// ---- migration / normalization ----
	if cfg.Version == 0 {
		cfg.Version = ConfigVersionCurrent
		if err := Save(cfg); err != nil {
			return model.AppConfig{}, err
		}
	}

	if cfg.Version != ConfigVersionCurrent {
		return model.AppConfig{}, fmt.Errorf(
			"unsupported config version %d (expected %d)",
			cfg.Version,
			ConfigVersionCurrent,
		)
	}

	changed := normalizeIDs(&cfg)
	if normalizeTelecom(&cfg) {
		changed = true
	}
	if migrateSFTP(&cfg) {
		changed = true
	}
	if normalizeSFTP(&cfg) {
		changed = true
	}
	if changed {
		if err := Save(cfg); err != nil {
			return model.AppConfig{}, err
		}
	}

	return cfg, nil
}

func normalizeTelecom(cfg *model.AppConfig) bool {
	changed := false
	for ni := range cfg.Networks {
		for hi := range cfg.Networks[ni].Hosts {
			h := &cfg.Networks[ni].Hosts[hi]

			// Driver migration: ioshell -> telecom
			if h.Driver == model.DriverIOShell {
				h.Driver = model.DriverTelecom
				changed = true
			}

			// Field migration: ioshell -> telecom
			if h.Telecom == nil && h.IOShell != nil {
				h.Telecom = h.IOShell
				h.IOShell = nil
				changed = true
			}
			if h.Telecom != nil && h.IOShell != nil {
				h.IOShell = nil
				changed = true
			}
		}
	}
	return changed
}

func normalizeIDs(cfg *model.AppConfig) bool {
	changed := false

	// Ensure network IDs are unique and non-zero.
	usedNet := make(map[int]struct{}, len(cfg.Networks))
	nextNet := 1
	for ni := range cfg.Networks {
		id := cfg.Networks[ni].ID
		for {
			if id <= 0 {
				id = nextNet
			}
			if _, ok := usedNet[id]; ok {
				id++
				continue
			}
			break
		}
		if cfg.Networks[ni].ID != id {
			cfg.Networks[ni].ID = id
			changed = true
		}
		usedNet[id] = struct{}{}
		for {
			nextNet++
			if _, ok := usedNet[nextNet]; !ok {
				break
			}
		}
	}

	// Ensure host IDs are unique globally across all networks and non-zero.
	usedHost := make(map[int]struct{})
	nextHost := 1
	for ni := range cfg.Networks {
		for hi := range cfg.Networks[ni].Hosts {
			id := cfg.Networks[ni].Hosts[hi].ID
			for {
				if id <= 0 {
					id = nextHost
				}
				if _, ok := usedHost[id]; ok {
					id++
					continue
				}
				break
			}
			if cfg.Networks[ni].Hosts[hi].ID != id {
				cfg.Networks[ni].Hosts[hi].ID = id
				changed = true
			}
			usedHost[id] = struct{}{}
			for {
				nextHost++
				if _, ok := usedHost[nextHost]; !ok {
					break
				}
			}
		}
	}

	return changed
}

func migrateSFTP(cfg *model.AppConfig) bool {
	changed := false
	for ni := range cfg.Networks {
		for hi := range cfg.Networks[ni].Hosts {
			h := &cfg.Networks[ni].Hosts[hi]
			if h.SFTP == nil && h.SFTPEnabled {
				h.SFTP = &model.SFTPConfig{
					Enabled:     true,
					Credentials: model.SFTPCredsConnection,
				}
				changed = true
			}
		}
	}
	return changed
}

func normalizeSFTP(cfg *model.AppConfig) bool {
	changed := false
	for ni := range cfg.Networks {
		for hi := range cfg.Networks[ni].Hosts {
			h := &cfg.Networks[ni].Hosts[hi]
			if h.SFTP == nil {
				if h.SFTPEnabled {
					// Prefer structured config; if legacy flag is set, migrate.
					h.SFTP = &model.SFTPConfig{
						Enabled:     true,
						Credentials: model.SFTPCredsConnection,
					}
					changed = true
				}
				continue
			}

			if !h.SFTP.Enabled {
				if h.SFTPEnabled {
					h.SFTPEnabled = false
					changed = true
				}
				h.SFTP = nil
				changed = true
				continue
			}

			if h.SFTP.Credentials == "" {
				h.SFTP.Credentials = model.SFTPCredsConnection
				changed = true
			}
			if h.SFTP.Credentials != model.SFTPCredsConnection && h.SFTP.Credentials != model.SFTPCredsCustom {
				h.SFTP.Credentials = model.SFTPCredsConnection
				changed = true
			}

			if h.SFTP.Credentials == model.SFTPCredsConnection {
				if h.SFTP.User != "" || h.SFTP.Password != "" {
					h.SFTP.User = ""
					h.SFTP.Password = ""
					changed = true
				}
			}

			if !h.SFTPEnabled {
				h.SFTPEnabled = true
				changed = true
			}
		}
	}
	return changed
}

// Save writes the config atomically (tmp + fsync + rename)
func Save(cfg model.AppConfig) error {
	p, err := ensureDir()
	if err != nil {
		return err
	}

	cfg.Version = ConfigVersionCurrent

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp := p + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}

	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmp, p); err != nil {
		return err
	}

	// fsync directory for durability
	dir := filepath.Dir(p)
	if df, err := os.Open(dir); err == nil {
		_ = syscall.Fsync(int(df.Fd()))
		df.Close()
	}

	return nil
}

// -----------------------------
// Export
// -----------------------------

// Export copies the config to ~/Downloads so users can easily find it.
func ExportToDownloads() (string, error) {
	cfg, _, err := EnsureConfig()
	if err != nil {
		return "", err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	downloads := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(downloads, 0o755)

	filename := "pterminal-config-" + time.Now().Format("20060102-150405") + ".json"
	out := filepath.Join(downloads, filename)

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(out, b, 0o600); err != nil {
		return "", err
	}

	return out, nil
}
