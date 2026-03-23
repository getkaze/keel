package cli

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getkaze/keel/internal/config"
)

const hostsMarkerBegin = "# BEGIN keel"
const hostsMarkerEnd = "# END keel"

// runHosts implements "keel hosts setup" and "keel hosts remove".
func runHosts(args []string, keelDir string) {
	if len(args) == 0 {
		fatalf("usage: keel hosts <setup|remove> [--ip <addr>]")
	}

	switch args[0] {
	case "setup":
		runHostsSetup(args[1:], keelDir)
	case "remove":
		runHostsRemove()
	default:
		fatalf("usage: keel hosts <setup|remove> [--ip <addr>]")
	}
}

func runHostsSetup(args []string, keelDir string) {
	// Determine IP: from --ip flag, from target, or default 127.0.0.1
	ip := ""
	for i, a := range args {
		if a == "--ip" && i+1 < len(args) {
			ip = args[i+1]
			if net.ParseIP(ip) == nil {
				fatalf("invalid IP address: %s", ip)
			}
			break
		}
	}

	if ip == "" {
		target, err := config.ReadTargetConfig(keelDir)
		if err == nil && target.Mode == "remote" {
			if target.ExternalIP != "" {
				ip = target.ExternalIP
			} else if target.Host != "" {
				// Host may be "user@1.2.3.4" — extract IP/hostname after @
				h := target.Host
				if at := strings.LastIndex(h, "@"); at >= 0 {
					h = h[at+1:]
				}
				ip = h
			}
		}
		if ip == "" {
			ip = "127.0.0.1"
		}
	}

	// Parse domains from dynamic.yml
	dynamicPath := filepath.Join(keelDir, "data", "config", "traefik", "dynamic.yml")
	domains, err := parseHostDomains(dynamicPath)
	if err != nil {
		fatalf("failed to parse traefik dynamic.yml: %v", err)
	}
	if len(domains) == 0 {
		fatalf("no Host() domains found in %s", dynamicPath)
	}

	// Build the hosts block
	block := buildHostsBlock(ip, domains)

	// Read current /etc/hosts
	hostsPath := "/etc/hosts"
	content, err := os.ReadFile(hostsPath)
	if err != nil {
		fatalf("failed to read %s: %v", hostsPath, err)
	}

	// Replace or append keel block
	newContent := replaceHostsBlock(string(content), block)

	// Write via sudo tee
	if err := writeSudo(hostsPath, newContent); err != nil {
		fatalf("failed to write %s: %v", hostsPath, err)
	}

	fmt.Printf("hosts updated (%s):\n", hostsPath)
	for _, d := range domains {
		fmt.Printf("  %s → %s\n", d, ip)
	}
}

func runHostsRemove() {
	hostsPath := "/etc/hosts"
	content, err := os.ReadFile(hostsPath)
	if err != nil {
		fatalf("failed to read %s: %v", hostsPath, err)
	}

	newContent := removeHostsBlock(string(content))
	if newContent == string(content) {
		fmt.Println("no keel entries found in /etc/hosts")
		return
	}

	if err := writeSudo(hostsPath, newContent); err != nil {
		fatalf("failed to write %s: %v", hostsPath, err)
	}

	fmt.Println("keel entries removed from /etc/hosts")
}

// parseHostDomains extracts unique Host(`...`) domains from the Traefik dynamic config.
func parseHostDomains(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	re := regexp.MustCompile("Host\\(`([^`]+)`\\)")
	seen := map[string]bool{}
	var domains []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		matches := re.FindAllStringSubmatch(scanner.Text(), -1)
		for _, m := range matches {
			domain := m[1]
			if !seen[domain] {
				seen[domain] = true
				domains = append(domains, domain)
			}
		}
	}
	return domains, scanner.Err()
}

func buildHostsBlock(ip string, domains []string) string {
	var b strings.Builder
	b.WriteString(hostsMarkerBegin + "\n")
	b.WriteString(ip + "  " + strings.Join(domains, " ") + "\n")
	b.WriteString(hostsMarkerEnd + "\n")
	return b.String()
}

func replaceHostsBlock(content, block string) string {
	begin := strings.Index(content, hostsMarkerBegin)
	end := strings.Index(content, hostsMarkerEnd)

	if begin >= 0 && end >= 0 {
		end += len(hostsMarkerEnd)
		// consume trailing newline
		if end < len(content) && content[end] == '\n' {
			end++
		}
		return content[:begin] + block + content[end:]
	}

	// Append with a blank line separator
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + "\n" + block
}

func removeHostsBlock(content string) string {
	begin := strings.Index(content, hostsMarkerBegin)
	end := strings.Index(content, hostsMarkerEnd)

	if begin < 0 || end < 0 {
		return content
	}

	end += len(hostsMarkerEnd)
	if end < len(content) && content[end] == '\n' {
		end++
	}
	// Remove extra blank line before the block
	if begin > 0 && content[begin-1] == '\n' {
		begin--
	}
	return content[:begin] + content[end:]
}

func writeSudo(path, content string) error {
	cmd := exec.Command("sudo", "tee", path)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = nil // suppress tee output
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
