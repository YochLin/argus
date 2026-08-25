package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"argus/internal/data"
	"argus/internal/sinopac"
)

// A TW study driven off Yahoo has two problems this cache exists to fix, and
// only one of them is speed:
//
//   - Speed. Yahoo is one request per ticker with a 200ms rate limit, so a
//     118-ticker TW run spends ~15 minutes just fetching, every run. That is
//     affordable once and prohibitive for a parameter grid, which is exactly
//     what the exit layer needs next.
//   - Survivorship bias. tw150_tickers.txt is TODAY's listing. Screening it
//     backwards silently drops everything that delisted, merged, or changed
//     code in between — docs/phase-23 §6 called fixing this too expensive and
//     deferred it. Sinopac's daily_quotes is point-in-time (see
//     sinopac.DailyQuote), so walking it forward costs nothing extra and the
//     bias is simply gone.
//
// The cache is a flat CSV of every (date, ticker) bar, built once and reused
// by every subsequent run. ~2,450 trading days x ~1,800 codes is ~4.4M rows,
// which is large for a file and unremarkable for a one-off offline study.
//
// ponytail: plain CSV, no compression or index. Gzip it if 200MB ever
// actually hurts.

// buildHistoryCache walks every calendar day in [from, to], asks the Shioaji
// daemon for that day's whole-market quotes, and writes them to path.
// Non-trading days come back empty and are skipped, which is why this walks
// calendar days rather than needing a trading calendar of its own.
func buildHistoryCache(ctx context.Context, c *sinopac.Client, from, to time.Time, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"Date", "Code", "Open", "High", "Low", "Close", "Volume"}); err != nil {
		return err
	}

	var days, tradingDays, rows int
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		days++
		date := d.Format("2006-01-02")
		// A build is thousands of sequential requests over ten-plus minutes,
		// so one slow response must not discard all of it — the daemon's
		// own client only retries a dropped Solace session, and a plain
		// 10s timeout (live-observed once mid-build, with the same date
		// answering fine seconds later) is not that.
		var quotes []sinopac.DailyQuote
		var err error
		for attempt := 0; attempt < 4; attempt++ {
			if quotes, err = c.DailyQuotes(ctx, date); err == nil {
				break
			}
			fmt.Printf("  %s: %v (retry %d/3)\n", date, err, attempt+1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
			}
		}
		if err != nil {
			return fmt.Errorf("daily_quotes %s: %w", date, err)
		}
		if len(quotes) == 0 {
			continue
		}
		tradingDays++
		for _, q := range quotes {
			if q.Close <= 0 {
				continue
			}
			if err := w.Write([]string{
				q.Date, q.Code,
				strconv.FormatFloat(q.Open, 'f', 4, 64),
				strconv.FormatFloat(q.High, 'f', 4, 64),
				strconv.FormatFloat(q.Low, 'f', 4, 64),
				strconv.FormatFloat(q.Close, 'f', 4, 64),
				strconv.FormatInt(q.Volume, 10),
			}); err != nil {
				return err
			}
			rows++
		}
		if tradingDays%100 == 0 {
			fmt.Printf("  cached %d trading days (%s), %d rows...\n", tradingDays, date, rows)
			w.Flush()
		}
	}
	w.Flush()
	fmt.Printf("Cache built: %s — %d calendar days walked, %d trading days, %d rows\n", path, days, tradingDays, rows)
	return w.Error()
}

// isOrdinaryEquity keeps 4-digit listed equities (1000-9999) and drops
// everything else the whole-market feed carries: ETFs and their leveraged
// variants (00xx, and the 5-6 character codes with a B/L/R/U suffix),
// warrants, and TDRs. The screens' own liquidity gate would filter most of
// this anyway, but the BASELINE would not — and a control group padded with
// microcap ETF noise is not the control the strategy numbers need to be read
// against.
func isOrdinaryEquity(code string) bool {
	if len(code) != 4 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return code[0] != '0'
}

// loadHistoryCache reads a cache written by buildHistoryCache into per-ticker
// candle series, oldest first. Tickers with fewer than minBars rows are
// dropped, matching the "too short" accounting the Yahoo path already does.
// keep lists codes to load regardless of isOrdinaryEquity — in practice the
// benchmark, which is an ETF (0050) and would otherwise be filtered out of
// its own study.
func loadHistoryCache(path string, minBars int, keep ...string) (map[string][]data.Candle, error) {
	keepSet := make(map[string]bool, len(keep))
	for _, k := range keep {
		keepSet[k] = true
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.ReuseRecord = true
	r.FieldsPerRecord = 7
	if _, err := r.Read(); err != nil { // header
		return nil, fmt.Errorf("reading header: %w", err)
	}

	out := make(map[string][]data.Candle)
	var skipped int
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		code := rec[1]
		if !isOrdinaryEquity(code) && !keepSet[code] {
			skipped++
			continue
		}
		date, err := time.Parse("2006-01-02", rec[0])
		if err != nil {
			return nil, fmt.Errorf("bad date %q: %w", rec[0], err)
		}
		num := func(i int) float64 { v, _ := strconv.ParseFloat(rec[i], 64); return v }
		vol, _ := strconv.ParseInt(rec[6], 10, 64)
		out[code] = append(out[code], data.Candle{
			Date: date, Open: num(2), High: num(3), Low: num(4), Close: num(5), Volume: vol,
		})
	}

	for code, candles := range out {
		if len(candles) < minBars {
			delete(out, code)
			continue
		}
		// The cache is written date-major, so each ticker's slice is already
		// ascending — sort anyway rather than depend on it, since everything
		// downstream (MA, ATR, forward returns) is silently wrong on an
		// out-of-order series instead of erroring.
		sort.Slice(candles, func(i, j int) bool { return candles[i].Date.Before(candles[j].Date) })
	}
	fmt.Printf("Loaded cache %s: %d tickers with >=%d bars (%d non-equity rows skipped)\n", path, len(out), minBars, skipped)
	return out, nil
}
