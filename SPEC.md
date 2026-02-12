# Kalshi BTC 15-Minute Binary Options — Infrastructure Spec

Extracted from KalshiCrypto. This doc covers the three reusable layers: Kalshi API, WebSocket price feeds, and the dashboard. Strategy logic is intentionally excluded — build your own on top.

---

## Strategy Overview (New)

**Buy contracts priced >= 55c (YES or NO) at the 4-minute mark before settlement.**

Rationale: A contract at 55c implies the market believes there's a 55% chance it resolves YES. By buying anything at 55c+, you're riding the market's existing conviction with a 4-minute horizon. No directional prediction needed — you're buying the side the market already favors.

Key parameters:
- Entry trigger: any contract (YES or NO) with price >= 55 cents
- Entry window: exactly 4 minutes before settlement (660s into the 15-min market)
- Position: buy the favored side, hold to settlement
- Risk management: TBD (Kelly, flat sizing, etc.)

---

## 1. Kalshi API

### Authentication (RSA-PSS)

Every request is signed with your RSA private key. No session tokens, no OAuth — just per-request signatures.

**Key loading** supports both PKCS8 and PKCS1 PEM formats:
```go
func LoadPrivateKey(path string) (*rsa.PrivateKey, error)
// 1. Read PEM file
// 2. Try x509.ParsePKCS8PrivateKey (standard)
// 3. Fallback: x509.ParsePKCS1PrivateKey (legacy RSA)
```

**Signing algorithm:**
```
message = timestampMS + method + fullPath
// e.g. "1707692400000GET/trade-api/v2/portfolio/balance"

hash    = SHA-256(message)
sig     = RSA-PSS-Sign(privateKey, hash, saltLen=hashLen)
encoded = Base64(sig)
```

**Headers on every request:**
```
KALSHI-ACCESS-KEY:       <api_key_id>
KALSHI-ACCESS-TIMESTAMP: <unix_millis>
KALSHI-ACCESS-SIGNATURE: <base64_sig>
```

### Base URLs

| Environment | REST | WebSocket |
|---|---|---|
| Production | `https://api.elections.kalshi.com/trade-api/v2` | `wss://api.elections.kalshi.com/trade-api/ws/v2` |
| Demo | `https://demo-api.kalshi.co/trade-api/v2` | `wss://demo-api.kalshi.co/trade-api/ws/v2` |

### REST Endpoints

#### GET /markets/{ticker}
Returns a single market. **Always use this for KXBTC15M** — the list endpoint returns stale strike data.

```json
{
  "market": {
    "ticker": "KXBTC15M-260211-1630-98000",
    "event_ticker": "KXBTC15M",
    "title": "...",
    "status": "active|closed|settled",
    "yes_bid": 55, "yes_ask": 58,
    "no_bid": 42, "no_ask": 45,
    "last_price": 56,
    "volume": 1200,
    "open_interest": 340,
    "floor_strike": 98000.0,
    "cap_strike": 98500.0,
    "close_time": "2026-02-11T16:30:00Z",
    "open_time": "2026-02-11T16:15:00Z",
    "expiration_time": "2026-02-11T16:30:00Z",
    "expected_expiration_time": "2026-02-11T16:30:00Z",
    "result": "yes|no",
    "rules_primary": "...",
    "subtitle": "...",
    "yes_sub_title": "...",
    "no_sub_title": "...",
    "custom_strike": "..."
  }
}
```

#### GET /markets?series_ticker=KXBTC15M&status=active&limit=200
Returns list of markets. **WARNING**: `rules_primary` / strike may be stale. Paginated via `cursor`.

#### GET /markets/{ticker}/orderbook?depth=N
```json
{
  "orderbook": {
    "ticker": "...",
    "yes": [[55, 100], [54, 200]],
    "no": [[45, 150], [44, 300]]
  }
}
```
Price levels: `[[price_cents, quantity], ...]` sorted best-first (highest price first).

#### GET /portfolio/balance
```json
{ "balance": 35537 }
```
Balance in cents.

#### GET /portfolio/positions?event_ticker=KXBTC15M&limit=200
```json
{
  "market_positions": [{
    "ticker": "...",
    "position": 10,
    "market_exposure": 500,
    "resting_orders_count": 0,
    "total_traded": 15,
    "realized_pnl": 200
  }]
}
```
`position`: positive = YES contracts, negative = NO contracts.

#### POST /portfolio/orders
```json
{
  "ticker": "KXBTC15M-260211-1630-98000",
  "action": "buy",
  "side": "yes",
  "type": "limit",
  "count": 10,
  "yes_price": 55,
  "time_in_force": "ioc"
}
```
Response wraps in `{"order": {...}}` with `order_id`, `status`, `filled_count`, `remaining_count`.

Time-in-force options: `"gtc"` (good til cancelled), `"ioc"` (immediate or cancel), `"fok"` (fill or kill).

#### DELETE /portfolio/orders/{order_id}
Cancels a resting order. No response body.

#### GET /portfolio/fills?limit=N&cursor=...
```json
{
  "fills": [{
    "fill_id": "...",
    "order_id": "...",
    "ticker": "...",
    "side": "yes",
    "action": "buy",
    "count": 10,
    "yes_price": 55,
    "no_price": 45,
    "is_taker": true,
    "created_time": "2026-02-11T16:26:00.123456789Z"
  }],
  "cursor": "next_page_token"
}
```

### Rate Limits
- **Reads**: 20/sec
- **Writes**: 10/sec
- No built-in retry/backoff in the original client — implement your own.

### Taker Fee Formula
```
fee = ceil(0.07 * contracts * P * (1 - P)) cents
```
Where P = price / 100. Maximum fee is at P=0.50 (~1.75c/contract). At P=0.55: ~1.73c/contract.

### Ticker Format
```
KXBTC15M-{YYMMDD}{HHMM}-{strike}
```
- `HHMM` = market **close** time (UTC)
- `strike` = BTC price level (integer)
- Example: `KXBTC15M-260211-1630-98000` closes at 16:30 UTC on Feb 11, strike $98,000

### Kalshi Orderbook WebSocket

Connect to `wss://api.elections.kalshi.com/trade-api/ws/v2` with auth headers in the HTTP upgrade.

**Subscribe:**
```json
{
  "id": 1,
  "cmd": "subscribe",
  "params": {
    "channels": ["orderbook_delta"],
    "market_tickers": ["KXBTC15M-260211-1630-98000"]
  }
}
```

**Messages received:**

Snapshot (full book):
```json
{
  "type": "orderbook_snapshot",
  "msg": {
    "market_ticker": "...",
    "yes": [[55, 100], [54, 200]],
    "no": [[45, 150]]
  }
}
```

Delta (incremental):
```json
{
  "type": "orderbook_delta",
  "msg": {
    "market_ticker": "...",
    "price": 55,
    "delta": -50,
    "side": "yes"
  }
}
```
Delta is additive: `new_qty = old_qty + delta`. Remove level if qty <= 0.

**Connection management:**
- 30-second read deadline (expect heartbeats/data within this window)
- Auto-reconnect with 2-second delay
- Track subscribed tickers; re-subscribe on reconnect

---

## 2. WebSocket Price Feeds

Four exchange feeds provide real-time BTC prices. The BRTI proxy computes a median for use as the settlement price estimator.

### Exchange Implementations

#### Binance (primary — highest liquidity)
- **URL**: `wss://stream.binance.us:9443/ws/btcusdt@bookTicker`
- **Pair**: BTC-USDT
- **No subscription message needed** — stream is in the URL
- **Read deadline**: 10 seconds
- **Message format**:
```json
{ "b": "97500.50", "a": "97501.00" }
```
Mid = (bid + ask) / 2. Prices are strings.

#### Coinbase
- **URL**: `wss://ws-feed.exchange.coinbase.com`
- **Pair**: BTC-USD
- **Subscribe**:
```json
{ "type": "subscribe", "product_ids": ["BTC-USD"], "channels": ["ticker"] }
```
- **Read deadline**: 10 seconds
- **Message format** (filter `type == "ticker"`):
```json
{ "type": "ticker", "best_bid": "97500.50", "best_ask": "97501.00", "product_id": "BTC-USD" }
```

#### Kraken
- **URL**: `wss://ws.kraken.com/v2`
- **Pair**: BTC/USD
- **Subscribe**:
```json
{ "method": "subscribe", "params": { "channel": "ticker", "symbol": ["BTC/USD"] } }
```
- **Read deadline**: 30 seconds
- **Message format** (filter `channel == "ticker"` and `type == "update"`):
```json
{ "channel": "ticker", "type": "update", "data": [{ "bid": 97500.50, "ask": 97501.00 }] }
```
**NOTE**: Kraken v2 sends bid/ask as **numeric floats**, not strings.

#### Bitstamp
- **URL**: `wss://ws.bitstamp.net`
- **Pair**: BTC-USD
- **Subscribe**:
```json
{ "event": "bts:subscribe", "data": { "channel": "order_book_btcusd" } }
```
- **Read deadline**: 30 seconds
- **Message format** (data messages have **empty** event field):
```json
{ "event": "", "channel": "order_book_btcusd", "data": { "bids": [["97500.50", "1.5"]], "asks": [["97501.00", "2.0"]] } }
```
Skip events: `bts:subscription_succeeded`, `bts:request_reconnect`. Empty event = data.

### BRTI Proxy (Price Aggregation)

```go
type BRTIProxy struct {
    feeds        []ExchangeFeed
    price        float64
    lastUpdate   time.Time
    priceHistory []TimedPrice  // 900-sample ring buffer (15 min at 1/sec)
    settlementTicks []float64  // 60 values during final minute
    sampling     bool
}
```

**Snapshot()**: Collects mid-prices from all non-stale feeds, returns **median**.
- Staleness threshold: **5 seconds** since last update
- Falls back to cached price if all feeds stale

**RecordSample()**: Called every 1 second. Writes to ring buffer for historical price access.

**PriceHistory(n)**: Returns last N prices (chronological) from ring buffer. Used for vol/momentum.

**Settlement window**: During final 60 seconds, `RecordSettlementTick()` captures per-second prices. `SettlementAverage()` = simple mean of all ticks. This is your BRTI proxy for determining settlement.

### ExchangeFeed Interface

```go
type ExchangeFeed interface {
    Name() string
    Run(ctx context.Context) error
    MidPrice() float64
    LastUpdate() time.Time
    IsStale() bool  // >5s since last update
}
```

### Reconnection Pattern (all feeds)

```
loop forever:
    err = connect(ctx, wsURL)
    if err: log warning
    if ctx.Done: return
    sleep 2 seconds
    log "reconnecting..."
```

### Price Validation
Rejects NaN, zero, and negative prices before updating state. All updates are mutex-protected.

---

## 3. Dashboard

Real-time web UI for monitoring trades and performance. Reads JSONL journal files, renders server-side HTML via HTMX.

### Stack
- **Backend**: Go HTTP server, `html/template`
- **Frontend**: HTMX 1.9.10 (polling), TailwindCSS (dark theme), Chart.js 4.4.1 (equity curve)
- **Data source**: JSONL journal files (append-only event log)
- **Deployment**: Tailscale Serve → localhost:8081

### Routes

| Route | Method | Returns | Poll interval |
|---|---|---|---|
| `/` | GET | Full HTML page | — |
| `/api/summary` | GET | HTML fragment (6 metric cards) | 3s |
| `/api/trades` | GET | HTML table (last 50 trades) | 5s |
| `/api/equity` | GET | JSON array of `{Time, BalanceCents}` | 5s |
| `/api/performance` | GET | HTML fragment (by-side, by-edge tables) | 5s |

**Query params**: `?mode=all|latest` (session scope), `?trading=paper|live` (filter by dry_run).

### Journal Event Types

The bot writes these JSONL events. The dashboard reads and aggregates them.

#### session_start
```json
{
  "type": "session_start",
  "time": "2026-02-11T01:15:46Z",
  "mode": "full",
  "dry_run": true,
  "env": "prod",
  "balance_cents": 35537
}
```

#### trade
```json
{
  "type": "trade",
  "time": "2026-02-11T01:20:00Z",
  "mode": "full",
  "ticker": "KXBTC15M-260211-1630-98000",
  "side": "YES",
  "action": "buy",
  "price": 55,
  "quantity": 10,
  "fee_cents": 17,
  "edge_pct": 3.5,
  "true_prob": 0.58,
  "kelly": 0.12,
  "order_id": "abc123",
  "filled": 10,
  "dry_run": true,
  "limit_price": 56,
  "available_depth": 200
}
```

#### settlement
```json
{
  "type": "settlement",
  "time": "2026-02-11T01:30:05Z",
  "mode": "full",
  "ticker": "KXBTC15M-260211-1630-98000",
  "strike": 98000.0,
  "avg_brti": 98050.5,
  "won": true,
  "pnl_cents": 450,
  "fee_cents": 17,
  "trades": 1,
  "net_position": 10,
  "settlement_ticks": [98050.1, 98050.5, 98050.9]
}
```
**Critical**: `won` = `pnl > 0`, NOT `avg_brti >= strike`.

### Dashboard Data Models

```go
type Summary struct {
    BalanceCents    int
    TotalPnL        int
    WinCount, LossCount int
    WinRate         float64
    TotalFees       int
    ROI             float64
    AvgEdge         float64
    TotalMarkets    int
}

type TradeView struct {
    Time     string
    Ticker   string
    Side     string   // "YES" / "NO"
    Quantity int
    AvgPrice float64  // quantity-weighted
    EdgePct  float64
    Result   string   // "win" / "loss" / "open"
    PnL      int
    Fees     int
}

type EquityPoint struct {
    Time         time.Time
    BalanceCents int
}
```

### Analytics

**Trade aggregation**: Multiple fills for the same ticker are merged into one `TradeView` with weighted averages.

**Balance tracking**: `current_balance = last_session_start_balance + sum(settlement_pnl)`. Handles bot restarts gracefully since `session_start` re-queries Kalshi API.

**Performance breakdown**: By side (YES/NO), by edge range (0-2%, 2-4%, ..., 10%+). Hour-of-day and phase breakdowns computed but not yet displayed in templates.

### Deployment

```bash
# Build
go build -o dashboard ./cmd/dashboard

# Configure Tailscale proxy (tailnet-only HTTPS)
tailscale serve --bg 8081

# Start
DASHBOARD_PORT=8081 DASHBOARD_JOURNAL_DIR=./journal ./dashboard
```

Accessible at `https://<hostname>.<tailnet>.ts.net:8081` from any device on your tailnet.

---

## 4. Configuration

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `KALSHI_API_KEY_ID` | (required) | Your Kalshi API key ID |
| `KALSHI_PRIV_KEY_PATH` | `./kalshi_private_key.pem` | Path to RSA private key |
| `KALSHI_ENV` | `prod` | `prod` or `demo` |
| `DRY_RUN` | `true` | Paper trading (log orders, don't submit) |
| `KELLY_FRACTION` | `0.25` | Kelly sizing multiplier |
| `MIN_EDGE_PCT` | `2.0` | Minimum edge to trade |
| `JOURNAL_PATH` | `./journal.jsonl` | Where to write journal events |
| `DASHBOARD_PORT` | `8080` | Dashboard HTTP port |
| `DASHBOARD_HOST` | `localhost` | Dashboard bind address |
| `DASHBOARD_JOURNAL_DIR` | `~/.kalshi-bot` | Where to find journal files |

### Config Loading

Uses `github.com/joho/godotenv` to load `.env` file, then reads env vars with typed defaults:
```go
func Load() (*Config, error)
// Validates: KALSHI_API_KEY_ID required, KALSHI_ENV in {prod, demo}
```

---

## 5. Known Pitfalls

1. **Stale strikes from list endpoint**: NEVER trust `GET /markets` `rules_primary` for KXBTC15M. Always fetch individual market via `GET /markets/{ticker}`.
2. **Kraken v2 sends floats**: `bid`/`ask` are numeric, not strings. Use `float64`, not `json.Number`.
3. **Bitstamp empty events**: Data messages have `event: ""`. Don't filter them out.
4. **Settlement `won` field**: Must be `pnl > 0`, not `avg_brti >= strike`.
5. **Settlement tick counting**: Record once per engine cycle, not once per market.
6. **YES + NO net to zero**: You cannot hold both sides simultaneously. Buying the opposite side closes your position.
7. **Fee at P=0.55**: `ceil(0.07 * 55 * 45 / 100)` = `ceil(1.7325)` = **2 cents/contract**.
