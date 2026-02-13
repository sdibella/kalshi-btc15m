#!/usr/bin/env python3
"""
Update Bayesian posterior with today's trade results.
Run nightly via cron to adapt Kelly sizing based on realized win rate.

Usage: python3 update_posterior.py [--date YYYY-MM-DD]
  If --date not specified, analyzes today's trades.
"""

import json
import sys
import os
from datetime import datetime, timedelta
from pathlib import Path

# Posterior file path (should match bot's location)
POSTERIOR_FILE = os.path.expanduser("~/Kalshi-BTC15min/posterior.json")
JOURNAL_FILE = os.path.expanduser("~/Kalshi-BTC15min/journal.jsonl")

def load_posterior():
    """Load current posterior from file, or return default Beta(83, 3)."""
    if not os.path.exists(POSTERIOR_FILE):
        return {"alpha": 83, "beta": 3}
    try:
        with open(POSTERIOR_FILE) as f:
            return json.load(f)
    except:
        return {"alpha": 83, "beta": 3}

def save_posterior(posterior):
    """Save posterior to file."""
    os.makedirs(os.path.dirname(POSTERIOR_FILE), exist_ok=True)
    with open(POSTERIOR_FILE, "w") as f:
        json.dump(posterior, f, indent=2)
    print(f"Saved posterior to {POSTERIOR_FILE}")

def count_today_trades(date_str=None):
    """
    Count wins/losses from today's trades in journal.jsonl.

    Args:
        date_str: YYYY-MM-DD (default: today)

    Returns:
        (wins, losses)
    """
    if date_str is None:
        date_str = datetime.now().strftime("%Y-%m-%d")

    target_date = datetime.strptime(date_str, "%Y-%m-%d")

    if not os.path.exists(JOURNAL_FILE):
        print(f"Journal file not found: {JOURNAL_FILE}")
        return 0, 0

    wins = 0
    losses = 0

    with open(JOURNAL_FILE) as f:
        for line in f:
            try:
                entry = json.loads(line.strip())
            except:
                continue

            if entry.get("type") != "settlement":
                continue

            # Parse timestamp
            try:
                ts = datetime.fromisoformat(entry["time"].replace("Z", "+00:00"))
                entry_date = ts.strftime("%Y-%m-%d")
            except:
                continue

            if entry_date != date_str:
                continue

            # Count win/loss
            if entry.get("won"):
                wins += 1
            else:
                losses += 1

    return wins, losses

def compute_posterior_stats(alpha, beta):
    """Compute posterior mean and median."""
    mean = alpha / (alpha + beta) if (alpha + beta) > 0 else 0.5
    # Beta median approximation
    median = (alpha - 1/3) / (alpha + beta - 2/3) if (alpha + beta) > 2 else mean
    return {"mean": mean, "median": median}

def main():
    date_str = None
    if len(sys.argv) > 2 and sys.argv[1] == "--date":
        date_str = sys.argv[2]

    # Load current posterior
    posterior = load_posterior()
    alpha, beta = posterior["alpha"], posterior["beta"]

    print(f"Current posterior: Beta({alpha}, {beta})")
    stats = compute_posterior_stats(alpha, beta)
    print(f"  Mean: {stats['mean']:.4f}")
    print(f"  Median: {stats['median']:.4f}")
    print()

    # Count trades
    wins, losses = count_today_trades(date_str)

    if wins == 0 and losses == 0:
        print(f"No settlements found for {date_str or 'today'}")
        return

    print(f"Today's trades: {wins}W / {losses}L ({wins}/{wins+losses} = {wins/(wins+losses)*100:.1f}% WR)")
    print()

    # Update posterior
    new_alpha = alpha + wins
    new_beta = beta + losses

    print(f"Updated posterior: Beta({new_alpha}, {new_beta})")
    new_stats = compute_posterior_stats(new_alpha, new_beta)
    print(f"  Mean: {new_stats['mean']:.4f}")
    print(f"  Median: {new_stats['median']:.4f}")
    print()

    # Save
    posterior["alpha"] = new_alpha
    posterior["beta"] = new_beta
    save_posterior(posterior)

    # Summary
    print(f"Summary:")
    print(f"  Prior: {alpha}W / {beta}L")
    print(f"  Today: {wins}W / {losses}L")
    print(f"  Posterior: {new_alpha}W / {new_beta}L")
    print(f"  WR shifted: {stats['median']:.1%} → {new_stats['median']:.1%}")

if __name__ == "__main__":
    main()
