package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sdibella/kalshi-btc15m/internal/config"
	"github.com/sdibella/kalshi-btc15m/internal/journal"
	"github.com/sdibella/kalshi-btc15m/internal/kalshi"
)

// Signal represents an entry signal for a market.
type Signal struct {
	Side       string // "yes", "no", or "" (no trade)
	LimitPrice int    // price in cents to place limit order at
	RefAsk     int    // the ask price that triggered the signal
}

// MarketState tracks the lifecycle of a single market.
type MarketState struct {
	Ticker        string
	Strike        float64
	StrikeFetched bool
	CloseTime     time.Time // when trading ends (market closes)
	Subscribed    bool

	// Entry state
	Evaluated  bool // true once we've evaluated at the 4-min mark (don't re-evaluate)
	Traded     bool
	Side       string
	EntryPrice int
	Contracts  int
	FeeCents   int

	// Order management
	OrderPending  bool
	OrderID       string
	OrderPlacedAt time.Time

	// Settlement — polled from Kalshi API after market settles (~6min post-close)
	Settled            bool
	LastSettlementPoll time.Time

	// Rate limiting for strike fetch
	LastStrikePoll time.Time
}

// Engine is the main trading engine for the BTC 15-min strategy.
type Engine struct {
	client  *kalshi.Client
	ws      *kalshi.WSClient
	cfg     *config.Config
	journal *journal.Journal

	markets map[string]*MarketState
	mu      sync.Mutex

	balance         int
	lastBalanceSync time.Time
	lastDiscovery   time.Time
}

// NewEngine creates a new strategy engine.
func NewEngine(client *kalshi.Client, ws *kalshi.WSClient, cfg *config.Config, j *journal.Journal) *Engine {
	return &Engine{
		client:  client,
		ws:      ws,
		cfg:     cfg,
		journal: j,
		markets: make(map[string]*MarketState),
	}
}

// Evaluate determines whether to trade based on orderbook prices.
// Threshold 80c filters for high-confidence markets (96% WR in backtest).
// Limit at ask price for immediate taker fill.
func Evaluate(yesBid, yesAsk int) Signal {
	const threshold = 80
	if yesAsk >= threshold {
		return Signal{Side: "yes", LimitPrice: yesAsk, RefAsk: yesAsk}
	}
	noAsk := 100 - yesBid
	if noAsk >= threshold {
		return Signal{Side: "no", LimitPrice: noAsk, RefAsk: noAsk}
	}
	return Signal{} // no trade
}

// InEntryWindow returns true when we're in the last 4 minutes before close.
// Per spec: evaluate once when secs_left first crosses below 534 (4 min before close).
// In secsUntilClose terms: 0 < secsUntilClose <= 240.
func InEntryWindow(secsUntilClose float64) bool {
	return secsUntilClose > 0 && secsUntilClose <= 240
}

// AssumedWinRate is the backtest win rate used for Kelly sizing.
// 93.5% from 31-trade backtest. Update as live data accumulates.
const AssumedWinRate = 0.935

// KellySize computes the quarter-Kelly contract count per the strategy spec.
//
//	fee         = 0.07 * min(entry, 100-entry)  (per contract, in cents)
//	win_profit  = 100 - entry - fee
//	loss_amount = entry + fee
//	b           = win_profit / loss_amount
//	kelly       = p - (q / b)
//	contracts   = floor(0.25 * kelly * bankroll / cost_per_contract)
//	cost_per_contract = entry + fee  (in cents)
//
// Returns 0 if Kelly says no bet. Minimum 1 contract.
func KellySize(limitPrice, balanceCents int) int {
	if limitPrice <= 0 || limitPrice >= 100 || balanceCents <= 0 {
		return 0
	}

	entry := float64(limitPrice)
	fee := 0.07 * math.Min(entry, 100-entry)
	winProfit := 100 - entry - fee
	lossAmount := entry + fee

	if winProfit <= 0 || lossAmount <= 0 {
		return 0
	}

	p := AssumedWinRate
	q := 1 - p
	b := winProfit / lossAmount
	kelly := p - (q / b)

	if kelly <= 0 {
		return 0
	}

	quarterKelly := 0.25 * kelly
	costPerContract := entry + fee
	contracts := int(math.Floor(quarterKelly * float64(balanceCents) / costPerContract))

	if contracts < 1 {
		return 0
	}
	return contracts
}

// TakerFee computes the Kalshi taker fee in cents.
// fee = ceil(0.07 * contracts * P * (1-P) * 100)
// where P = priceCents / 100
func TakerFee(contracts, priceCents int) int {
	p := float64(priceCents) / 100.0
	fee := 0.07 * float64(contracts) * p * (1 - p) * 100.0
	return int(math.Ceil(fee))
}

// ComputePnL computes the P&L in cents for a settled position.
// win: pnl = (100 - entry) * contracts - fee
// loss: pnl = -(entry * contracts + fee)
func ComputePnL(won bool, entryPrice, contracts, feeCents int) int {
	if won {
		return (100-entryPrice)*contracts - feeCents
	}
	return -(entryPrice*contracts + feeCents)
}

// reconcilePositions queries the Kalshi API for existing BTC15M positions
// and pre-populates the markets map so the engine doesn't re-trade on restart.
func (e *Engine) reconcilePositions(ctx context.Context) {
	if e.cfg.DryRun {
		slog.Info("skipping position reconciliation (dry-run mode)")
		return
	}

	positions, err := e.client.GetPositions(ctx, "")
	if err != nil {
		slog.Error("position reconciliation failed", "err", err)
		return
	}

	reconciled := 0
	for _, pos := range positions {
		if pos.Position == 0 || !strings.HasPrefix(pos.Ticker, "KXBTC15M-") {
			continue
		}

		m, err := e.client.GetMarket(ctx, pos.Ticker)
		if err != nil {
			slog.Warn("reconcile: failed to get market", "ticker", pos.Ticker, "err", err)
			continue
		}

		// Already settled — nothing to track
		if m.Result != "" {
			continue
		}

		closeTime, err := m.CloseTimeParsed()
		if err != nil || closeTime.IsZero() {
			slog.Warn("reconcile: bad close time", "ticker", pos.Ticker, "err", err)
			continue
		}

		var side string
		contracts := pos.Position
		if contracts > 0 {
			side = "yes"
		} else {
			side = "no"
			contracts = -contracts
		}

		avgPrice, fee := e.reconstructEntry(ctx, pos.Ticker, side, contracts)

		ms := &MarketState{
			Ticker:        pos.Ticker,
			CloseTime:     closeTime,
			Strike:        m.StrikePrice(),
			StrikeFetched: m.StrikePrice() > 0,
			Evaluated:     true,
			Traded:        true,
			Side:          side,
			EntryPrice:    avgPrice,
			Contracts:     contracts,
			FeeCents:      fee,
		}

		e.mu.Lock()
		e.markets[pos.Ticker] = ms
		e.mu.Unlock()

		// Subscribe to WS if market is still open (needed for settlement polling after close)
		if err := e.ws.Subscribe([]string{pos.Ticker}); err != nil {
			slog.Warn("reconcile: ws subscribe failed", "ticker", pos.Ticker, "err", err)
		} else {
			ms.Subscribed = true
		}

		slog.Info("reconciled position",
			"ticker", pos.Ticker,
			"side", side,
			"contracts", contracts,
			"avgPrice", avgPrice,
			"fee", fee,
			"closeTime", closeTime.Format(time.RFC3339),
		)
		reconciled++
	}

	slog.Info("position reconciliation complete", "reconciled", reconciled)
}

// reconstructEntry computes the weighted-average entry price from fills for a position.
func (e *Engine) reconstructEntry(ctx context.Context, ticker, side string, contracts int) (avgPrice, fee int) {
	params := url.Values{}
	params.Set("ticker", ticker)

	fills, _, err := e.client.GetFills(ctx, params)
	if err != nil {
		slog.Warn("reconcile: failed to get fills", "ticker", ticker, "err", err)
		return 0, 0
	}

	totalCost := 0
	totalContracts := 0
	for _, f := range fills {
		if f.Action != "buy" {
			continue
		}
		var price int
		if f.Side == "yes" {
			price = f.YesPrice
		} else {
			price = f.NoPrice
		}
		totalCost += f.Count * price
		totalContracts += f.Count
	}

	if totalContracts == 0 {
		return 0, 0
	}

	avgPrice = totalCost / totalContracts
	fee = TakerFee(contracts, avgPrice)
	return avgPrice, fee
}

// Run starts the engine's main loop with a 1-second ticker.
func (e *Engine) Run(ctx context.Context) error {
	e.reconcilePositions(ctx)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	slog.Info("strategy engine started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	// Refresh balance every 60 seconds
	if time.Since(e.lastBalanceSync) > 60*time.Second {
		if bal, err := e.client.GetBalance(ctx); err == nil {
			e.balance = bal.Balance
			e.lastBalanceSync = time.Now()
		}
	}

	// Discover new markets every 30 seconds
	if time.Since(e.lastDiscovery) > 30*time.Second {
		e.discoverMarkets(ctx)
		e.lastDiscovery = time.Now()
	}

	// Process each tracked market
	e.mu.Lock()
	tickers := make([]string, 0, len(e.markets))
	for t := range e.markets {
		tickers = append(tickers, t)
	}
	e.mu.Unlock()

	for _, ticker := range tickers {
		e.mu.Lock()
		ms := e.markets[ticker]
		e.mu.Unlock()

		if ms == nil || ms.Settled {
			continue
		}

		e.processMarket(ctx, ms)
	}
}

func (e *Engine) discoverMarkets(ctx context.Context) {
	markets, err := e.client.GetMarkets(ctx, "KXBTC15M", "open")
	if err != nil {
		slog.Warn("market discovery failed", "err", err)
		return
	}

	for _, m := range markets {
		e.mu.Lock()
		_, exists := e.markets[m.Ticker]
		e.mu.Unlock()

		if exists {
			continue
		}

		closeTime, err := m.CloseTimeParsed()
		if err != nil || closeTime.IsZero() {
			slog.Warn("bad close time", "ticker", m.Ticker, "err", err)
			continue
		}

		secsUntilClose := time.Until(closeTime).Seconds()
		if secsUntilClose <= 0 {
			continue // already closed
		}

		ms := &MarketState{
			Ticker:    m.Ticker,
			CloseTime: closeTime,
		}

		// Try to get strike immediately
		strike := m.StrikePrice()
		if strike > 0 {
			ms.Strike = strike
			ms.StrikeFetched = true
		}

		e.mu.Lock()
		e.markets[m.Ticker] = ms
		e.mu.Unlock()

		slog.Info("market discovered",
			"ticker", m.Ticker,
			"closeTime", closeTime.Format(time.RFC3339),
			"secsUntilClose", int(secsUntilClose),
			"strike", strike,
		)

		// Subscribe to WS orderbook
		if err := e.ws.Subscribe([]string{m.Ticker}); err != nil {
			slog.Warn("ws subscribe failed", "ticker", m.Ticker, "err", err)
		} else {
			ms.Subscribed = true
		}
	}
}

func (e *Engine) processMarket(ctx context.Context, ms *MarketState) {
	secsUntilClose := time.Until(ms.CloseTime).Seconds()

	// Fetch strike if not yet fetched (every 10s)
	if !ms.StrikeFetched && time.Since(ms.LastStrikePoll) > 10*time.Second {
		ms.LastStrikePoll = time.Now()
		if m, err := e.client.GetMarket(ctx, ms.Ticker); err == nil {
			strike := m.StrikePrice()
			if strike > 0 {
				ms.Strike = strike
				ms.StrikeFetched = true
				slog.Info("strike fetched", "ticker", ms.Ticker, "strike", strike)
			}
		}
	}

	// After market closes, poll Kalshi API for settlement result
	// Markets settle ~6 minutes after close
	if secsUntilClose <= 0 && ms.Traded && !ms.Settled {
		e.pollSettlement(ctx, ms)
		return
	}

	// Check pending order status (30s timeout)
	if ms.OrderPending {
		e.checkOrderStatus(ctx, ms)
		return
	}

	// Skip if already evaluated/traded or outside entry window.
	// Per spec: evaluate ONCE when secs_left first enters the window. Don't re-evaluate.
	if ms.Evaluated || ms.Traded || !InEntryWindow(secsUntilClose) {
		return
	}

	// Need strike to evaluate — retry next tick if not yet available
	if !ms.StrikeFetched {
		slog.Warn("evaluation deferred - strike not available", "ticker", ms.Ticker)
		return
	}

	// Get orderbook from WS — retry next tick if not yet available
	ob := e.ws.GetOrderbook(ms.Ticker)
	if ob == nil {
		slog.Warn("evaluation deferred - orderbook not available", "ticker", ms.Ticker)
		return
	}

	yesBid := ob.BestYesBid()
	yesAsk := ob.BestYesAsk()

	if yesBid == 0 || yesAsk == 100 {
		slog.Warn("evaluation deferred - orderbook empty",
			"ticker", ms.Ticker,
			"yesBid", yesBid,
			"yesAsk", yesAsk,
		)
		return
	}

	// All data available — mark evaluated so we don't re-evaluate
	ms.Evaluated = true

	slog.Info("evaluating orderbook",
		"ticker", ms.Ticker,
		"yesBid", yesBid,
		"yesAsk", yesAsk,
		"noAsk", 100-yesBid,
		"secsUntilClose", int(secsUntilClose),
	)

	// Evaluate signal
	sig := Evaluate(yesBid, yesAsk)
	if sig.Side == "" {
		slog.Info("no signal - prices below threshold",
			"ticker", ms.Ticker,
			"yesAsk", yesAsk,
			"noAsk", 100-yesBid,
			"threshold", 80,
		)
		return // no trade
	}

	slog.Info("signal detected",
		"ticker", ms.Ticker,
		"side", sig.Side,
		"limitPrice", sig.LimitPrice,
		"refAsk", sig.RefAsk,
		"secsUntilClose", int(secsUntilClose),
		"strike", ms.Strike,
	)

	// Place order
	e.placeOrder(ctx, ms, sig)
}

func (e *Engine) placeOrder(ctx context.Context, ms *MarketState, sig Signal) {
	contracts := KellySize(sig.LimitPrice, e.balance)
	if contracts == 0 {
		slog.Info("kelly says no trade",
			"ticker", ms.Ticker,
			"side", sig.Side,
			"limitPrice", sig.LimitPrice,
			"refAsk", sig.RefAsk,
			"balance", e.balance,
		)
		return
	}

	fee := TakerFee(contracts, sig.LimitPrice)

	if e.cfg.DryRun {
		// Dry run: simulate immediate fill at limit price
		ms.Traded = true
		ms.Side = sig.Side
		ms.EntryPrice = sig.LimitPrice
		ms.Contracts = contracts
		ms.FeeCents = fee

		_ = e.journal.Log(journal.NewTrade(
			ms.Ticker, sig.Side, "buy",
			sig.LimitPrice, contracts, fee,
			"dry-run", contracts, true, sig.LimitPrice,
		))

		slog.Info("dry-run trade",
			"ticker", ms.Ticker,
			"side", sig.Side,
			"price", sig.LimitPrice,
			"contracts", contracts,
			"fee", fee,
			"balance", e.balance,
		)
		return
	}

	// Real order
	req := kalshi.OrderRequest{
		Ticker:      ms.Ticker,
		Action:      "buy",
		Side:        sig.Side,
		Type:        "limit",
		Count:       contracts,
		TimeInForce: "good_till_canceled",
	}

	if sig.Side == "yes" {
		req.YesPrice = sig.LimitPrice
	} else {
		req.NoPrice = sig.LimitPrice
	}

	order, err := e.client.CreateOrder(ctx, req)
	if err != nil {
		slog.Error("order placement failed", "ticker", ms.Ticker, "err", err)
		return
	}

	ms.OrderPending = true
	ms.OrderID = order.OrderID
	ms.OrderPlacedAt = time.Now()
	ms.Side = sig.Side
	ms.FeeCents = fee

	slog.Info("order placed",
		"ticker", ms.Ticker,
		"orderID", order.OrderID,
		"side", sig.Side,
		"price", sig.LimitPrice,
		"contracts", contracts,
	)
}

func (e *Engine) checkOrderStatus(ctx context.Context, ms *MarketState) {
	elapsed := time.Since(ms.OrderPlacedAt)

	if elapsed < 30*time.Second {
		return // not yet timed out
	}

	// Check fills
	params := url.Values{}
	params.Set("ticker", ms.Ticker)
	params.Set("order_id", ms.OrderID)

	fills, _, err := e.client.GetFills(ctx, params)
	if err != nil {
		slog.Warn("fill check failed", "ticker", ms.Ticker, "err", err)
		return
	}

	totalFilled := 0
	totalCost := 0
	for _, f := range fills {
		totalFilled += f.Count
		if f.Side == "yes" {
			totalCost += f.Count * f.YesPrice
		} else {
			totalCost += f.Count * f.NoPrice
		}
	}

	if totalFilled > 0 {
		// Order filled
		avgPrice := totalCost / totalFilled
		ms.OrderPending = false
		ms.Traded = true
		ms.EntryPrice = avgPrice
		ms.Contracts = totalFilled
		ms.FeeCents = TakerFee(totalFilled, avgPrice)

		_ = e.journal.Log(journal.NewTrade(
			ms.Ticker, ms.Side, "buy",
			avgPrice, totalFilled, ms.FeeCents,
			ms.OrderID, totalFilled, false, avgPrice,
		))

		slog.Info("order filled",
			"ticker", ms.Ticker,
			"side", ms.Side,
			"avgPrice", avgPrice,
			"filled", totalFilled,
		)
	} else {
		// Not filled after 30s — cancel
		if err := e.client.CancelOrder(ctx, ms.OrderID); err != nil {
			slog.Warn("order cancel failed", "ticker", ms.Ticker, "err", err)
		} else {
			slog.Info("order cancelled (unfilled after 30s)", "ticker", ms.Ticker)
		}
		ms.OrderPending = false
	}
}

// pollSettlement checks the Kalshi API for the market's settlement result.
// Markets settle ~6 minutes after close. We poll every 10 seconds until the
// result field is populated ("yes" or "no"), then compute P&L.
func (e *Engine) pollSettlement(ctx context.Context, ms *MarketState) {
	// Rate limit: poll every 10 seconds
	if time.Since(ms.LastSettlementPoll) < 10*time.Second {
		return
	}
	ms.LastSettlementPoll = time.Now()

	// Bail out after 15 minutes of polling (something is wrong)
	if time.Since(ms.CloseTime) > 15*time.Minute {
		slog.Error("settlement timeout — gave up polling after 15 min",
			"ticker", ms.Ticker,
		)
		ms.Settled = true
		e.cleanupMarket(ms)
		return
	}

	m, err := e.client.GetMarket(ctx, ms.Ticker)
	if err != nil {
		slog.Warn("settlement poll failed", "ticker", ms.Ticker, "err", err)
		return
	}

	// Result is empty until Kalshi settles the market
	if m.Result == "" {
		sinceClosed := time.Since(ms.CloseTime).Round(time.Second)
		slog.Debug("awaiting settlement", "ticker", ms.Ticker, "sinceClose", sinceClosed)
		return
	}

	// Market settled — result is "yes" or "no"
	yesResolved := m.Result == "yes"

	var sideWon bool
	if ms.Side == "yes" {
		sideWon = yesResolved
	} else {
		sideWon = !yesResolved
	}

	pnl := ComputePnL(sideWon, ms.EntryPrice, ms.Contracts, ms.FeeCents)
	won := pnl > 0

	if err := e.journal.Log(journal.NewSettlement(
		ms.Ticker, ms.Strike, 0, won, pnl, ms.FeeCents,
		ms.Side, ms.EntryPrice, ms.Contracts, nil, e.cfg.DryRun,
	)); err != nil {
		slog.Error("failed to journal settlement - will retry",
			"ticker", ms.Ticker,
			"err", err,
		)
		return
	}

	slog.Info("settlement",
		"ticker", ms.Ticker,
		"side", ms.Side,
		"result", m.Result,
		"sideWon", sideWon,
		"won", won,
		"pnl", fmt.Sprintf("$%.2f", float64(pnl)/100.0),
		"entry", ms.EntryPrice,
		"contracts", ms.Contracts,
		"waitTime", time.Since(ms.CloseTime).Round(time.Second),
	)

	ms.Settled = true
	e.cleanupMarket(ms)
}

func (e *Engine) cleanupMarket(ms *MarketState) {
	e.ws.Unsubscribe([]string{ms.Ticker})

	e.mu.Lock()
	delete(e.markets, ms.Ticker)
	e.mu.Unlock()
}
