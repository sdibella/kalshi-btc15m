#!/bin/bash
# Cron job to update Bayesian posterior nightly at midnight UTC.
# Reads today's settlements from journal, updates posterior.json.
# Add to crontab with: crontab -e
#   0 0 * * * /home/stefan/Kalshi-BTC15min/scripts/cron_update_posterior.sh >> /home/stefan/Kalshi-BTC15min/posterior_update.log 2>&1

set -e

cd /home/stefan/Kalshi-BTC15min

echo "=== Posterior Update $(date) ==="

python3 scripts/update_posterior.py

# Restart bot to pick up new posterior
echo "Restarting bot to load updated posterior..."
sudo systemctl restart kalshi-btc15m

echo "Done."
