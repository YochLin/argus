package data

import (
	"context"
	"errors"
	"sync"
	"time"

	"argus/internal/market"
	"argus/internal/sinopac"
)

// Shioaji wraps a *sinopac.Client as a Provider — broker-grade TW quotes in
// front of Yahoo's unofficial, suffix-guessing chart API (see yahoo.go).
// Constructed only when SHIOAJI_ADDR is set (cmd/bot/main.go); when it
// isn't, this type never exists and the Multi chain is exactly what it was
// before this file — no shioaji reference anywhere in a default run.
type Shioaji struct {
	c *sinopac.Client

	// exchangeCache mirrors Yahoo's twSuffixCache (yahoo.go) — a bare TW
	// ticker carries no listed-exchange info, so the first lookup tries
	// TSE then OTC and remembers whichever answered.
	exchangeMu    sync.Mutex
	exchangeCache map[string]string
}

func NewShioaji(c *sinopac.Client) *Shioaji {
	return &Shioaji{c: c, exchangeCache: make(map[string]string)}
}

func (s *Shioaji) Name() string { return "shioaji" }

// errShioajiNotTW mirrors FinMind/GoogleNews's own "not this market" guard
// (errNotTWTicker/errGoogleNewsNotTW) — Shioaji only carries TW listings.
var errShioajiNotTW = errors.New("shioaji: taiwan market only")

// errShioajiUnsupported covers the Provider methods Shioaji has no data
// for — it implements the full interface only to sit in the Multi chain.
var errShioajiUnsupported = errors.New("shioaji: not supported")

func (s *Shioaji) GetNews(string, int) ([]NewsItem, error) { return nil, errShioajiUnsupported }

func (s *Shioaji) exchangesFor(ticker string) []string {
	s.exchangeMu.Lock()
	ex, ok := s.exchangeCache[ticker]
	s.exchangeMu.Unlock()
	if ok {
		return []string{ex}
	}
	return []string{"TSE", "OTC"}
}

func (s *Shioaji) cacheExchange(ticker, exchange string) {
	s.exchangeMu.Lock()
	s.exchangeCache[ticker] = exchange
	s.exchangeMu.Unlock()
}

func (s *Shioaji) GetQuote(ticker string) (*Quote, error) {
	if market.Of(ticker) != market.TW {
		return nil, errShioajiNotTW
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var lastErr error
	for _, exchange := range s.exchangesFor(ticker) {
		snaps, err := s.c.Snapshots(ctx, []sinopac.Contract{{SecurityType: "STK", Exchange: exchange, Code: ticker}})
		if err != nil {
			lastErr = err
			continue
		}
		if len(snaps) == 0 {
			lastErr = errors.New("shioaji: empty snapshot for " + ticker)
			continue
		}
		s.cacheExchange(ticker, exchange)
		snap := snaps[0]
		ts, err := time.ParseInLocation("2006-01-02T15:04:05", snap.Datetime, time.Local)
		if err != nil {
			ts = time.Now()
		}
		return &Quote{
			Ticker:        ticker,
			Price:         snap.Close,
			Open:          snap.Open,
			High:          snap.High,
			Low:           snap.Low,
			PrevClose:     snap.Close - snap.ChangePrice,
			Volume:        snap.TotalVolume,
			ChangePercent: snap.ChangeRate,
			Timestamp:     ts,
		}, nil
	}
	return nil, lastErr
}

func (s *Shioaji) GetMarketMovers() ([]string, error) { return nil, errShioajiUnsupported }

// ShioajiMovers implements TWMarketMoversProvider on top of Scanner's
// AmountRank, falling back to fallback (the existing *TWSE) on any error —
// a daemon that's down or mid session-drop must never break /dailyreport's
// TW movers section, same "fallback is free" reasoning as Shioaji sitting
// ahead of Yahoo in the Multi chain (see main.go wiring).
type ShioajiMovers struct {
	c        *sinopac.Client
	fallback TWMarketMoversProvider
}

func NewShioajiMovers(c *sinopac.Client, fallback TWMarketMoversProvider) *ShioajiMovers {
	return &ShioajiMovers{c: c, fallback: fallback}
}

// shioajiMoversCount matches TWSE.GetMarketMovers's own top-20 shape.
const shioajiMoversCount = 20

func (m *ShioajiMovers) GetMarketMovers() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	items, err := m.c.Scanner(ctx, sinopac.ScannerAmountRank, time.Now().Format("2006-01-02"), shioajiMoversCount)
	if err != nil || len(items) == 0 {
		return m.fallback.GetMarketMovers()
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Code == "" || isTWETF(it.Code) {
			continue
		}
		out = append(out, it.Code)
	}
	if len(out) == 0 {
		return m.fallback.GetMarketMovers()
	}
	return out, nil
}
