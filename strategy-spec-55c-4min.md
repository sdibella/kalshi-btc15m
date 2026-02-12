# Strategy Spec: 55c @ 4min with Limit Orders

## Overview

Buy whichever side (YES or NO) the market favors at 4 minutes before close, using limit orders at the bid. Hold to settlement.

**Backtest (1 day, 31 trades):** 93.5% WR, +$2.71/day after fees with limit entry.

---

## Timing

Markets run on 15-minute windows: `:00-:15`, `:15-:30`, `:30-:45`, `:45-:00`.

```
secs_left counts down to SETTLEMENT, not to market close.

  secs_left ≈ 534  →  4 min before close (ENTRY WINDOW OPENS)
  secs_left ≈ 294  →  market closes (trading stops)
  secs_left ≈ 0    →  settlement

  Formula: target_secs_left = 294 + (minutes_before_close × 60)
  Entry:   secs_left ≤ 534 AND secs_left > 294
```

**Entry window:** Evaluate once when `secs_left` first crosses below 534. Do not re-evaluate or chase.

---

## Entry Signal

For each market in the current 15-minute window:

```
yes_ask = market.yes_ask
no_ask  = 100 - market.yes_bid

if yes_ask >= 55:
    side  = YES
    price = yes_ask          # reference for signal
    limit = market.yes_bid   # limit order at bid
elif no_ask >= 55:
    side  = NO
    price = no_ask           # reference for signal
    limit = 100 - yes_ask    # limit order at bid (no_bid = 100 - yes_ask)
else:
    NO TRADE (skip this market)
```

**One trade per market.** If multiple strikes qualify in the same window, trade each independently.

---

## Order Placement

Place a **limit order at the bid**, not a market order at the ask.

```
spread   = ask - bid          # typically 1-2c
savings  = spread per trade   # avg 1.8c observed
```

If the limit order doesn't fill within 30 seconds, cancel it. Do not chase with a market order — the edge diminishes if you pay the spread.

---

## Fee Calculation

Kalshi charges on entry only (settlement is free):

```
fee = 0.07 × min(entry_price, 100 - entry_price)
```

Examples at common entry prices:

| Entry | Fee   | Max Profit (if win) | Max Loss (if lose) |
|-------|-------|---------------------|--------------------|
| 55c   | 3.15c | 41.85c              | 58.15c             |
| 65c   | 2.45c | 32.55c              | 67.45c             |
| 75c   | 1.75c | 23.25c              | 76.75c             |
| 85c   | 1.05c | 13.95c              | 86.05c             |
| 95c   | 0.35c | 4.65c               | 95.35c             |

---

## Position Sizing (Kelly Criterion)

```
kelly_fraction = p - (q / b)

where:
    p = assumed win rate
    q = 1 - p
    b = win_profit / loss_amount
    win_profit  = 100 - entry_price - fee
    loss_amount = entry_price + fee
```

**Use Quarter Kelly** (multiply fraction by 0.25) for safety.

```
contracts = floor(quarter_kelly × bankroll / cost_per_contract)
cost_per_contract = entry_price + fee

# Minimum 1 contract, maximum capped by liquidity
contracts = max(1, min(contracts, liquidity_cap))
```

Kelly fractions at different assumed win rates (avg entry ~80c):

| Win Rate | Full Kelly | Quarter Kelly | Sizing on $500 bankroll |
|----------|-----------|---------------|-------------------------|
| 85%      | ~0%       | ~0%           | 1 contract (minimum)    |
| 90%      | ~18%      | ~4.5%         | ~2-3 contracts          |
| 95%      | ~59%      | ~14.8%        | ~8-10 contracts         |

**Important:** Until you have 100+ trades confirming the actual win rate, use 1 contract per trade (flat bet). Kelly with an inaccurate win rate estimate is worse than flat betting.

---

## Liquidity Cap

Observed Kalshi BTC 15-min volumes:

- 1 big market per window: $7,880-$25,000
- Other strikes: $200-$2,000

Conservative max fill per trade: **$50-$300** (avoid taking >15% of the book).

```
max_contracts = floor(max_fill_dollars × 100 / cost_per_contract)
contracts     = min(kelly_contracts, max_contracts)
```

---

## Exit

**Hold to settlement. No early exit.**

Backtested: exiting at 2min or 1min before close reduces P&L by $1.19-$1.88/day. The edge is realized at settlement.

---

## P&L Calculation

```
if side == settlement_result:
    pnl = (100 - entry_price - fee) × contracts     # WIN
else:
    pnl = -(entry_price + fee) × contracts           # LOSS
```

---

## Risk Parameters

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Min entry price | 55c | Below 55c, market is too uncertain |
| Entry window | 4 min before close | 5min adds 3 extra losses; 3min has fewer opportunities |
| Order type | Limit at bid | Saves avg 1.8c/trade vs market orders |
| Fill timeout | 30 seconds | Cancel unfilled limits, don't chase |
| Sizing | Quarter Kelly (or flat 1 contract) | Zero bust risk observed |
| Max position | 15% of visible book depth | Avoid moving the market |
| Hold duration | To settlement | Early exit tested, loses money |

---

## Expected Performance

Flat bet (1 contract per trade):

| Win Rate | EV/trade | Trades/day | Daily EV | Monthly (22d) |
|----------|----------|-----------|----------|---------------|
| 85%      | -0.55c   | ~31       | -$0.17   | -$3.76        |
| 90%      | +3.88c   | ~31       | +$1.20   | +$26.44       |
| 95%      | +8.30c   | ~31       | +$2.57   | +$56.63       |

**Break-even win rate: ~88%.** Below this, the strategy loses money after fees.

---

## Paper Trade Tracking

Log per trade:

```json
{
  "ticker": "KXBTC15M-...",
  "ts": "2026-02-10T17:30:00Z",
  "side": "YES",
  "signal_price": 74,
  "limit_price": 71,
  "fill_price": 71,
  "filled": true,
  "fee": 2.03,
  "contracts": 1,
  "settlement": "YES",
  "pnl": 26.97,
  "bankroll_after": 50027,
  "secs_left_at_entry": 530,
  "brti": 69013.04,
  "strike": 68931.13,
  "distance": 81.91
}
```

Key metrics to track on dashboard:
- Running win rate (need 100+ trades for confidence)
- Cumulative P&L
- Fill rate on limit orders (if <70%, spreads may be too tight)
- Avg entry price (should be ~80c weighted average)
- Max drawdown
