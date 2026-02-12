package dashboard

import "time"

// View models for API responses

type Summary struct {
	BalanceCents    int     `json:"balance_cents"`
	SessionPnL      int     `json:"session_pnl"`
	TotalPnL        int     `json:"total_pnl"`
	WinCount        int     `json:"win_count"`
	LossCount       int     `json:"loss_count"`
	WinRate         float64 `json:"win_rate"`
	TotalFees       int     `json:"total_fees"`
	ROI             float64 `json:"roi"`
	CurrentDrawdown float64 `json:"current_drawdown_pct"`
	MaxDrawdown     float64 `json:"max_drawdown_pct"`
	TotalMarkets    int     `json:"total_markets"`
	Streak          int     `json:"streak"` // positive=wins, negative=losses
	LastUpdated     string  `json:"last_updated"`
}

type TradeView struct {
	Time     string  `json:"time"`
	Ticker   string  `json:"ticker"`
	Side     string  `json:"side"`
	Quantity int     `json:"quantity"`
	AvgPrice float64 `json:"avg_price"`
	Result   string  `json:"result"` // "win"/"loss"/"open"
	PnL      int     `json:"pnl"`
	Fees     int     `json:"fees"`
}

type EquityPoint struct {
	Time         time.Time `json:"time"`
	BalanceCents int       `json:"balance_cents"`
}

type SideStats struct {
	Trades   int     `json:"trades"`
	Wins     int     `json:"wins"`
	WinRate  float64 `json:"win_rate"`
	AvgPnL   float64 `json:"avg_pnl"`
	TotalPnL int     `json:"total_pnl"`
}

type PriceRangeStats struct {
	Label    string  `json:"label"` // e.g. "80-84c"
	Trades   int     `json:"trades"`
	Wins     int     `json:"wins"`
	WinRate  float64 `json:"win_rate"`
	AvgPnL   float64 `json:"avg_pnl"`
	TotalPnL int     `json:"total_pnl"`
}

type PerformanceBreakdown struct {
	BySide      map[string]SideStats `json:"by_side"`
	ByPrice     []PriceRangeStats    `json:"by_price"`
	AvgWin      float64              `json:"avg_win"`
	AvgLoss     float64              `json:"avg_loss"`
	Expectancy  float64              `json:"expectancy"`
	TotalFees   int                  `json:"total_fees"`
}

// Session info for selector
type SessionInfo struct {
	Filename  string    `json:"filename"`
	StartTime time.Time `json:"start_time"`
	Display   string    `json:"display"` // Human-readable like "Feb 10, 2:15 PM"
}
