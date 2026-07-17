#!/usr/bin/env bash
#
# Regenerates internal/db/sp500_tickers.txt from the datasets/
# s-and-p-500-companies CSV (itself a continuous sync of Wikipedia's S&P 500
# constituents list) — see docs/phase-2.6-universe-refresh.md for why this
# source was chosen over scraping Wikipedia directly or Finnhub's
# premium-only /index/constituents.
#
# Run this by hand, roughly monthly. The resulting diff goes through a
# normal PR/CI like any other code change — there is no runtime network
# dependency introduced in the bot itself. On the next merge to main, the
# daily-scheduled deploy restarts the bot process, which runs
# db.SyncSP500() once at startup to reconcile the new list against the
# universe table (see bot.SyncUniverse).
set -euo pipefail

SOURCE_URL="https://raw.githubusercontent.com/datasets/s-and-p-500-companies/main/data/constituents.csv"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_FILE="$SCRIPT_DIR/../internal/db/sp500_tickers.txt"

tmp_csv="$(mktemp)"
tmp_tickers="$(mktemp)"
trap 'rm -f "$tmp_csv" "$tmp_tickers"' EXIT

echo "Fetching $SOURCE_URL ..."
curl -sS --fail --max-time 30 "$SOURCE_URL" -o "$tmp_csv"

# Symbol is always the first CSV column and tickers never contain a comma,
# so splitting on the first field is safe even though later columns (e.g.
# Headquarters Location) are quoted and do contain commas.
tail -n +2 "$tmp_csv" | awk -F, '{print $1}' | sort > "$tmp_tickers"

count=$(wc -l < "$tmp_tickers" | tr -d ' ')
if [ "$count" -lt 480 ] || [ "$count" -gt 520 ]; then
  echo "error: got $count tickers, expected 480-520 — source format may have changed, refusing to overwrite $OUT_FILE" >&2
  exit 1
fi

if bad=$(grep -vE '^[A-Z][A-Z0-9.-]{0,6}$' "$tmp_tickers"); then
  echo "error: ticker(s) with unexpected shape:" >&2
  echo "$bad" >&2
  echo "source format may have changed, refusing to overwrite $OUT_FILE" >&2
  exit 1
fi

cp "$tmp_tickers" "$OUT_FILE"
echo "Wrote $count tickers to $OUT_FILE"
