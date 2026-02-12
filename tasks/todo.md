# Kalshi BTC15M Trading Bot - Build Plan

## Phase 1: Skeleton & Infrastructure
- [x] Create go.mod, .env.example, .gitignore
- [x] Copy kalshi/auth.go (import path change)
- [x] Copy kalshi/client.go (import path + DELETE signing fix)
- [x] Copy kalshi/ws.go (import path change)
- [x] Copy feed/ files (binance, coinbase, kraken, bitstamp, feed.go)
- [x] Simplify config/config.go (removed Mode, Kelly, risk fields)
- [x] Simplify journal/journal.go (3 event types: SessionStart, Trade, Settlement)
- [x] Write cmd/bot/main.go
- [x] `go build ./cmd/bot` compiles
- [x] `go vet ./...` passes

## Phase 2: Strategy Core - Signal & Entry
- [x] Implement Evaluate() - 55c signal logic
- [x] Implement InEntryWindow() - 294 < secsLeft <= 534
- [x] Implement TakerFee() - ceil(0.07 * contracts * P * (1-P) * 100)
- [x] Implement Engine.Run() - 1s ticker + 60s balance refresh
- [x] Implement Engine.tick() - discover + process markets
- [x] Implement Engine.discoverMarkets() - poll KXBTC15M series
- [x] Unit tests for Evaluate, InEntryWindow, TakerFee, ComputePnL (25 tests, all pass)

## Phase 3: Order Management
- [x] Implement placeOrder() - limit order with gtc, dry-run simulation
- [x] Implement checkOrderStatus() - 30s timeout, fill check, cancel
- [x] Dry-run mode skips API calls, simulates immediate fill

## Phase 4: Settlement & P&L
- [x] Implement settleMarket() - BRTI settlement average
- [x] ComputePnL() - win = (100-entry)*contracts - fee, loss = -(entry*contracts + fee)
- [x] won field = pnl > 0 (per spec)
- [x] Settlement tick recording during final 60s
- [x] Unsubscribe WS + cleanup on settlement

## Phase 5: Dashboard
- [x] Copy dashboard/config.go
- [x] Simplify dashboard/types.go (removed EdgeRangeStats, AvgEdge)
- [x] Simplify dashboard/reader.go (removed Signal/MarketDiscovered/SessionEnd)
- [x] Simplify dashboard/analytics.go (removed edge/kelly aggregation)
- [x] Write cmd/dashboard/main.go
- [x] Write templates (index, summary, trades, performance)
- [x] Remove "Avg Edge" card from summary
- [x] Remove Edge % column from trades
- [x] Remove "by edge range" table from performance
- [x] `go build ./cmd/dashboard` compiles

## Phase 6: Verification
- [x] `go build ./...` compiles (both binaries)
- [x] `go vet ./...` passes (no issues)
- [x] `go test ./...` passes (25/25 tests)

## Review
All 6 phases implemented. Key changes from KalshiCrypto:
- **DELETE signing bug fixed** in client.go (now uses `c.signPath(path)`)
- **Config simplified**: removed Mode, Kelly, risk management fields
- **Journal simplified**: 3 event types (SessionStart without Mode, Trade without edge/kelly, Settlement with side/entry/contracts)
- **Strategy written fresh**: ~320 lines, focused 55c signal + limit order lifecycle
- **Dashboard simplified**: removed edge-range breakdown, avg edge card, edge column
