package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"

	"argus/internal/data"
)

// earnings_cache.go is Phase 25 §8.1 step 3's offline (ticker, filed_date)
// cache, same two-step pattern as history_cache.go's -build-history:
// -earnings-dates pays SEC EDGAR's per-ticker fetch once, every later run
// reads the CSV and touches no network — needed because post_gap_drift_
// confirmed gets re-evaluated across every time-slice/universe combination a
// study run sweeps, and SEC's 200ms-throttled API would make that as
// expensive as the history fetch it already avoids re-paying.

// buildEarningsDatesCache calls SEC EDGAR's GetFilingDates for every ticker
// and writes the (ticker, filed date) pairs to path. US-only — SEC has no TW
// equivalent (see data.SEC.GetFilingDates's doc comment).
func buildEarningsDatesCache(sec *data.SEC, tickers []string, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"Ticker", "FiledDate"}); err != nil {
		return err
	}

	var ok, failed, rows int
	for i, ticker := range tickers {
		dates, err := sec.GetFilingDates(ticker)
		if err != nil {
			// One ticker's CIK/EDGAR miss must not cost the whole build —
			// same accounting convention as buildHistoryCacheYahoo.
			fmt.Printf("  %s: %v (skipped)\n", ticker, err)
			failed++
			continue
		}
		for _, d := range dates {
			if err := w.Write([]string{ticker, d}); err != nil {
				return err
			}
			rows++
		}
		ok++
		if (i+1)%50 == 0 {
			fmt.Printf("  cached %d/%d tickers, %d rows...\n", i+1, len(tickers), rows)
			w.Flush()
		}
	}
	w.Flush()
	fmt.Printf("Earnings-dates cache built: %s — %d tickers ok, %d failed, %d rows\n", path, ok, failed, rows)
	return w.Error()
}

// loadEarningsDatesCache reads a cache written by buildEarningsDatesCache
// into a per-ticker set of filed dates (YYYY-MM-DD), for the O(1) membership
// check nearFilingDate needs on every evaluated day.
func loadEarningsDatesCache(path string) (map[string]map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil { // header
		return nil, fmt.Errorf("reading header: %w", err)
	}
	out := make(map[string]map[string]bool)
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		ticker, date := rec[0], rec[1]
		if out[ticker] == nil {
			out[ticker] = make(map[string]bool)
		}
		out[ticker][date] = true
	}
	return out, nil
}

// nearFilingDate reports whether candles[t]'s trading day — or the trading
// day immediately before or after it — is one of ticker's SEC filed dates.
// Phase 25 §8.1.5: a companyfacts "filed" date is the SEC submission date,
// not the announcement moment — an after-hours earnings report filed the
// next morning lands one trading day after the gap it actually caused — so
// this is deliberately a ±1 trading day window, not exact-day equality. dates
// may be nil (ticker missing from the cache, e.g. a fetch failure during the
// build): map reads on a nil map return false, so this degrades to "never
// confirmed" rather than panicking.
func nearFilingDate(dates map[string]bool, candles []data.Candle, t int) bool {
	if dates[candles[t].Date.Format("2006-01-02")] {
		return true
	}
	if t > 0 && dates[candles[t-1].Date.Format("2006-01-02")] {
		return true
	}
	if t+1 < len(candles) && dates[candles[t+1].Date.Format("2006-01-02")] {
		return true
	}
	return false
}
