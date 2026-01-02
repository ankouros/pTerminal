package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ankouros/pterminal/internal/model"
)

func TestImportSamakiaInventoryHostList(t *testing.T) {
	payload := map[string]any{
		"hosts": []any{
			map[string]any{
				"name": "web-1",
				"host": "10.0.0.1",
				"user": "root",
				"port": 2222,
				"role": "platform",
			},
		},
	}
	data, _ := json.Marshal(payload)

	cfg := model.AppConfig{Version: ConfigVersionCurrent}
	updated, summary, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Samakia Inventory", matchModeHostname)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Added != 1 || summary.Updated != 0 || summary.Removed != 0 || summary.Skipped != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(updated.Networks) != 1 || len(updated.Networks[0].Hosts) != 1 {
		t.Fatalf("expected one network and one host")
	}
	got := updated.Networks[0].Hosts[0]
	if got.Role != model.HostRolePlatform {
		t.Fatalf("expected platform role, got %q", got.Role)
	}
	if got.User != "root" || got.Port != 2222 || got.Host != "10.0.0.1" {
		t.Fatalf("unexpected host fields: %+v", got)
	}
	if got.Auth.Method != model.AuthKey {
		t.Fatalf("expected key auth, got %q", got.Auth.Method)
	}
	if got.HostKey.Mode != model.HostKeyKnownHosts {
		t.Fatalf("expected known_hosts mode, got %q", got.HostKey.Mode)
	}
	if got.ManagedBy == "" {
		t.Fatalf("expected managedBy set")
	}
}

func TestImportSamakiaInventoryTerraformOutput(t *testing.T) {
	payload := map[string]any{
		"lxc_inventory": map[string]any{
			"value": map[string]any{
				"node1": map[string]any{
					"hostname": "node1",
					"node":     "pve",
					"vmid":     101,
				},
			},
		},
	}
	data, _ := json.Marshal(payload)

	cfg := model.AppConfig{Version: ConfigVersionCurrent}
	updated, summary, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Fabric Inventory", matchModeHostname)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Added != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	got := updated.Networks[0].Hosts[0]
	if got.Role != model.HostRoleFabric {
		t.Fatalf("expected fabric role, got %q", got.Role)
	}
	if got.Host != "node1" || got.User != "samakia" || got.Port != 22 {
		t.Fatalf("unexpected host fields: %+v", got)
	}
	if got.ManagedBy == "" {
		t.Fatalf("expected managedBy set")
	}
}

func TestImportSamakiaInventoryIdempotentUpdateAndRemove(t *testing.T) {
	cfg := model.AppConfig{
		Version: ConfigVersionCurrent,
		Networks: []model.Network{
			{
				ID:   1,
				Name: "Samakia Inventory",
				Hosts: []model.Host{
					{
						ID:        1,
						Name:      "node1",
						Host:      "10.0.0.1",
						Port:      22,
						User:      "samakia",
						Role:      model.HostRoleFabric,
						ManagedBy: samakiaManagedBy,
					},
					{
						ID:        2,
						Name:      "node2",
						Host:      "10.0.0.2",
						Port:      22,
						User:      "samakia",
						Role:      model.HostRoleFabric,
						ManagedBy: samakiaManagedBy,
					},
				},
			},
		},
	}

	payload := map[string]any{
		"role": "fabric",
		"hosts": []any{
			map[string]any{
				"hostname":     "node1",
				"ansible_host": "10.0.0.10",
			},
			map[string]any{
				"hostname":     "node3",
				"ansible_host": "10.0.0.3",
			},
		},
	}
	data, _ := json.Marshal(payload)

	updated, summary, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Samakia Inventory", matchModeHostname)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Added != 1 || summary.Updated != 1 || summary.Removed != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	var node1 model.Host
	var node2 model.Host
	var node3 model.Host
	for _, host := range updated.Networks[0].Hosts {
		switch host.Name {
		case "node1":
			node1 = host
		case "node2":
			node2 = host
		case "node3":
			node3 = host
		}
	}

	if node1.Host != "10.0.0.10" {
		t.Fatalf("expected node1 host updated, got %q", node1.Host)
	}
	if !node2.Deleted {
		t.Fatalf("expected node2 deleted")
	}
	if node3.Host != "10.0.0.3" {
		t.Fatalf("expected node3 added")
	}
}

func TestImportSamakiaInventoryDoesNotRemoveUnmanaged(t *testing.T) {
	cfg := model.AppConfig{
		Version: ConfigVersionCurrent,
		Networks: []model.Network{
			{
				ID:   1,
				Name: "Samakia Inventory",
				Hosts: []model.Host{
					{
						ID:   1,
						Name: "manual",
						Host: "10.0.0.9",
						Port: 22,
						User: "samakia",
						Role: model.HostRoleFabric,
					},
				},
			},
		},
	}

	payload := map[string]any{
		"role": "fabric",
		"hosts": []any{
			map[string]any{
				"hostname":     "node1",
				"ansible_host": "10.0.0.1",
			},
		},
	}
	data, _ := json.Marshal(payload)

	updated, summary, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Samakia Inventory", matchModeHostname)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Removed != 0 {
		t.Fatalf("expected no removals, got %+v", summary)
	}

	foundManual := false
	for _, host := range updated.Networks[0].Hosts {
		if host.Name == "manual" {
			foundManual = true
			if host.Deleted {
				t.Fatalf("manual host should not be deleted")
			}
		}
	}
	if !foundManual {
		t.Fatalf("expected manual host present")
	}
}

func TestImportSamakiaInventoryDoesNotUpdateUnmanagedNameMatch(t *testing.T) {
	cfg := model.AppConfig{
		Version: ConfigVersionCurrent,
		Networks: []model.Network{
			{
				ID:   1,
				Name: "Samakia Inventory",
				Hosts: []model.Host{
					{
						ID:   1,
						Name: "node1",
						Host: "10.0.0.9",
						Port: 22,
						User: "samakia",
						Role: model.HostRoleFabric,
					},
				},
			},
		},
	}

	payload := map[string]any{
		"role": "fabric",
		"hosts": []any{
			map[string]any{
				"hostname":     "node1",
				"ansible_host": "10.0.0.1",
			},
		},
	}
	data, _ := json.Marshal(payload)

	updated, summary, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Samakia Inventory", matchModeHostname)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Added != 1 {
		t.Fatalf("expected added host, got %+v", summary)
	}

	manualHost := findHostByName(updated.Networks[0].Hosts, "node1")
	if manualHost == nil {
		t.Fatalf("expected manual host present")
	}
	if manualHost.Host != "10.0.0.9" {
		t.Fatalf("manual host should remain unchanged")
	}
	importedHost := findImportedHost(updated.Networks[0].Hosts, "node1")
	if importedHost == nil {
		t.Fatalf("expected imported host present")
	}
	if importedHost.Host != "10.0.0.1" {
		t.Fatalf("expected imported host address updated")
	}
	if importedHost.ManagedBy != samakiaManagedBy {
		t.Fatalf("expected managedBy set for imported host")
	}
}

func TestImportSamakiaInventoryMatchModeHost(t *testing.T) {
	cfg := model.AppConfig{
		Version: ConfigVersionCurrent,
		Networks: []model.Network{
			{
				ID:   1,
				Name: "Samakia Inventory",
				Hosts: []model.Host{
					{
						ID:        1,
						Name:      "node1",
						Host:      "10.0.0.1",
						Port:      22,
						User:      "samakia",
						Role:      model.HostRoleFabric,
						ManagedBy: samakiaManagedBy,
					},
				},
			},
		},
	}

	payload := map[string]any{
		"role": "fabric",
		"hosts": []any{
			map[string]any{
				"name": "renamed",
				"host": "10.0.0.1",
				"user": "root",
			},
		},
	}
	data, _ := json.Marshal(payload)

	updated, summary, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Samakia Inventory", matchModeHost)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Updated != 1 || summary.Added != 0 || summary.Removed != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(updated.Networks[0].Hosts) != 1 {
		t.Fatalf("expected one host after import")
	}
	got := updated.Networks[0].Hosts[0]
	if got.User != "root" {
		t.Fatalf("expected user updated, got %q", got.User)
	}
}

func TestImportSamakiaInventoryMatchModeUIDRequiresUID(t *testing.T) {
	payload := map[string]any{
		"role": "fabric",
		"hosts": []any{
			map[string]any{
				"name": "node1",
				"host": "10.0.0.1",
			},
		},
	}
	data, _ := json.Marshal(payload)

	cfg := model.AppConfig{Version: ConfigVersionCurrent}
	_, _, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Samakia Inventory", matchModeUID)
	if err == nil {
		t.Fatalf("expected uid match mode error")
	}
}

func TestImportSamakiaInventoryMatchModeUIDUpdatesManaged(t *testing.T) {
	cfg := model.AppConfig{
		Version: ConfigVersionCurrent,
		Networks: []model.Network{
			{
				ID:   1,
				Name: "Samakia Inventory",
				Hosts: []model.Host{
					{
						ID:        1,
						Name:      "node1",
						UID:       "node-1",
						Host:      "10.0.0.1",
						Port:      22,
						User:      "samakia",
						Role:      model.HostRoleFabric,
						ManagedBy: samakiaManagedBy,
					},
				},
			},
		},
	}

	payload := map[string]any{
		"role": "fabric",
		"hosts": []any{
			map[string]any{
				"name": "new-name",
				"host": "10.0.0.2",
				"uid":  "node-1",
				"user": "root",
			},
		},
	}
	data, _ := json.Marshal(payload)

	updated, summary, err := ImportSamakiaInventory(cfg, writeTempJSON(t, data), "Samakia Inventory", matchModeUID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Updated != 1 || summary.Added != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	got := updated.Networks[0].Hosts[0]
	if got.Host != "10.0.0.2" {
		t.Fatalf("expected host updated, got %q", got.Host)
	}
	if got.User != "root" {
		t.Fatalf("expected user updated, got %q", got.User)
	}
}

func findHostByName(hosts []model.Host, name string) *model.Host {
	for i := range hosts {
		if hosts[i].Name == name {
			return &hosts[i]
		}
	}
	return nil
}

func findImportedHost(hosts []model.Host, baseName string) *model.Host {
	for i := range hosts {
		h := &hosts[i]
		if h.ManagedBy != samakiaManagedBy {
			continue
		}
		if strings.HasPrefix(h.Name, baseName) {
			return h
		}
	}
	return nil
}

func writeTempJSON(t *testing.T, data []byte) string {
	t.Helper()
	f := t.TempDir() + "/inventory.json"
	if err := os.WriteFile(f, data, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return f
}
