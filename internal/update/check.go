package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const latestReleaseURL = "https://api.github.com/repos/ankouros/pTerminal/releases/latest"

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

type Release struct {
	Tag         string  `json:"tag_name"`
	Body        string  `json:"body"`
	HTMLURL     string  `json:"html_url"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int    `json:"size"`
}

// Latest fetches the most recent GitHub release for pTerminal.
func Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pTerminal")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected GitHub response: %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	return &rel, nil
}
