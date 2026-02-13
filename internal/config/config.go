package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	KalshiAPIKeyID    string
	KalshiPrivKeyPath string
	KalshiEnv         string // "prod" or "demo"
	DryRun            bool
	JournalPath       string

	// Dashboard
	DashboardPort int
	DashboardHost string
	JournalDir    string

	// Volatility filter
	VolDataDir   string  // path to data collector's data directory
	VolMaxStdDev float64 // stddev threshold in dollars to block trading
}

func (c *Config) BaseURL() string {
	if c.KalshiEnv == "prod" {
		return "https://api.elections.kalshi.com/trade-api/v2"
	}
	return "https://demo-api.kalshi.co/trade-api/v2"
}

func (c *Config) WSBaseURL() string {
	if c.KalshiEnv == "prod" {
		return "wss://api.elections.kalshi.com/trade-api/ws/v2"
	}
	return "wss://demo-api.kalshi.co/trade-api/ws/v2"
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		KalshiAPIKeyID:    os.Getenv("KALSHI_API_KEY_ID"),
		KalshiPrivKeyPath: getEnvDefault("KALSHI_PRIV_KEY_PATH", "./kalshi_private_key.pem"),
		KalshiEnv:         getEnvDefault("KALSHI_ENV", "prod"),
		DryRun:            getEnvBool("DRY_RUN", true),
		JournalPath:       getEnvDefault("JOURNAL_PATH", "./journal.jsonl"),
		DashboardPort:     getEnvInt("DASHBOARD_PORT", 8080),
		DashboardHost:     getEnvDefault("DASHBOARD_HOST", "localhost"),
		JournalDir:        getEnvDefault("DASHBOARD_JOURNAL_DIR", "."),
		VolDataDir:        getEnvDefault("VOL_DATA_DIR", "/home/stefan/KalshiBTC15min-data/data"),
		VolMaxStdDev:      getEnvFloat("VOL_MAX_STDDEV", 200.0),
	}

	if cfg.KalshiAPIKeyID == "" {
		return nil, fmt.Errorf("KALSHI_API_KEY_ID is required")
	}
	if cfg.KalshiEnv != "prod" && cfg.KalshiEnv != "demo" {
		return nil, fmt.Errorf("KALSHI_ENV must be 'prod' or 'demo', got %q", cfg.KalshiEnv)
	}

	return cfg, nil
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getEnvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
