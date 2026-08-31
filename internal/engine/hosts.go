package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const managedHostsMarker = "# govard-managed"

var hostsFilePath = "/etc/hosts"

// IsDomainResolvableLocally checks if the domain resolves to localhost (127.0.0.1 or ::1)
// with a default timeout of 15 seconds.
func IsDomainResolvableLocally(domain string) bool {
	return isDomainResolvableLocallyWithTimeout(domain, 15*time.Second)
}

// isDomainResolvableLocallyWithTimeout checks if the domain resolves to localhost (127.0.0.1 or ::1)
// within the specified timeout.
func isDomainResolvableLocallyWithTimeout(domain string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", domain)
	if err != nil {
		return false
	}

	for _, ip := range ips {
		if ip.IsLoopback() {
			return true
		}
	}
	return false
}

func AddHostsEntry(domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	lines, err := readHostsLines()
	if err != nil {
		return err
	}

	found := false
	updated := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		raw := line
		parsed, ok := parseHostsLine(raw)
		if !ok {
			updated = append(updated, raw)
			continue
		}

		if containsToken(parsed.Hosts, domain) {
			found = true
		}
		updated = append(updated, raw)
	}

	if found {
		return nil
	}

	updated = append(updated, "127.0.0.1 "+domain+" "+managedHostsMarker)
	return writeHostsLines(updated)
}

func RemoveHostsEntry(domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil
	}

	lines, err := readHostsLines()
	if err != nil {
		return err
	}

	changed := false
	updated := make([]string, 0, len(lines))
	for _, line := range lines {
		parsed, ok := parseHostsLine(line)
		if !ok {
			updated = append(updated, line)
			continue
		}

		filteredHosts := make([]string, 0, len(parsed.Hosts))
		for _, host := range parsed.Hosts {
			if host == domain {
				changed = true
				continue
			}
			filteredHosts = append(filteredHosts, host)
		}

		if len(filteredHosts) == 0 {
			changed = true
			continue
		}

		updated = append(updated, rebuildHostsLine(parsed.IP, filteredHosts, parsed.Comment))
	}

	if !changed {
		return nil
	}

	return writeHostsLines(updated)
}

type parsedHostsLine struct {
	IP      string
	Hosts   []string
	Comment string
}

func parseHostsLine(line string) (parsedHostsLine, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return parsedHostsLine{}, false
	}

	content := trimmed
	comment := ""
	if idx := strings.Index(content, "#"); idx >= 0 {
		comment = strings.TrimSpace(content[idx:])
		content = strings.TrimSpace(content[:idx])
	}

	fields := strings.Fields(content)
	if len(fields) < 2 {
		return parsedHostsLine{}, false
	}

	return parsedHostsLine{
		IP:      fields[0],
		Hosts:   fields[1:],
		Comment: comment,
	}, true
}

func rebuildHostsLine(ip string, hosts []string, comment string) string {
	line := strings.TrimSpace(ip + " " + strings.Join(hosts, " "))
	if strings.TrimSpace(comment) != "" {
		line += " " + strings.TrimSpace(comment)
	}
	return line
}

func containsToken(tokens []string, token string) bool {
	for _, item := range tokens {
		if item == token {
			return true
		}
	}
	return false
}

func readHostsLines() ([]string, error) {
	data, err := os.ReadFile(hostsFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read hosts file: %w", err)
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	return lines, nil
}

func writeHostsLines(lines []string) error {
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	dir := filepath.Dir(hostsFilePath)
	tempFile, err := os.CreateTemp(dir, "govard-hosts-*")
	if err != nil {
		return fmt.Errorf("failed to create temp hosts file: %w", err)
	}
	tempPath := tempFile.Name()
	if _, err := tempFile.WriteString(content); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to write temp hosts file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to close temp hosts file: %w", err)
	}

	if err := os.Rename(tempPath, hostsFilePath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to replace hosts file (try running with elevated permissions): %w", err)
	}
	return nil
}

func SetHostsFilePathForTest(path string) func() {
	previous := hostsFilePath
	hostsFilePath = path
	return func() {
		hostsFilePath = previous
	}
}
