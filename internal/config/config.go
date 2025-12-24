package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
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
	if normalizeTeamMembers(&cfg) {
		changed = true
	}
	if normalizeTeamRequests(&cfg) {
		changed = true
	}
	if normalizeUIDs(&cfg) {
		changed = true
	}
	if normalizeScopes(&cfg) {
		changed = true
	}
	if dedupePersonalNetworks(&cfg) {
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

func normalizeTeamMembers(cfg *model.AppConfig) bool {
	changed := false
	for i := range cfg.Teams {
		t := &cfg.Teams[i]
		hasAdmin := false
		for mi := range t.Members {
			m := &t.Members[mi]
			role := strings.TrimSpace(m.Role)
			if role == "" {
				role = model.TeamRoleUser
				changed = true
			}
			if role != model.TeamRoleAdmin && role != model.TeamRoleUser {
				role = model.TeamRoleUser
				changed = true
			}
			m.Role = role
			if role == model.TeamRoleAdmin {
				hasAdmin = true
			}
		}
		if !hasAdmin && len(t.Members) > 0 {
			t.Members[0].Role = model.TeamRoleAdmin
			changed = true
		}
	}
	return changed
}

func normalizeTeamRequests(cfg *model.AppConfig) bool {
	changed := false
	for i := range cfg.Teams {
		t := &cfg.Teams[i]
		if len(t.Requests) == 0 {
			continue
		}

		byEmail := map[string]model.TeamJoinRequest{}
		for _, req := range t.Requests {
			email := strings.ToLower(strings.TrimSpace(req.Email))
			if email == "" {
				changed = true
				continue
			}
			req.Email = email
			req.Name = strings.TrimSpace(req.Name)
			if req.ID == "" {
				req.ID = model.NewID()
				changed = true
			}
			switch req.Status {
			case model.TeamJoinPending, model.TeamJoinApproved, model.TeamJoinDeclined:
			default:
				req.Status = model.TeamJoinPending
				changed = true
			}

			if existing, ok := byEmail[email]; ok {
				byEmail[email] = pickJoinRequest(existing, req)
			} else {
				byEmail[email] = req
			}
		}

		normalized := make([]model.TeamJoinRequest, 0, len(byEmail))
		for _, req := range byEmail {
			normalized = append(normalized, req)
		}
		sort.Slice(normalized, func(i, j int) bool {
			return normalized[i].Email < normalized[j].Email
		})

		if !reflect.DeepEqual(t.Requests, normalized) {
			t.Requests = normalized
			changed = true
		}
	}
	return changed
}

func pickJoinRequest(a, b model.TeamJoinRequest) model.TeamJoinRequest {
	aResolved := a.Status != model.TeamJoinPending
	bResolved := b.Status != model.TeamJoinPending
	if aResolved && !bResolved {
		return a
	}
	if bResolved && !aResolved {
		return b
	}

	at := joinRequestUpdatedAt(a)
	bt := joinRequestUpdatedAt(b)
	if bt > at {
		return b
	}
	if at > bt {
		return a
	}
	if a.Status == model.TeamJoinApproved && b.Status == model.TeamJoinDeclined {
		return a
	}
	if b.Status == model.TeamJoinApproved && a.Status == model.TeamJoinDeclined {
		return b
	}
	return a
}

func joinRequestUpdatedAt(r model.TeamJoinRequest) int64 {
	if r.Status != model.TeamJoinPending && r.ResolvedAt > 0 {
		return r.ResolvedAt
	}
	return r.RequestedAt
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

func dedupePersonalNetworks(cfg *model.AppConfig) bool {
	changed := false
	seen := map[string]struct{}{}
	keep := make([]model.Network, 0, len(cfg.Networks))

	for _, netw := range cfg.Networks {
		if netw.TeamID != "" || netw.Deleted {
			keep = append(keep, netw)
			continue
		}
		fp := networkFingerprint(netw)
		if _, ok := seen[fp]; ok {
			changed = true
			continue
		}
		seen[fp] = struct{}{}
		keep = append(keep, netw)
	}

	if changed {
		cfg.Networks = keep
	}
	return changed
}

func networkFingerprint(netw model.Network) string {
	hosts := make([]string, 0, len(netw.Hosts))
	for _, h := range netw.Hosts {
		if h.Deleted {
			continue
		}
		hosts = append(hosts, hostFingerprint(h))
	}
	sort.Strings(hosts)

	var b strings.Builder
	b.WriteString(strings.TrimSpace(netw.Name))
	b.WriteString("|")
	for _, h := range hosts {
		b.WriteString(h)
		b.WriteString(";")
	}
	return b.String()
}

func hostFingerprint(h model.Host) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(h.Name))
	b.WriteString("|")
	b.WriteString(h.Host)
	b.WriteString("|")
	b.WriteString(strconv.Itoa(h.Port))
	b.WriteString("|")
	b.WriteString(h.User)
	b.WriteString("|")
	b.WriteString(string(h.Driver))
	b.WriteString("|")
	b.WriteString(string(h.Auth.Method))
	b.WriteString("|")
	b.WriteString(h.Auth.KeyPath)
	b.WriteString("|")
	b.WriteString(h.Auth.Password)
	b.WriteString("|")
	b.WriteString(string(h.HostKey.Mode))
	b.WriteString("|")
	if h.Telecom != nil {
		b.WriteString(h.Telecom.Path)
		b.WriteString("|")
		b.WriteString(h.Telecom.Protocol)
		b.WriteString("|")
		b.WriteString(h.Telecom.Command)
		b.WriteString("|")
		if len(h.Telecom.Args) > 0 {
			b.WriteString(strings.Join(h.Telecom.Args, ","))
		}
		b.WriteString("|")
		b.WriteString(h.Telecom.WorkDir)
		b.WriteString("|")
		if len(h.Telecom.Env) > 0 {
			keys := make([]string, 0, len(h.Telecom.Env))
			for k := range h.Telecom.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString(k)
				b.WriteString("=")
				b.WriteString(h.Telecom.Env[k])
				b.WriteString(",")
			}
		}
	}
	b.WriteString("|")
	if h.SFTP != nil {
		b.WriteString("sftp:")
		if h.SFTP.Enabled {
			b.WriteString("1")
		} else {
			b.WriteString("0")
		}
		b.WriteString("|")
		b.WriteString(string(h.SFTP.Credentials))
		b.WriteString("|")
		b.WriteString(h.SFTP.User)
		b.WriteString("|")
		b.WriteString(h.SFTP.Password)
	}
	return b.String()
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
