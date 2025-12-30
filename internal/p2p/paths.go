package p2p

import (
	"errors"
	"path"
	"path/filepath"
	"strings"

	"github.com/ankouros/pterminal/internal/teamrepo"
)

func cleanTeamRelPath(relPath string) (string, error) {
	rel := strings.TrimSpace(relPath)
	if rel == "" {
		return "", errors.New("empty path")
	}
	if strings.Contains(rel, "\\") {
		return "", errors.New("invalid path")
	}
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", errors.New("invalid path")
	}
	return clean, nil
}

func (s *Service) teamFilePath(teamID, relPath string) (string, error) {
	teamDir, err := teamrepo.EnsureTeamDir(s.baseDir, teamID)
	if err != nil {
		return "", err
	}
	clean, err := cleanTeamRelPath(relPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(teamDir, filepath.FromSlash(clean)), nil
}
