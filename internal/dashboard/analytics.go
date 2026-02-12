package dashboard

import (
	"time"

	"github.com/sdibella/kalshi-btc15m/internal/journal"
)

// Analyzer aggregates journal events into dashboard views.
type Analyzer struct {
	trades      map[string]*tradeAggregator
	settlements []journal.Settlement

	lastSessionBalance int
	postSessionPnL     int
	hasSession         bool

	equityCurve []EquityPoint
	startTime   time.Time
}

// tradeAggregator accumulates trade fills for a single market.
type tradeAggregator struct {
	ticker    string
	side      string
	time      string
	quantity  int
	totalCost int // sum of (quantity * price)
	fees      int
	settled   bool
	won       bool
	pnl       int
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		trades:      make(map[string]*tradeAggregator),
		settlements: make([]journal.Settlement, 0),
		equityCurve: make([]EquityPoint, 0),
	}
}

// ProcessEvents processes a slice of journal events and aggregates trade data.
func (a *Analyzer) ProcessEvents(events []interface{}) {
	for _, event := range events {
		switch e := event.(type) {
		case journal.SessionStart:
			a.processSessionStart(e)
		case journal.Trade:
			a.processTrade(e)
		case journal.Settlement:
			a.processSettlement(e)
		}
	}
}

func (a *Analyzer) processSessionStart(s journal.SessionStart) {
	t, err := time.Parse(time.RFC3339Nano, s.Time)
	if err != nil {
		t = time.Now()
	}

	if !a.hasSession {
		a.startTime = t
		a.equityCurve = append(a.equityCurve, EquityPoint{
			Time:         t,
			BalanceCents: s.BalanceCents,
		})
	}

	a.lastSessionBalance = s.BalanceCents
	a.postSessionPnL = 0
	a.hasSession = true
}

func (a *Analyzer) processTrade(t journal.Trade) {
	ticker := t.Ticker

	agg, exists := a.trades[ticker]
	if !exists {
		agg = &tradeAggregator{
			ticker: ticker,
			side:   t.Side,
			time:   t.Time,
		}
		a.trades[ticker] = agg
	}

	agg.quantity += t.Quantity
	agg.totalCost += t.Quantity * t.Price
	agg.fees += t.FeeCents
}

func (a *Analyzer) processSettlement(s journal.Settlement) {
	a.settlements = append(a.settlements, s)
	a.postSessionPnL += s.PnLCents

	if a.hasSession {
		currentBal := a.lastSessionBalance + a.postSessionPnL
		t, err := time.Parse(time.RFC3339Nano, s.Time)
		if err != nil {
			t = time.Now()
		}
		a.equityCurve = append(a.equityCurve, EquityPoint{
			Time:         t,
			BalanceCents: currentBal,
		})
	}

	ticker := s.Ticker
	agg, exists := a.trades[ticker]
	if !exists {
		agg = &tradeAggregator{
			ticker: ticker,
			side:   s.Side,
			time:   s.Time,
		}
		a.trades[ticker] = agg
	}

	agg.settled = true
	agg.won = s.Won
	agg.pnl = s.PnLCents
}

func (a *Analyzer) currentBalance() int {
	return a.lastSessionBalance + a.postSessionPnL
}

// GetTrades returns all aggregated trades as TradeView objects.
func (a *Analyzer) GetTrades() []TradeView {
	trades := make([]TradeView, 0, len(a.trades))

	for _, agg := range a.trades {
		avgPrice := 0.0
		if agg.quantity > 0 {
			avgPrice = float64(agg.totalCost) / float64(agg.quantity)
		}

		result := "open"
		if agg.settled {
			if agg.won {
				result = "win"
			} else {
				result = "loss"
			}
		}

		tv := TradeView{
			Time:     agg.time,
			Ticker:   agg.ticker,
			Side:     agg.side,
			Quantity: agg.quantity,
			AvgPrice: avgPrice,
			Result:   result,
			PnL:      agg.pnl,
			Fees:     agg.fees,
		}

		trades = append(trades, tv)
	}

	return trades
}

// ComputeSummary returns summary statistics for the journal.
func (a *Analyzer) ComputeSummary() Summary {
	var totalNetPnL, totalFees int
	var winCount, lossCount int

	for _, s := range a.settlements {
		totalNetPnL += s.PnLCents
		totalFees += s.FeeCents
		if s.Won {
			winCount++
		} else {
			lossCount++
		}
	}

	totalMarkets := winCount + lossCount

	winRate := 0.0
	if totalMarkets > 0 {
		winRate = float64(winCount) / float64(totalMarkets)
	}

	curBal := a.currentBalance()

	roi := 0.0
	if a.lastSessionBalance > 0 {
		roi = float64(totalNetPnL) / float64(a.lastSessionBalance) * 100
	}

	peakBal := curBal
	for _, ep := range a.equityCurve {
		if ep.BalanceCents > peakBal {
			peakBal = ep.BalanceCents
		}
	}
	currentDrawdown := 0.0
	if peakBal > 0 {
		currentDrawdown = float64(peakBal-curBal) / float64(peakBal) * 100
	}

	// Compute current streak from settlements in chronological order
	streak := 0
	for i := len(a.settlements) - 1; i >= 0; i-- {
		if a.settlements[i].Won {
			if streak < 0 {
				break
			}
			streak++
		} else {
			if streak > 0 {
				break
			}
			streak--
		}
	}

	// Compute max drawdown from equity curve
	maxDrawdown := 0.0
	peak := 0
	for _, ep := range a.equityCurve {
		if ep.BalanceCents > peak {
			peak = ep.BalanceCents
		}
		if peak > 0 {
			dd := float64(peak-ep.BalanceCents) / float64(peak) * 100
			if dd > maxDrawdown {
				maxDrawdown = dd
			}
		}
	}

	return Summary{
		BalanceCents:    curBal,
		SessionPnL:      totalNetPnL,
		TotalPnL:        totalNetPnL,
		WinCount:        winCount,
		LossCount:       lossCount,
		WinRate:         winRate,
		TotalFees:       totalFees,
		ROI:             roi,
		CurrentDrawdown: currentDrawdown,
		MaxDrawdown:     maxDrawdown,
		TotalMarkets:    totalMarkets,
		Streak:          streak,
	}
}

// ComputePerformance returns performance breakdown by side and entry price.
func (a *Analyzer) ComputePerformance() PerformanceBreakdown {
	bySide := make(map[string]SideStats)

	// Price range buckets: 80-84, 85-89, 90-94, 95-99
	type bucket struct {
		label string
		lo    int // inclusive
		hi    int // inclusive
	}
	buckets := []bucket{
		{"80-84c", 80, 84},
		{"85-89c", 85, 89},
		{"90-94c", 90, 94},
		{"95-99c", 95, 99},
	}
	priceStats := make([]PriceRangeStats, len(buckets))
	for i, b := range buckets {
		priceStats[i].Label = b.label
	}

	var totalWinPnL, totalLossPnL float64
	var winCount, lossCount, totalFees int

	for _, agg := range a.trades {
		if !agg.settled {
			continue
		}

		// By side
		if agg.side != "" {
			sideStats := bySide[agg.side]
			sideStats.Trades++
			if agg.won {
				sideStats.Wins++
			}
			sideStats.TotalPnL += agg.pnl
			bySide[agg.side] = sideStats
		}

		// By entry price bucket
		entryPrice := 0
		if agg.quantity > 0 {
			entryPrice = agg.totalCost / agg.quantity
		}
		for i, b := range buckets {
			if entryPrice >= b.lo && entryPrice <= b.hi {
				priceStats[i].Trades++
				if agg.won {
					priceStats[i].Wins++
				}
				priceStats[i].TotalPnL += agg.pnl
				break
			}
		}

		// Avg win/loss
		totalFees += agg.fees
		if agg.won {
			totalWinPnL += float64(agg.pnl)
			winCount++
		} else {
			totalLossPnL += float64(agg.pnl)
			lossCount++
		}
	}

	for side, stats := range bySide {
		if stats.Trades > 0 {
			stats.WinRate = float64(stats.Wins) / float64(stats.Trades)
			stats.AvgPnL = float64(stats.TotalPnL) / float64(stats.Trades)
			bySide[side] = stats
		}
	}

	for i := range priceStats {
		if priceStats[i].Trades > 0 {
			priceStats[i].WinRate = float64(priceStats[i].Wins) / float64(priceStats[i].Trades)
			priceStats[i].AvgPnL = float64(priceStats[i].TotalPnL) / float64(priceStats[i].Trades)
		}
	}

	avgWin := 0.0
	if winCount > 0 {
		avgWin = totalWinPnL / float64(winCount)
	}
	avgLoss := 0.0
	if lossCount > 0 {
		avgLoss = totalLossPnL / float64(lossCount)
	}

	total := winCount + lossCount
	expectancy := 0.0
	if total > 0 {
		wr := float64(winCount) / float64(total)
		expectancy = avgWin*wr + avgLoss*(1-wr) // avgLoss is already negative
	}

	return PerformanceBreakdown{
		BySide:     bySide,
		ByPrice:    priceStats,
		AvgWin:     avgWin,
		AvgLoss:    avgLoss,
		Expectancy: expectancy,
		TotalFees:  totalFees,
	}
}

// GetEquityCurve returns the equity curve, sampled to 1000 points if longer.
func (a *Analyzer) GetEquityCurve() []EquityPoint {
	if len(a.equityCurve) <= 1000 {
		return a.equityCurve
	}

	sampled := make([]EquityPoint, 1000)
	step := float64(len(a.equityCurve)-1) / 999.0

	for i := 0; i < 1000; i++ {
		idx := int(float64(i) * step)
		sampled[i] = a.equityCurve[idx]
	}

	return sampled
}
