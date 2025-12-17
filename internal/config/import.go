package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ankouros/pterminal/internal/model"
)

// ImportFromFile loads a config JSON from an arbitrary path, normalizes it, and
// overwrites the application's active config file. It also writes a backup of
// the existing config (if present) next to the config file.
func ImportFromFile(path string) (cfg model.AppConfig, backupPath string, err error) {
	if path == "" {
		return model.AppConfig{}, "", errors.New("import path is empty")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return model.AppConfig{}, "", err
	}

	if err := json.Unmarshal(b, &cfg); err != nil {
		return model.AppConfig{}, "", fmt.Errorf("invalid config JSON: %w", err)
	}

	if cfg.Version == 0 {
		cfg.Version = ConfigVersionCurrent
	}
	if cfg.Version != ConfigVersionCurrent {
		return model.AppConfig{}, "", fmt.Errorf(
			"unsupported config version %d (expected %d)",
			cfg.Version,
			ConfigVersionCurrent,
		)
	}

	_ = normalizeIDs(&cfg)
	_ = normalizeTelecom(&cfg)
	_ = migrateSFTP(&cfg)
	_ = normalizeSFTP(&cfg)

	cfgPath, err := ensureDir()
	if err != nil {
		return model.AppConfig{}, "", err
	}

	// Best-effort backup of existing config (if it exists).
	if existing, readErr := os.ReadFile(cfgPath); readErr == nil && len(existing) > 0 {
		dir := filepath.Dir(cfgPath)
		backupPath = filepath.Join(
			dir,
			ConfigFileName+".bak-"+time.Now().Format("20060102-150405"),
		)
		_ = os.WriteFile(backupPath, existing, 0o600)
	}

	if err := Save(cfg); err != nil {
		return model.AppConfig{}, backupPath, err
	}

	return cfg, backupPath, nil
}
