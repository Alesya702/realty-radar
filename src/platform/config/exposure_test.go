package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalDeploymentBindsToLoopback checks the local development files that
// decide who can reach the MVP. The application and the database have no
// authentication, so neither may be published on all interfaces (A-02).
func TestLocalDeploymentBindsToLoopback(t *testing.T) {
	root := filepath.Join("..", "..", "..")

	compose := readRepoFile(t, filepath.Join(root, "docker-compose.yml"))
	for _, published := range publishedPorts(compose) {
		if !strings.HasPrefix(published, "127.0.0.1:") {
			t.Errorf("docker-compose.yml publishes %q on all interfaces, want a 127.0.0.1 binding", published)
		}
	}

	env := readRepoFile(t, filepath.Join(root, ".env.example"))
	for _, line := range strings.Split(env, "\n") {
		line = strings.TrimSpace(line)
		addr, found := strings.CutPrefix(line, "RR_HTTP_ADDR=")
		if !found {
			continue
		}
		if !strings.HasPrefix(addr, "127.0.0.1:") {
			t.Errorf(".env.example sets RR_HTTP_ADDR=%q, want a 127.0.0.1 address", addr)
		}
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

// publishedPorts returns the host side of every "host:container" port mapping
// of docker-compose.yml (lines of the form `- "..."` under `ports:`).
func publishedPorts(compose string) []string {
	var ports []string
	inPorts := false
	for _, raw := range strings.Split(compose, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		switch {
		case line == "ports:":
			inPorts = true
		case !inPorts || strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, "- "):
			ports = append(ports, strings.Trim(strings.TrimPrefix(line, "- "), `"`))
		default:
			inPorts = false
		}
	}
	return ports
}
