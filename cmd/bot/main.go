package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sdibella/kalshi-btc15m/internal/config"
	"github.com/sdibella/kalshi-btc15m/internal/journal"
	"github.com/sdibella/kalshi-btc15m/internal/kalshi"
	"github.com/sdibella/kalshi-btc15m/internal/strategy"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "paper trade only (no real orders)")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	// Logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	// CLI overrides
	if *dryRun {
		cfg.DryRun = true
	}

	slog.Info("kalshi btc15m bot starting",
		"env", cfg.KalshiEnv,
		"dryRun", cfg.DryRun,
	)

	// Init Kalshi REST client
	client, err := kalshi.NewClient(cfg)
	if err != nil {
		slog.Error("kalshi client init failed", "err", err)
		os.Exit(1)
	}

	// Init Kalshi WebSocket client for orderbook streaming
	wsClient, err := kalshi.NewWSClient(cfg)
	if err != nil {
		slog.Error("kalshi ws client init failed", "err", err)
		os.Exit(1)
	}

	// Context with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start dashboard subprocess
	dashboardCmd := startDashboard()

	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		if dashboardCmd != nil && dashboardCmd.Process != nil {
			dashboardCmd.Process.Signal(syscall.SIGTERM)
		}
		cancel()
	}()

	// Start Kalshi WebSocket for orderbook streaming
	go func() {
		if err := wsClient.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("kalshi ws error", "err", err)
		}
	}()

	// Verify auth with a balance check
	bal, err := client.GetBalance(ctx)
	if err != nil {
		slog.Error("auth check failed — cannot reach Kalshi API", "err", err)
		os.Exit(1)
	}
	slog.Info("authenticated", "balance", fmt.Sprintf("$%.2f", float64(bal.Balance)/100.0))

	// Load Bayesian posterior from file (or use default Beta(83, 3))
	posteriorPath := "posterior.json"
	if err := strategy.BayesianWinRate.LoadFromFile(posteriorPath); err != nil {
		slog.Error("failed to load Bayesian posterior", "err", err)
		// Continue with default prior
	}
	slog.Info("Bayesian posterior loaded (monitoring only, Kelly uses fixed 0.92)",
		"median", fmt.Sprintf("%.1f%%", strategy.BayesianWinRate.Median()*100),
	)

	// Init journal
	j, err := journal.New(cfg.JournalPath)
	if err != nil {
		slog.Error("journal init failed", "err", err)
		os.Exit(1)
	}
	defer j.Close()
	if err := j.Log(journal.NewSessionStart(cfg.KalshiEnv, cfg.DryRun, bal.Balance)); err != nil {
		slog.Error("failed to journal session start", "err", err)
	}
	slog.Info("journal opened", "path", cfg.JournalPath)

	// Start strategy engine
	engine := strategy.NewEngine(client, wsClient, cfg, j)
	if err := engine.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("engine error", "err", err)
		os.Exit(1)
	}

	slog.Info("bot stopped")
}


func startDashboard() *exec.Cmd {
	// Find dashboard binary in same directory as this executable
	exePath, err := os.Executable()
	if err != nil {
		slog.Error("failed to get executable path", "err", err)
		return nil
	}

	dashboardBinary := filepath.Join(filepath.Dir(exePath), "btc15m-dashboard")
	_, err = os.Stat(dashboardBinary)
	if err != nil {
		slog.Warn("dashboard binary not found", "path", dashboardBinary)
		return nil
	}

	cmd := exec.Command(dashboardBinary)

	// Open dashboard log file
	dashboardDir := filepath.Dir(exePath)
	logFile, err := os.OpenFile(
		filepath.Join(dashboardDir, "dashboard.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		slog.Error("failed to open dashboard log", "err", err)
		return nil
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err = cmd.Start()
	if err != nil {
		slog.Error("failed to start dashboard", "err", err)
		logFile.Close()
		return nil
	}

	slog.Info("dashboard started", "pid", cmd.Process.Pid, "logFile", logFile.Name())
	return cmd
}
