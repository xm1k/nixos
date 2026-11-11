package main

import (
	"os"
	"path/filepath"
	"strings"
)

func xdg(env, fallback string) string {
	if d := os.Getenv(env); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

func configDir() string { return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "bmusic") }
func stateDir() string  { return filepath.Join(xdg("XDG_STATE_HOME", ".local/state"), "bmusic") }

func runtimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}

func socketPath() string { return filepath.Join(runtimeDir(), "bmusic.sock") }
func mpvSocket() string  { return filepath.Join(runtimeDir(), "bmusic-mpv.sock") }
func tokenFile() string  { return filepath.Join(configDir(), "session") }

// loadToken: BMUSIC_TOKEN wins over the saved session.
func loadToken() string {
	if t := os.Getenv("BMUSIC_TOKEN"); t != "" {
		return t
	}
	b, err := os.ReadFile(tokenFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func saveToken(t string) error {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(tokenFile(), []byte(t+"\n"), 0o600)
}

// cleanEnv drops proxy variables: b.net is not reachable through the proxy.
func cleanEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToLower(k) {
		case "http_proxy", "https_proxy", "all_proxy", "no_proxy":
			continue
		}
		out = append(out, kv)
	}
	return out
}
