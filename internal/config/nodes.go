package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type Node struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	AuthType string `json:"authType"` // password | key | agent
}

// -----------------------------
// Paths
// -----------------------------

func nodesPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	base := filepath.Join(dir, "pterminal")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}

	return filepath.Join(base, "nodes.json"), nil
}

// -----------------------------
// Load
// -----------------------------

func LoadNodes() ([]Node, error) {
	path, err := nodesPath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return []Node{}, nil // empty but valid
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if len(b) == 0 {
		return []Node{}, nil
	}

	var nodes []Node
	if err := json.Unmarshal(b, &nodes); err != nil {
		return nil, fmt.Errorf("invalid nodes.json: %w", err)
	}

	return nodes, nil
}

// -----------------------------
// Save (atomic)
// -----------------------------

func SaveNodes(nodes []Node) error {
	path, err := nodesPath()
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"

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

	if err := os.Rename(tmp, path); err != nil {
		return err
	}

	// fsync directory for durability
	dir := filepath.Dir(path)
	if df, err := os.Open(dir); err == nil {
		_ = syscall.Fsync(int(df.Fd()))
		df.Close()
	}

	return nil
}
