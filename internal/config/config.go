package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ankouros/pterminal/internal/model"
)

const (
	ConfigDirName  = "pterminal"
	ConfigFileName = "pterminal.json"

	ConfigVersionCurrent = 2
)

var cfgMu sync.Mutex

// -----------------------------
// Defaults
// -----------------------------

func DefaultConfig() model.AppConfig {
	return model.AppConfig{
		Version: ConfigVersionCurrent,
		User: model.UserProfile{
			DeviceID: model.NewID(),
		},
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
						Scope:       model.ScopePrivate,
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
	cfgMu.Lock()
	defer cfgMu.Unlock()

	return ensureConfigLocked()
}

func Load() (model.AppConfig, error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	return loadLocked()
}

func ensureConfigLocked() (model.AppConfig, string, error) {
	p, err := ConfigPath()
	if err != nil {
		return model.AppConfig{}, "", err
	}

	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		cfg := DefaultConfig()
		if err := saveLocked(cfg); err != nil {
			return model.AppConfig{}, "", err
		}
		return cfg, p, nil
	}

	cfg, err := loadLocked()
	return cfg, p, err
}

func loadLocked() (model.AppConfig, error) {
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
		if err := saveLocked(cfg); err != nil {
			return model.AppConfig{}, err
		}
	}

	if cfg.Version == 1 {
		cfg.Version = ConfigVersionCurrent
		if err := saveLocked(cfg); err != nil {
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
	if normalizeUser(&cfg) {
		changed = true
	}
	if normalizeTeams(&cfg) {
		changed = true
	}
	if normalizeUIDs(&cfg) {
		changed = true
	}
	if normalizeScopes(&cfg) {
		changed = true
	}
	if changed {
		if err := saveLocked(cfg); err != nil {
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

func normalizeUser(cfg *model.AppConfig) bool {
	if cfg.User.DeviceID == "" {
		cfg.User.DeviceID = model.NewID()
		return true
	}
	return false
}

func normalizeTeams(cfg *model.AppConfig) bool {
	changed := false
	seen := make(map[string]struct{}, len(cfg.Teams))
	for i := range cfg.Teams {
		t := &cfg.Teams[i]
		if t.ID == "" || containsTeamID(seen, t.ID) {
			t.ID = model.NewID()
			changed = true
		}
		seen[t.ID] = struct{}{}
	}
	return changed
}

func containsTeamID(seen map[string]struct{}, id string) bool {
	_, ok := seen[id]
	return ok
}

func normalizeScopes(cfg *model.AppConfig) bool {
	changed := false
	for ni := range cfg.Networks {
		netw := &cfg.Networks[ni]
		teamFromHosts := ""
		for hi := range netw.Hosts {
			h := &netw.Hosts[hi]
			if h.Scope == "" {
				h.Scope = model.ScopePrivate
				changed = true
			}
			if h.Scope == model.ScopeTeam && h.TeamID == "" && netw.TeamID != "" {
				h.TeamID = netw.TeamID
				changed = true
			}
			if h.Scope == model.ScopePrivate && h.TeamID != "" {
				h.TeamID = ""
				changed = true
			}
			if h.Scope == model.ScopeTeam && h.TeamID != "" && teamFromHosts == "" {
				teamFromHosts = h.TeamID
			}
		}
		if netw.TeamID == "" && teamFromHosts != "" {
			netw.TeamID = teamFromHosts
			changed = true
		}
	}

	for i := range cfg.Scripts {
		s := &cfg.Scripts[i]
		if s.Scope == "" {
			s.Scope = model.ScopePrivate
			changed = true
		}
		if s.Scope == model.ScopePrivate && s.TeamID != "" {
			s.TeamID = ""
			changed = true
		}
	}
	return changed
}

func normalizeUIDs(cfg *model.AppConfig) bool {
	changed := false
	for ni := range cfg.Networks {
		netw := &cfg.Networks[ni]
		if netw.UID == "" {
			netw.UID = model.NewID()
			changed = true
		}
		for hi := range netw.Hosts {
			h := &netw.Hosts[hi]
			if h.UID == "" {
				h.UID = model.NewID()
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
	cfgMu.Lock()
	defer cfgMu.Unlock()

	return saveLocked(cfg)
}

func saveLocked(cfg model.AppConfig) error {
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
