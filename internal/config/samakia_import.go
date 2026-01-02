package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ankouros/pterminal/internal/model"
)

type SamakiaImportSummary struct {
	Source       string                     `json:"source"`
	NetworkName  string                     `json:"networkName"`
	NetworkID    int                        `json:"networkId"`
	MatchMode    string                     `json:"matchMode"`
	Added        int                        `json:"added"`
	Updated      int                        `json:"updated"`
	Skipped      int                        `json:"skipped"`
	Removed      int                        `json:"removed"`
	RoleCounts   map[string]int             `json:"roleCounts"`
	AddedHosts   []SamakiaImportHostSummary `json:"addedHosts,omitempty"`
	UpdatedHosts []SamakiaImportHostSummary `json:"updatedHosts,omitempty"`
	RemovedHosts []SamakiaImportHostSummary `json:"removedHosts,omitempty"`
}

type inventoryHost struct {
	Name string
	Host string
	UID  string
	User string
	Port int
	Role model.HostRole
}

type SamakiaImportHostSummary struct {
	Name string         `json:"name"`
	Host string         `json:"host"`
	User string         `json:"user,omitempty"`
	Port int            `json:"port,omitempty"`
	Role model.HostRole `json:"role"`
}

const (
	samakiaManagedBy  = "samakia-import"
	matchModeHostname = "hostname"
	matchModeHost     = "host"
	matchModeUID      = "uid"
)

func ImportSamakiaInventory(
	cfg model.AppConfig,
	path string,
	networkName string,
	matchMode string,
) (model.AppConfig, SamakiaImportSummary, error) {
	if path == "" {
		return cfg, SamakiaImportSummary{}, errors.New("inventory path is empty")
	}
	if strings.TrimSpace(networkName) == "" {
		networkName = "Samakia Inventory"
	}
	normalizedMatchMode, err := normalizeMatchMode(matchMode)
	if err != nil {
		return cfg, SamakiaImportSummary{}, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, SamakiaImportSummary{}, err
	}

	var payload any
	if err := json.Unmarshal(b, &payload); err != nil {
		return cfg, SamakiaImportSummary{}, fmt.Errorf("invalid inventory JSON: %w", err)
	}

	hosts, source := parseSamakiaInventory(payload)
	if len(hosts) == 0 {
		return cfg, SamakiaImportSummary{}, errors.New("no hosts found in inventory")
	}
	if normalizedMatchMode == matchModeUID {
		for _, host := range hosts {
			if strings.TrimSpace(host.UID) == "" {
				return cfg, SamakiaImportSummary{}, errors.New("uid match mode requires uid (or id/vmid) for every host")
			}
		}
	}

	net := findOrCreateNetwork(&cfg, networkName)
	if net == nil {
		return cfg, SamakiaImportSummary{}, errors.New("failed to create network")
	}

	usedNames := map[string]struct{}{}
	for _, h := range net.Hosts {
		if h.Deleted {
			continue
		}
		if h.Name != "" {
			usedNames[h.Name] = struct{}{}
		}
	}

	summary := SamakiaImportSummary{
		Source:      source,
		NetworkName: net.Name,
		NetworkID:   net.ID,
		MatchMode:   normalizedMatchMode,
		RoleCounts:  map[string]int{},
	}

	seenKeys := map[string]struct{}{}
	for _, host := range hosts {
		if host.Host == "" {
			summary.Skipped++
			continue
		}

		if host.Port == 0 {
			host.Port = 22
		}
		if host.User == "" {
			host.User = "samakia"
		}
		if host.Role == "" {
			host.Role = model.HostRoleGeneric
		}

		key := importKey(normalizedMatchMode, host.Name, host.Host, host.UID, host.Role)
		if key == "" {
			summary.Skipped++
			continue
		}
		if _, ok := seenKeys[key]; ok {
			summary.Skipped++
			continue
		}
		seenKeys[key] = struct{}{}

		existing := findExistingSamakiaHost(net.Hosts, host, normalizedMatchMode)
		if existing != nil {
			changed := false
			if existing.Name == "" && host.Name != "" {
				existing.Name = host.Name
				changed = true
			}
			if existing.Host != host.Host {
				existing.Host = host.Host
				changed = true
			}
			if existing.User != host.User {
				existing.User = host.User
				changed = true
			}
			if existing.Port != host.Port {
				existing.Port = host.Port
				changed = true
			}
			if existing.Role != host.Role {
				existing.Role = host.Role
				changed = true
			}
			if host.UID != "" && existing.UID != host.UID {
				existing.UID = host.UID
				changed = true
			}
			if existing.Driver == "" {
				existing.Driver = model.DriverSSH
				changed = true
			}
			if existing.Auth.Method == "" {
				existing.Auth = model.AuthConfig{
					Method:   model.AuthKey,
					KeyPath:  "",
					Password: "",
				}
				changed = true
			}
			if existing.HostKey.Mode == "" {
				existing.HostKey.Mode = model.HostKeyKnownHosts
				changed = true
			}
			if existing.ManagedBy != samakiaManagedBy {
				existing.ManagedBy = samakiaManagedBy
				changed = true
			}
			if changed {
				summary.Updated++
				summary.UpdatedHosts = append(summary.UpdatedHosts, buildImportSummary(existing))
			} else {
				summary.Skipped++
			}
		} else {
			name := strings.TrimSpace(host.Name)
			if name == "" {
				name = host.Host
			}
			name = uniqueName(name, usedNames)
			usedNames[name] = struct{}{}

			net.Hosts = append(net.Hosts, model.Host{
				ID:        0,
				Name:      name,
				UID:       host.UID,
				Host:      host.Host,
				Port:      host.Port,
				User:      host.User,
				Role:      host.Role,
				ManagedBy: samakiaManagedBy,
				Driver:    model.DriverSSH,
				Auth: model.AuthConfig{
					Method:   model.AuthKey,
					KeyPath:  "",
					Password: "",
				},
				HostKey: model.HostKeyConfig{Mode: model.HostKeyKnownHosts},
				Scope:   model.ScopePrivate,
			})
			summary.Added++
			summary.AddedHosts = append(summary.AddedHosts, SamakiaImportHostSummary{
				Name: name,
				Host: host.Host,
				User: host.User,
				Port: host.Port,
				Role: host.Role,
			})
		}
		summary.RoleCounts[string(host.Role)]++
	}

	for i := range net.Hosts {
		h := &net.Hosts[i]
		if h.Deleted {
			continue
		}
		if h.Role != model.HostRoleFabric && h.Role != model.HostRolePlatform {
			continue
		}
		if h.ManagedBy != samakiaManagedBy {
			continue
		}
		key := importKey(normalizedMatchMode, h.Name, h.Host, h.UID, h.Role)
		if key == "" {
			continue
		}
		if _, ok := seenKeys[key]; ok {
			continue
		}
		h.Deleted = true
		summary.Removed++
		summary.RemovedHosts = append(summary.RemovedHosts, buildImportSummary(h))
	}

	_ = normalizeIDs(&cfg)
	_ = normalizeUIDs(&cfg)
	_ = normalizeScopes(&cfg)
	_ = normalizeTeams(&cfg)
	_ = normalizeTeamMembers(&cfg)
	_ = normalizeTeamRequests(&cfg)
	_ = normalizeUser(&cfg)
	_ = migrateSFTP(&cfg)
	_ = normalizeSFTP(&cfg)
	_ = normalizeTelecom(&cfg)
	_ = StripSecrets(&cfg)

	return cfg, summary, nil
}

func parseSamakiaInventory(payload any) ([]inventoryHost, string) {
	if payload == nil {
		return nil, ""
	}

	if list := parseHostList(payload); len(list) > 0 {
		return list, "host_list"
	}

	if terraform := parseTerraformInventory(payload); len(terraform) > 0 {
		return terraform, "terraform_output"
	}

	if ansible := parseAnsibleInventory(payload); len(ansible) > 0 {
		return ansible, "ansible_inventory"
	}

	return nil, ""
}

func parseHostList(payload any) []inventoryHost {
	slice := asSlice(payload)
	if len(slice) == 0 {
		if data := asMap(payload); data != nil {
			slice = asSlice(data["hosts"])
			if len(slice) == 0 {
				slice = asSlice(data["nodes"])
			}
		}
	}
	if len(slice) == 0 {
		return nil
	}

	data := asMap(payload)
	defaultRole := roleFromString(mapString(data, "role"))
	if defaultRole == "" {
		defaultRole = roleFromString(mapString(data, "source"))
	}
	if defaultRole == "" {
		defaultRole = model.HostRolePlatform
	}

	out := make([]inventoryHost, 0, len(slice))
	for _, item := range slice {
		entry := asMap(item)
		if entry == nil {
			continue
		}
		host := inventoryHostFromMap(entry, "", defaultRole)
		if host.Host == "" {
			continue
		}
		out = append(out, host)
	}
	return out
}

func parseTerraformInventory(payload any) []inventoryHost {
	data := asMap(payload)
	if data == nil {
		return nil
	}

	lxc := asMap(data["lxc_inventory"])
	if lxc == nil {
		return nil
	}
	if value := asMap(lxc["value"]); value != nil {
		lxc = value
	}
	if len(lxc) == 0 {
		return nil
	}

	out := make([]inventoryHost, 0, len(lxc))
	for key, raw := range lxc {
		entry := asMap(raw)
		if entry == nil {
			continue
		}
		host := inventoryHostFromMap(entry, key, model.HostRoleFabric)
		if host.Host == "" {
			continue
		}
		out = append(out, host)
	}
	return out
}

func parseAnsibleInventory(payload any) []inventoryHost {
	data := asMap(payload)
	if data == nil {
		return nil
	}

	meta := asMap(data["_meta"])
	hostvars := asMap(meta["hostvars"])
	if len(hostvars) == 0 {
		return nil
	}

	out := make([]inventoryHost, 0, len(hostvars))
	for hostName, raw := range hostvars {
		entry := asMap(raw)
		if entry == nil {
			continue
		}
		host := inventoryHostFromMap(entry, hostName, model.HostRoleFabric)
		if host.Host == "" {
			continue
		}
		out = append(out, host)
	}
	return out
}

func inventoryHostFromMap(entry map[string]any, fallbackName string, defaultRole model.HostRole) inventoryHost {
	name := firstNonEmpty(
		mapString(entry, "hostname"),
		mapString(entry, "name"),
		fallbackName,
	)
	addr := firstNonEmpty(
		mapString(entry, "ansible_host"),
		mapString(entry, "ip"),
		mapString(entry, "host"),
		mapString(entry, "address"),
	)
	if addr == "" {
		addr = name
	}

	user := firstNonEmpty(
		mapString(entry, "user"),
		mapString(entry, "ansible_user"),
	)
	port := mapInt(entry, "port")
	if port == 0 {
		port = mapInt(entry, "ansible_port")
	}

	role := roleFromString(mapString(entry, "role"))
	if role == "" {
		role = roleFromString(mapString(entry, "samakia_role"))
	}
	if role == "" {
		role = defaultRole
	}

	return inventoryHost{
		Name: name,
		Host: addr,
		UID: firstNonEmpty(
			mapStringAny(entry, "uid"),
			mapStringAny(entry, "id"),
			mapStringAny(entry, "vmid"),
		),
		User: user,
		Port: port,
		Role: role,
	}
}

func roleFromString(raw string) model.HostRole {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "fabric":
		return model.HostRoleFabric
	case "platform":
		return model.HostRolePlatform
	case "generic":
		return model.HostRoleGeneric
	default:
		return ""
	}
}

func findOrCreateNetwork(cfg *model.AppConfig, name string) *model.Network {
	for i := range cfg.Networks {
		net := &cfg.Networks[i]
		if net.Deleted {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(net.Name), strings.TrimSpace(name)) {
			return net
		}
	}
	cfg.Networks = append(cfg.Networks, model.Network{
		ID:    0,
		Name:  name,
		Hosts: []model.Host{},
	})
	return &cfg.Networks[len(cfg.Networks)-1]
}

func buildImportSummary(host *model.Host) SamakiaImportHostSummary {
	if host == nil {
		return SamakiaImportHostSummary{}
	}
	return SamakiaImportHostSummary{
		Name: host.Name,
		Host: host.Host,
		User: host.User,
		Port: host.Port,
		Role: host.Role,
	}
}

func importKey(matchMode, name, host, uid string, role model.HostRole) string {
	base := strings.TrimSpace(matchValue(matchMode, name, host, uid))
	if base == "" || role == "" {
		return ""
	}
	return strings.ToLower(string(role)) + "|" + strings.ToLower(base)
}

func findExistingSamakiaHost(hosts []model.Host, incoming inventoryHost, matchMode string) *model.Host {
	if incoming.Role == "" {
		return nil
	}
	match := strings.TrimSpace(matchValue(matchMode, incoming.Name, incoming.Host, incoming.UID))
	if match == "" {
		return nil
	}
	for i := range hosts {
		h := &hosts[i]
		if h.Deleted || h.Role != incoming.Role {
			continue
		}
		if h.ManagedBy != samakiaManagedBy {
			continue
		}
		candidate := strings.TrimSpace(matchValue(matchMode, h.Name, h.Host, h.UID))
		if candidate != "" && strings.EqualFold(candidate, match) {
			return h
		}
	}
	return nil
}

func matchValue(matchMode, name, host, uid string) string {
	switch matchMode {
	case matchModeHost:
		return strings.TrimSpace(host)
	case matchModeUID:
		return strings.TrimSpace(uid)
	default:
		base := strings.TrimSpace(name)
		if base == "" {
			base = strings.TrimSpace(host)
		}
		return base
	}
}

func normalizeMatchMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "hostname", "name":
		return matchModeHostname, nil
	case "host", "address":
		return matchModeHost, nil
	case "uid", "id":
		return matchModeUID, nil
	default:
		return "", fmt.Errorf("unsupported match mode: %s", mode)
	}
}

func uniqueName(name string, used map[string]struct{}) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "host"
	}
	if _, ok := used[name]; !ok {
		return name
	}
	base := fmt.Sprintf("%s (imported)", name)
	if _, ok := used[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (imported %d)", name, i)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}

func asMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func asSlice(v any) []any {
	if v == nil {
		return nil
	}
	if list, ok := v.([]any); ok {
		return list
	}
	return nil
}

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func mapInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case json.Number:
		n, _ := val.Int64()
		return int(n)
	default:
		return 0
	}
}

func mapStringAny(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case json.Number:
		return strings.TrimSpace(val.String())
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
