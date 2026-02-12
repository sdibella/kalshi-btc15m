package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/sdibella/kalshi-btc15m/internal/dashboard"
)

//go:embed web/templates/*
var templateFS embed.FS

var templates *template.Template

func main() {
	_ = godotenv.Load()
	cfg := dashboard.ConfigFromEnv()
	reader := dashboard.NewReader(cfg)

	funcMap := template.FuncMap{
		"divf":    func(a, b float64) float64 { return a / b },
		"mulf":    func(a, b float64) float64 { return a * b },
		"addf":    func(a, b float64) float64 { return a + b },
		"toupper": strings.ToUpper,
		"float":   func(i int) float64 { return float64(i) },
		"abs": func(i int) int {
			if i < 0 {
				return -i
			}
			return i
		},
	}
	var err error
	templates, err = template.New("").Funcs(funcMap).ParseFS(templateFS, "web/templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/summary", handleSummary(reader))
	mux.HandleFunc("/api/trades", handleTrades(reader))
	mux.HandleFunc("/api/equity", handleEquity(reader))
	mux.HandleFunc("/api/performance", handlePerformance(reader))
	mux.HandleFunc("/", handleIndex())

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("BTC15M Dashboard starting on http://%s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func filterEvents(events []dashboard.Event, trading string) []dashboard.Event {
	if trading == "" {
		return events
	}
	wantDryRun := trading == "paper"

	var filtered []dashboard.Event
	sessionDryRun := -1

	for _, e := range events {
		switch e.Type {
		case "session_start":
			if e.SessionStart.DryRun {
				sessionDryRun = 1
			} else {
				sessionDryRun = 0
			}
			if e.SessionStart.DryRun == wantDryRun {
				filtered = append(filtered, e)
			}
		case "trade":
			if e.Trade.DryRun == wantDryRun {
				filtered = append(filtered, e)
			}
		case "settlement":
			if e.Settlement != nil && e.Settlement.DryRun == wantDryRun {
				filtered = append(filtered, e)
			}
		default:
			if sessionDryRun < 0 {
				continue
			}
			if (sessionDryRun == 1) == wantDryRun {
				filtered = append(filtered, e)
			}
		}
	}
	return filtered
}

func getEvents(reader *dashboard.Reader, r *http.Request) ([]dashboard.Event, error) {
	var events []dashboard.Event
	var err error

	if reader.Config().JournalFile != "" {
		events, err = reader.ParseJournal(reader.Config().JournalFile)
	} else {
		mode := r.URL.Query().Get("mode")

		if mode == "all" {
			events, err = reader.ParseAllSessions()
		} else {
			sessions, discoverErr := reader.DiscoverSessions()
			if discoverErr != nil {
				return nil, fmt.Errorf("failed to discover sessions: %w", discoverErr)
			}

			if len(sessions) == 0 {
				return nil, fmt.Errorf("no journal sessions found")
			}

			latest := sessions[0]
			journalPath := filepath.Join(reader.Config().JournalDir, latest.Filename)
			events, err = reader.ParseJournal(journalPath)
		}
	}

	if err != nil {
		return nil, err
	}

	trading := r.URL.Query().Get("trading")
	return filterEvents(events, trading), nil
}

func toInterfaceEvents(events []dashboard.Event) []interface{} {
	interfaceEvents := make([]interface{}, 0, len(events))
	for _, e := range events {
		switch e.Type {
		case "session_start":
			interfaceEvents = append(interfaceEvents, *e.SessionStart)
		case "trade":
			interfaceEvents = append(interfaceEvents, *e.Trade)
		case "settlement":
			interfaceEvents = append(interfaceEvents, *e.Settlement)
		}
	}
	return interfaceEvents
}

func handleSummary(reader *dashboard.Reader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := getEvents(reader, r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get events: %v", err), http.StatusInternalServerError)
			return
		}

		analyzer := dashboard.NewAnalyzer()
		analyzer.ProcessEvents(toInterfaceEvents(events))
		summary := analyzer.ComputeSummary()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(w, "summary.html", summary); err != nil {
			log.Printf("Failed to render summary template: %v", err)
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
		}
	}
}

func handleTrades(reader *dashboard.Reader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := getEvents(reader, r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get events: %v", err), http.StatusInternalServerError)
			return
		}

		analyzer := dashboard.NewAnalyzer()
		analyzer.ProcessEvents(toInterfaceEvents(events))
		trades := analyzer.GetTrades()

		sort.Slice(trades, func(i, j int) bool {
			return trades[i].Time > trades[j].Time
		})

		if len(trades) > 50 {
			trades = trades[:50]
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(w, "trades.html", trades); err != nil {
			log.Printf("Failed to render trades template: %v", err)
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
		}
	}
}

func handleEquity(reader *dashboard.Reader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := getEvents(reader, r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get events: %v", err), http.StatusInternalServerError)
			return
		}

		analyzer := dashboard.NewAnalyzer()
		analyzer.ProcessEvents(toInterfaceEvents(events))
		equity := analyzer.GetEquityCurve()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(equity); err != nil {
			log.Printf("Failed to encode equity curve: %v", err)
		}
	}
}

func handlePerformance(reader *dashboard.Reader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := getEvents(reader, r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get events: %v", err), http.StatusInternalServerError)
			return
		}

		analyzer := dashboard.NewAnalyzer()
		analyzer.ProcessEvents(toInterfaceEvents(events))
		performance := analyzer.ComputePerformance()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(w, "performance.html", performance); err != nil {
			log.Printf("Failed to render performance template: %v", err)
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
		}
	}
}

func handleIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(w, "index.html", nil); err != nil {
			log.Printf("Failed to render index template: %v", err)
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
		}
	}
}
