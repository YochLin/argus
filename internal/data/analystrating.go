package data

import (
	"fmt"
	"sort"
)

// AnalystRating is a snapshot of Wall Street analyst buy/hold/sell counts for
// a ticker, plus the immediately preceding month's counts so a prompt can
// describe the trend (more analysts turning bullish/bearish), not just a
// static headcount.
type AnalystRating struct {
	Ticker                                 string
	Period                                 string // "2026-06-01", the most recent month Finnhub has counts for
	StrongBuy, Buy, Hold, Sell, StrongSell int
	// HasPrev is false when Finnhub has only one period on record (e.g. a
	// recently-covered ticker) — PrevStrongBuy..PrevStrongSell are all zero
	// in that case and must not be read as "no analysts."
	HasPrev                                                    bool
	PrevStrongBuy, PrevBuy, PrevHold, PrevSell, PrevStrongSell int
}

// AnalystRatingProvider is implemented only by Finnhub — same reasoning as
// FundamentalsProvider/EarningsProvider: no Yahoo equivalent we're willing to
// depend on.
type AnalystRatingProvider interface {
	GetAnalystRating(ticker string) (*AnalystRating, error)
}

type finnhubAnalystRating struct {
	Symbol     string `json:"symbol"`
	Period     string `json:"period"`
	StrongBuy  int    `json:"strongBuy"`
	Buy        int    `json:"buy"`
	Hold       int    `json:"hold"`
	Sell       int    `json:"sell"`
	StrongSell int    `json:"strongSell"`
}

// GetAnalystRating fetches Finnhub's /stock/recommendation trend, one ticker
// per call — unlike the earnings calendar, this endpoint only accepts a
// single symbol, so (like GetFundamentals) it's only called for a bounded
// ticker set (watchlist, not the broad candidate list) to stay under the
// free-tier rate limit.
func (f *Finnhub) GetAnalystRating(ticker string) (*AnalystRating, error) {
	var result []finnhubAnalystRating
	if err := f.get(fmt.Sprintf("/stock/recommendation?symbol=%s", ticker), &result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("finnhub: no analyst ratings for %s", ticker)
	}

	// Finnhub documents this as most-recent-period-first, but sort
	// defensively rather than trust response ordering for something that
	// determines which entry counts as "current" vs. "previous."
	sort.Slice(result, func(i, j int) bool { return result[i].Period > result[j].Period })

	latest := result[0]
	rating := &AnalystRating{
		Ticker:     ticker,
		Period:     latest.Period,
		StrongBuy:  latest.StrongBuy,
		Buy:        latest.Buy,
		Hold:       latest.Hold,
		Sell:       latest.Sell,
		StrongSell: latest.StrongSell,
	}
	if len(result) > 1 {
		prev := result[1]
		rating.HasPrev = true
		rating.PrevStrongBuy = prev.StrongBuy
		rating.PrevBuy = prev.Buy
		rating.PrevHold = prev.Hold
		rating.PrevSell = prev.Sell
		rating.PrevStrongSell = prev.StrongSell
	}
	return rating, nil
}
