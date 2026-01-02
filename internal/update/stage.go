package update

import (
	"os"
)

// ApplyStagedBinary replaces the running executable with a staged binary if present.
// It returns the path applied, or an empty string when no staged binary exists.
func ApplyStagedBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	staged := StagedBinaryPath(exe)
	if _, err := os.Stat(staged); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := os.Rename(staged, exe); err != nil {
		return "", err
	}
	return exe, nil
}

// StagedBinaryPath returns the staging path for a given executable.
func StagedBinaryPath(exe string) string {
	return exe + ".next"
}
