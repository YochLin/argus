package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"argus/internal/data"
)

// insider_cache.go is Phase 25 §8.2's offline (ticker -> transactions) cache,
// same two-step pattern as earnings_cache.go: -insider-tx pays Finnhub's
// per-ticker fetch once, every later run reads the CSV and touches no
// network. Unlike history_cache.go's TW path, this needs no day-major
// walk — GetInsiderTransactionsRange already takes a date range per
// ticker, so one request per ticker covers the whole study window.

// buildInsiderTxCache calls GetInsiderTransactionsRange for every ticker and
// writes the raw rows to path. US-only — Finnhub has no TW insider-filing
// equivalent (see data.InsiderTransactionProvider's doc comment).
func buildInsiderTxCache(f *data.Finnhub, tickers []string, from, to, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	defer w.Flush()
	if err := w.Write([]string{"Ticker", "Name", "Change", "TransactionDate", "TransactionCode", "TransactionPrice"}); err != nil {
		return err
	}

	var ok, failed, rows int
	for i, ticker := range tickers {
		time.Sleep(1100 * time.Millisecond) // Finnhub free tier: 60 calls/min
		txs, err := f.GetInsiderTransactionsRange(ticker, from, to)
		if err != nil {
			// One ticker's Finnhub miss must not cost the whole build — same
			// accounting convention as buildEarningsDatesCache.
			fmt.Printf("  %s: %v (skipped)\n", ticker, err)
			failed++
			continue
		}
		for _, tx := range txs {
			if err := w.Write([]string{
				ticker, tx.Name, strconv.FormatInt(tx.Change, 10), tx.TransactionDate, tx.TransactionCode,
				strconv.FormatFloat(tx.TransactionPrice, 'f', 4, 64),
			}); err != nil {
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
	fmt.Printf("Insider-tx cache built: %s — %d tickers ok, %d failed, %d rows\n", path, ok, failed, rows)
	return w.Error()
}

// loadInsiderTxCache reads a cache written by buildInsiderTxCache into a
// per-ticker slice of transactions — CheckInsiderClusterBuyExact does its
// own trailing-window filtering per evaluation day, so this is a flat
// per-ticker list, not pre-aligned to any date index (unlike
// loadHistoryCache's per-ticker candle slices).
func loadInsiderTxCache(path string) (map[string][]data.InsiderTransaction, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil { // header
		return nil, fmt.Errorf("reading header: %w", err)
	}
	out := make(map[string][]data.InsiderTransaction)
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		ticker := rec[0]
		change, _ := strconv.ParseInt(rec[2], 10, 64)
		price, _ := strconv.ParseFloat(rec[5], 64)
		out[ticker] = append(out[ticker], data.InsiderTransaction{
			Ticker: ticker, Name: rec[1], Change: change,
			TransactionDate: rec[3], TransactionCode: rec[4], TransactionPrice: price,
		})
	}
	return out, nil
}
