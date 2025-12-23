package teamrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Manifest struct {
	TeamID      string      `json:"teamId"`
	GeneratedAt int64       `json:"generatedAt"`
	Files       []FileEntry `json:"files"`
}

type FileEntry struct {
	Path    string `json:"path"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"modTime,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

func TeamDir(baseDir, teamID string) string {
	return filepath.Join(baseDir, "teams", teamID)
}

func ManifestPath(teamDir string) string {
	return filepath.Join(teamDir, ".pterminal", "manifest.json")
}

func EnsureTeamDir(baseDir, teamID string) (string, error) {
	teamDir := TeamDir(baseDir, teamID)
	if err := os.MkdirAll(teamDir, 0o700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(teamDir, ".pterminal"), 0o700); err != nil {
		return "", err
	}
	return teamDir, nil
}

func LoadManifest(teamDir string) (Manifest, error) {
	path := ManifestPath(teamDir)
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func WriteManifest(teamDir string, m Manifest) error {
	path := ManifestPath(teamDir)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func BuildManifest(teamDir, teamID string) (Manifest, error) {
	prev, _ := LoadManifest(teamDir)
	prevMap := map[string]FileEntry{}
	for _, f := range prev.Files {
		prevMap[f.Path] = f
	}

	entries := map[string]FileEntry{}

	root := filepath.Clean(teamDir)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if filepath.Base(path) == ".pterminal" {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = normalizePath(rel)
		hash, err := hashFile(path)
		if err != nil {
			return nil
		}
		entries[rel] = FileEntry{
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
			Hash:    hash,
		}
		return nil
	})

	now := time.Now().Unix()
	for path, prevEntry := range prevMap {
		if _, ok := entries[path]; ok {
			continue
		}
		if prevEntry.Deleted {
			entries[path] = prevEntry
			continue
		}
		entries[path] = FileEntry{
			Path:    path,
			Size:    prevEntry.Size,
			ModTime: now,
			Hash:    prevEntry.Hash,
			Deleted: true,
		}
	}

	files := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry)
	}

	return Manifest{
		TeamID:      teamID,
		GeneratedAt: now,
		Files:       files,
	}, nil
}

func normalizePath(path string) string {
	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, string(filepath.Separator))
	return strings.ReplaceAll(path, string(filepath.Separator), "/")
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
