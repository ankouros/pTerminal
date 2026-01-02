package config

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SamakiaImportReport struct {
	GeneratedAt string               `json:"generatedAt"`
	ImportPath  string               `json:"importPath,omitempty"`
	Summary     SamakiaImportSummary `json:"summary"`
}

func ExportSamakiaImportReport(summary SamakiaImportSummary, importPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	downloads := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(downloads, 0o755)

	report := SamakiaImportReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
		ImportPath:  importPath,
		Summary:     summary,
	}

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}

	filename := "pterminal-samakia-import-" + time.Now().Format("20060102-150405") + ".json"
	out := filepath.Join(downloads, filename)
	if err := os.WriteFile(out, b, 0o600); err != nil {
		return "", err
	}

	return out, nil
}

func ExportSamakiaImportReportCSV(summary SamakiaImportSummary, importPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	downloads := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(downloads, 0o755)

	filename := "pterminal-samakia-import-" + time.Now().Format("20060102-150405") + ".csv"
	out := filepath.Join(downloads, filename)

	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	if err := writer.Write([]string{"section", "metric", "name", "host", "user", "port", "role", "value"}); err != nil {
		return "", err
	}

	writeSummaryRow := func(metric, value string) error {
		return writer.Write([]string{"summary", metric, "", "", "", "", "", value})
	}

	if err := writeSummaryRow("network", summary.NetworkName); err != nil {
		return "", err
	}
	if err := writeSummaryRow("source", summary.Source); err != nil {
		return "", err
	}
	if err := writeSummaryRow("match_mode", summary.MatchMode); err != nil {
		return "", err
	}
	if err := writeSummaryRow("import_path", importPath); err != nil {
		return "", err
	}
	if err := writeSummaryRow("added", strconv.Itoa(summary.Added)); err != nil {
		return "", err
	}
	if err := writeSummaryRow("updated", strconv.Itoa(summary.Updated)); err != nil {
		return "", err
	}
	if err := writeSummaryRow("skipped", strconv.Itoa(summary.Skipped)); err != nil {
		return "", err
	}
	if err := writeSummaryRow("removed", strconv.Itoa(summary.Removed)); err != nil {
		return "", err
	}

	writeHostRows := func(metric string, hosts []SamakiaImportHostSummary) error {
		for _, host := range hosts {
			if err := writer.Write([]string{
				"host",
				metric,
				host.Name,
				host.Host,
				host.User,
				strconv.Itoa(host.Port),
				string(host.Role),
				"",
			}); err != nil {
				return err
			}
		}
		return nil
	}

	if err := writeHostRows("added", summary.AddedHosts); err != nil {
		return "", err
	}
	if err := writeHostRows("updated", summary.UpdatedHosts); err != nil {
		return "", err
	}
	if err := writeHostRows("removed", summary.RemovedHosts); err != nil {
		return "", err
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return out, nil
}

func ExportSamakiaImportReportMarkdown(summary SamakiaImportSummary, importPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	downloads := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(downloads, 0o755)

	filename := "pterminal-samakia-import-" + time.Now().Format("20060102-150405") + ".md"
	out := filepath.Join(downloads, filename)

	var b strings.Builder
	b.WriteString("# Samakia Import Report\n\n")
	b.WriteString("- Generated: ")
	b.WriteString(time.Now().Format(time.RFC3339))
	b.WriteString("\n")
	b.WriteString("- Network: ")
	b.WriteString(summary.NetworkName)
	b.WriteString("\n")
	if summary.MatchMode != "" {
		b.WriteString("- Match mode: ")
		b.WriteString(summary.MatchMode)
		b.WriteString("\n")
	}
	if summary.Source != "" {
		b.WriteString("- Source: ")
		b.WriteString(summary.Source)
		b.WriteString("\n")
	}
	if importPath != "" {
		b.WriteString("- Import path: ")
		b.WriteString(importPath)
		b.WriteString("\n")
	}
	b.WriteString("- Added: ")
	b.WriteString(strconv.Itoa(summary.Added))
	b.WriteString("\n")
	b.WriteString("- Updated: ")
	b.WriteString(strconv.Itoa(summary.Updated))
	b.WriteString("\n")
	b.WriteString("- Skipped: ")
	b.WriteString(strconv.Itoa(summary.Skipped))
	b.WriteString("\n")
	b.WriteString("- Removed: ")
	b.WriteString(strconv.Itoa(summary.Removed))
	b.WriteString("\n")
	if roles := formatRoleCounts(summary.RoleCounts); roles != "" {
		b.WriteString("- Roles: ")
		b.WriteString(roles)
		b.WriteString("\n")
	}

	appendHostSection := func(title string, hosts []SamakiaImportHostSummary) {
		if len(hosts) == 0 {
			return
		}
		b.WriteString("\n## ")
		b.WriteString(title)
		b.WriteString("\n")
		for _, host := range hosts {
			b.WriteString("- ")
			b.WriteString(formatImportEntry(host))
			b.WriteString("\n")
		}
	}

	appendHostSection("Added hosts", summary.AddedHosts)
	appendHostSection("Updated hosts", summary.UpdatedHosts)
	appendHostSection("Removed hosts", summary.RemovedHosts)

	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
		return "", err
	}

	return out, nil
}

func formatRoleCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for key, count := range counts {
		if count <= 0 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+": "+strconv.Itoa(counts[key]))
	}
	return strings.Join(parts, ", ")
}

func formatImportEntry(entry SamakiaImportHostSummary) string {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		name = strings.TrimSpace(entry.Host)
	}
	if name == "" {
		name = "unknown"
	}
	host := strings.TrimSpace(entry.Host)
	port := ""
	if entry.Port > 0 {
		port = ":" + strconv.Itoa(entry.Port)
	}
	user := strings.TrimSpace(entry.User)
	addr := ""
	if host != "" {
		if user != "" {
			addr = user + "@"
		}
		addr += host + port
	}
	role := strings.TrimSpace(string(entry.Role))
	if role != "" {
		role = " | " + role
	}
	if addr != "" {
		return name + " | " + addr + role
	}
	return name + role
}
