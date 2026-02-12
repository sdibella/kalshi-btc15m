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

	// Init journal
	j, err := journal.New(cfg.JournalPath)
	if err != nil {
		slog.Error("journal init failed", "err", err)
		os.Exit(1)
	}
	defer j.Close()
	_ = j.Log(journal.NewSessionStart(cfg.KalshiEnv, cfg.DryRun, bal.Balance))
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

	dashboardBinary := filepath.Join(filepath.Dir(exePath), "dashboard")
	_, err = os.Stat(dashboardBinary)
	if err != nil {
		slog.Warn("dashboard binary not found", "path", dashboardBinary)
		return nil
	}

	cmd := exec.Command(dashboardBinary)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		slog.Error("failed to start dashboard", "err", err)
		return nil
	}

	slog.Info("dashboard started", "pid", cmd.Process.Pid)
	return cmd
}
