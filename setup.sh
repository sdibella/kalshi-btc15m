#!/bin/bash
set -euo pipefail

# Setup script for Kalshi-BTC15min
# Copies credentials and config from KalshiCrypto into this project

SOURCE="/Users/gw/Workspace/tradebots/KalshiCrypto"
DEST="$(cd "$(dirname "$0")" && pwd)"

echo "Setting up Kalshi-BTC15min..."
echo "  Source: $SOURCE"
echo "  Dest:   $DEST"

# Copy private key (symlink to avoid duplication)
if [ -f "$SOURCE/kalshi_private_key.pem" ]; then
    ln -sf "$SOURCE/kalshi_private_key.pem" "$DEST/kalshi_private_key.pem"
    echo "  Linked kalshi_private_key.pem"
else
    echo "  WARNING: $SOURCE/kalshi_private_key.pem not found"
fi

# Copy .env (actual copy so you can customize without affecting the other bot)
if [ -f "$SOURCE/.env" ]; then
    cp "$SOURCE/.env" "$DEST/.env"
    echo "  Copied .env"

    # Override defaults for this strategy
    # Force DRY_RUN=true for safety
    if grep -q "^DRY_RUN=" "$DEST/.env"; then
        sed -i '' 's/^DRY_RUN=.*/DRY_RUN=true/' "$DEST/.env"
    else
        echo "DRY_RUN=true" >> "$DEST/.env"
    fi

    # Set a separate journal path
    if grep -q "^JOURNAL_PATH=" "$DEST/.env"; then
        sed -i '' "s|^JOURNAL_PATH=.*|JOURNAL_PATH=./journal.jsonl|" "$DEST/.env"
    else
        echo "JOURNAL_PATH=./journal.jsonl" >> "$DEST/.env"
    fi

    echo "  Set DRY_RUN=true and JOURNAL_PATH=./journal.jsonl"
else
    echo "  WARNING: $SOURCE/.env not found"
fi

echo ""
echo "Done. Review .env before running:"
echo "  cat $DEST/.env"
