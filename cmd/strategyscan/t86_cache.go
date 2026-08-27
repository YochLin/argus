package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"argus/internal/data"
)

// t86_cache.go is the trust-net analog of history_cache.go's OHLCV
// builder/loader — see that file's package doc comment for the general
// shape (build once, read many). It exists because 網5's data source, TWSE's
// T86 report, has no ranged query: one HTTP request is one calendar day,
// whole market. Walking that live, per ticker, across a decade-long backtest
// (~2,500 trading days x 150 tw150 tickers) would be ~375,000 sequential
// requests — this instead pays ~2,500 requests ONCE (one per calendar day,
// whole market, keeping only the tw150 rows) and every subsequent run reads
// the file. Before this cache existed, -history-file (the fast, point-in-
// time Shioaji OHLCV cache) required -skip-trust for exactly this reason —
// see main.go's error message before this file was added.

// buildT86Cache walks every calendar day in [from, to], fetches TWSE's
// whole-market T86 report for that day, and writes ONLY the tickers in keep
// to path — keeping the file to the tw150 universe's size instead of the
// whole market's (~2,000 codes), since that's the only universe 網5 is ever
// backtested against (see main.go's TW ticker list). A day with no rows at
// all (weekend/holiday) is skipped, same as buildHistoryCache; a day that
// errors is retried a few times before giving up on the whole build, since a
// silently-missing day would show up downstream only as a mysteriously-empty
// AlignTrustNet slot.
func buildT86Cache(twse *data.TWSE, keep map[string]bool, from, to time.Time, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"Date", "Code", "ForeignNet", "TrustNet"}); err != nil {
		return err
	}

	// consecutiveEmptyWeekdays guards against a silent data hole: TWSE
	// answers BOTH a genuine non-trading day and a WAF block with the exact
	// same "no data" shape (twse_t86.go's doFetchT86TrustForeignDay doc
	// comment — live-verified 2026-08-27 building this exact cache, which
	// tripped a sustained IP-level block after a too-fast first attempt).
	// GetT86Day/fetchT86TrustForeignDay can't tell the two apart and
	// returns (nil, nil) either way, so a WAF block would otherwise pass
	// through this loop as a long run of "holidays" instead of an error —
	// corrupting the cache with silent gaps rather than failing loudly.
	// Weekends are excluded from the count (they're unconditionally real
	// non-trading days); Lunar New Year is the longest legitimate all-
	// weekday closure TWSE actually has (up to ~7 weekdays), so the
	// threshold below is set past that with margin.
	const maxConsecutiveEmptyWeekdays = 12
	consecutiveEmptyWeekdays := 0

	var days, tradingDays, rows int
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		days++
		date := d.Format("2006-01-02")

		var dayMap map[string]data.TrustNetDay
		var err error
		for attempt := 0; attempt < 4; attempt++ {
			if dayMap, err = twse.GetT86Day(d); err == nil {
				break
			}
			fmt.Printf("  %s: %v (retry %d/3)\n", date, err, attempt+1)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
		if err != nil {
			return fmt.Errorf("t86 %s: %w", date, err)
		}
		// Paced on EVERY calendar day, not just the ones with rows to write —
		// a weekend still costs a real HTTP round-trip (see
		// doFetchT86TrustForeignDay), so pacing only the non-empty branch
		// would let a run of weekends/holidays fire back-to-back with no
		// delay at all. See the sleep call's own comment for the rate this
		// value is based on.
		time.Sleep(2 * time.Second)
		if len(dayMap) == 0 {
			if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
				consecutiveEmptyWeekdays++
				if consecutiveEmptyWeekdays > maxConsecutiveEmptyWeekdays {
					return fmt.Errorf("t86: %d consecutive empty weekdays ending %s — this almost certainly means TWSE started rejecting requests (WAF/rate-limit) partway through the build, not %d real holidays in a row; re-run from this date with a slower pace once cleared, don't trust a cache built past this point",
						consecutiveEmptyWeekdays, date, consecutiveEmptyWeekdays)
				}
			}
			continue
		}
		consecutiveEmptyWeekdays = 0
		tradingDays++
		for code, row := range dayMap {
			if !keep[code] {
				continue
			}
			if err := w.Write([]string{
				date, code,
				strconv.FormatInt(row.ForeignNet, 10),
				strconv.FormatInt(row.Net, 10),
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
	fmt.Printf("T86 cache built: %s — %d calendar days walked, %d trading days, %d rows (%d tickers kept)\n", path, days, tradingDays, rows, len(keep))
	return w.Error()
}

// loadT86Cache reads a cache written by buildT86Cache into per-ticker
// data.TrustNetDay series, oldest first — the trust-net analog of
// loadHistoryCache.
func loadT86Cache(path string) (map[string][]data.TrustNetDay, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.ReuseRecord = true
	r.FieldsPerRecord = 4
	if _, err := r.Read(); err != nil { // header
		return nil, fmt.Errorf("reading header: %w", err)
	}

	out := make(map[string][]data.TrustNetDay)
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		date, err := time.Parse("2006-01-02", rec[0])
		if err != nil {
			return nil, fmt.Errorf("bad date %q: %w", rec[0], err)
		}
		foreignNet, _ := strconv.ParseInt(rec[2], 10, 64)
		trustNet, _ := strconv.ParseInt(rec[3], 10, 64)
		code := rec[1]
		out[code] = append(out[code], data.TrustNetDay{Date: date, Net: trustNet, ForeignNet: foreignNet})
	}
	for code, rows := range out {
		sort.Slice(rows, func(i, j int) bool { return rows[i].Date.Before(rows[j].Date) })
		out[code] = rows
	}
	fmt.Printf("Loaded T86 cache %s: %d tickers\n", path, len(out))
	return out, nil
}
