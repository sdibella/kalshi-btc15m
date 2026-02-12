package journal

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Journal is an append-only JSONL writer for trade events.
type Journal struct {
	f  *os.File
	mu sync.Mutex
}

// New opens (or creates) the journal file in append mode.
func New(path string) (*Journal, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &Journal{f: f}, nil
}

// Log marshals event to JSON and appends it as a single line.
func (j *Journal) Log(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	j.mu.Lock()
	defer j.mu.Unlock()
	if _, err = j.f.Write(data); err != nil {
		return err
	}
	return j.f.Sync()
}

// Close flushes and closes the underlying file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.f.Close()
}

// Event types -- simplified for BTC 15-min strategy.

type SessionStart struct {
	Type         string `json:"type"`
	Time         string `json:"time"`
	DryRun       bool   `json:"dry_run"`
	Env          string `json:"env"`
	BalanceCents int    `json:"balance_cents"`
}

func NewSessionStart(env string, dryRun bool, balance int) SessionStart {
	return SessionStart{
		Type:         "session_start",
		Time:         time.Now().UTC().Format(time.RFC3339Nano),
		DryRun:       dryRun,
		Env:          env,
		BalanceCents: balance,
	}
}

type Trade struct {
	Type       string `json:"type"`
	Time       string `json:"time"`
	Ticker     string `json:"ticker"`
	Side       string `json:"side"`
	Action     string `json:"action"`
	Price      int    `json:"price"`
	Quantity   int    `json:"quantity"`
	FeeCents   int    `json:"fee_cents"`
	OrderID    string `json:"order_id"`
	Filled     int    `json:"filled"`
	DryRun     bool   `json:"dry_run"`
	LimitPrice int    `json:"limit_price"`
}

func NewTrade(ticker, side, action string, price, quantity, feeCents int, orderID string, filled int, dryRun bool, limitPrice int) Trade {
	return Trade{
		Type:       "trade",
		Time:       time.Now().UTC().Format(time.RFC3339Nano),
		Ticker:     ticker,
		Side:       side,
		Action:     action,
		Price:      price,
		Quantity:   quantity,
		FeeCents:   feeCents,
		OrderID:    orderID,
		Filled:     filled,
		DryRun:     dryRun,
		LimitPrice: limitPrice,
	}
}

type Settlement struct {
	Type            string    `json:"type"`
	Time            string    `json:"time"`
	Ticker          string    `json:"ticker"`
	Strike          float64   `json:"strike"`
	AvgBRTI         float64   `json:"avg_brti"`
	Won             bool      `json:"won"`
	PnLCents        int       `json:"pnl_cents"`
	FeeCents        int       `json:"fee_cents"`
	Side            string    `json:"side"`
	EntryPrice      int       `json:"entry_price"`
	Contracts       int       `json:"contracts"`
	SettlementTicks []float64 `json:"settlement_ticks"`
	DryRun          bool      `json:"dry_run"`
}

func NewSettlement(ticker string, strike, avgBRTI float64, won bool, pnl, fees int, side string, entryPrice, contracts int, ticks []float64, dryRun bool) Settlement {
	return Settlement{
		Type:            "settlement",
		Time:            time.Now().UTC().Format(time.RFC3339Nano),
		Ticker:          ticker,
		Strike:          strike,
		AvgBRTI:         avgBRTI,
		Won:             won,
		PnLCents:        pnl,
		FeeCents:        fees,
		Side:            side,
		EntryPrice:      entryPrice,
		Contracts:       contracts,
		SettlementTicks: ticks,
		DryRun:          dryRun,
	}
}
