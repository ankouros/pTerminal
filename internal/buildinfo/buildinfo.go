package buildinfo

import "fmt"

var (
	Version   = "dev"
	GitCommit = ""
	BuildTime = ""
)

// String returns a human-readable version string.
func String() string {
	if Version == "" {
		Version = "dev"
	}
	info := fmt.Sprintf("pTerminal %s", Version)
	if GitCommit != "" {
		info = fmt.Sprintf("%s (%s)", info, GitCommit)
	}
	if BuildTime != "" {
		info = fmt.Sprintf("%s built at %s", info, BuildTime)
	}
	return info
}
