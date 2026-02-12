package dashboard

import (
	"os"
	"path/filepath"
	"strconv"
)

// Config holds configuration for the dashboard HTTP server.
type Config struct {
	Port        int    // HTTP server port
	Host        string // Bind address (e.g., "localhost", "0.0.0.0")
	JournalDir  string // Path to directory containing journal JSONL files
	JournalFile string // Path to a specific journal file (overrides JournalDir when set)
	RefreshRate int    // Seconds between dashboard updates
}

// DefaultConfig returns dashboard configuration with sensible defaults.
func DefaultConfig() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return Config{
		Port:        8080,
		Host:        "localhost",
		JournalDir:  filepath.Join(home, ".kalshi-bot"),
		RefreshRate: 3,
	}
}

// ConfigFromEnv returns configuration with values overridden from environment variables.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if port := os.Getenv("DASHBOARD_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Port = p
		}
	}

	if host := os.Getenv("DASHBOARD_HOST"); host != "" {
		cfg.Host = host
	}

	if dir := os.Getenv("DASHBOARD_JOURNAL_DIR"); dir != "" {
		cfg.JournalDir = dir
	}

	if file := os.Getenv("DASHBOARD_JOURNAL_FILE"); file != "" {
		cfg.JournalFile = file
	}

	return cfg
}
