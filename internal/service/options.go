package service

import (
	"fmt"
	"math"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/option"
)

// FindOptionQuote fetches p's live chain and returns the data.OptionQuote
// matching its specific contract, plus days-to-expiry. There's no
// single-contract quote endpoint — Yahoo only serves a whole expiry's chain
// — so this costs one request per distinct (underlying, expiry) position;
// fine at single-user, few-position scale (ponytail: cache per-request if
// /portfolio ever holds enough contracts to make that slow). The single
// shared chain-fetch-and-match step behind both OptionMark (mark/dte only)
// and internal/web's fetchOptionMarkAndGreeks (mark/dte plus Greeks, which
// need the matched quote's ImpliedVolatility that OptionMark alone doesn't
// expose) — Phase 24 tech debt 5's tail.
func FindOptionQuote(chain data.OptionChainProvider, p db.OptionPosition) (data.OptionQuote, int, error) {
	if chain == nil {
		return data.OptionQuote{}, 0, fmt.Errorf("option chain provider unavailable")
	}
	expiry, err := time.Parse("2006-01-02", p.Expiry)
	if err != nil {
		return data.OptionQuote{}, 0, err
	}
	quotes, err := chain.GetOptionChain(p.Underlying, expiry)
	if err != nil {
		return data.OptionQuote{}, 0, err
	}
	for _, q := range quotes {
		if q.ContractSymbol == p.ContractSymbol {
			dte := int(math.Ceil(time.Until(expiry).Hours() / 24))
			return q, dte, nil
		}
	}
	return data.OptionQuote{}, 0, fmt.Errorf("contract not found in chain")
}

// OptionMark fetches p's live chain via FindOptionQuote and returns the mark
// price (option.Mark) and days-to-expiry for its specific contract.
func OptionMark(chain data.OptionChainProvider, p db.OptionPosition) (mark float64, dte int, err error) {
	q, dte, err := FindOptionQuote(chain, p)
	if err != nil {
		return 0, 0, err
	}
	return option.Mark(q.Bid, q.Ask, q.LastPrice), dte, nil
}

// GatherOptionCandidates fetches every expiry within profile's DTE band (one
// chain request each — Yahoo's endpoint is per-expiry, see OptionMark's doc
// comment for the same constraint) and runs option.Select over the merged
// result.
func GatherOptionCandidates(chain data.OptionChainProvider, ticker string, spot float64, profile option.Profile, now time.Time) ([]option.Candidate, error) {
	expirations, err := chain.GetOptionExpirations(ticker)
	if err != nil {
		return nil, err
	}

	var quoteChain []data.OptionQuote
	for _, expiry := range expirations {
		dte := int(math.Ceil(time.Until(expiry).Hours() / 24))
		if dte < profile.DTEMin || dte > profile.DTEMax {
			continue
		}
		quotes, err := chain.GetOptionChain(ticker, expiry)
		if err != nil {
			logger.Errorf("option select %s @ %s: %v", ticker, expiry.Format("2006-01-02"), err)
			continue
		}
		quoteChain = append(quoteChain, quotes...)
	}
	return option.Select(quoteChain, spot, now, profile), nil
}

// ATMIVStore is the persistence boundary RecordDailyATMIV needs — narrowed
// to the one write it performs, same convention as RiskStore/SnapshotStore.
type ATMIVStore interface {
	SaveATMIV(ticker, date string, iv float64, dte int) error
}

// RecordDailyATMIV snapshots one ATM-ish implied volatility reading per
// ticker that has open option interest available — see migration 15's doc
// comment for why this starts accumulating now despite having no reader for
// 6-12 months. Silent, best-effort per ticker — same "never block the
// closing snapshot" convention as the rest of that job.
func RecordDailyATMIV(chain data.OptionChainProvider, store ATMIVStore, tickers []string, prices map[string]float64, date string) {
	if chain == nil {
		return
	}
	for _, ticker := range tickers {
		spot, ok := prices[ticker]
		if !ok || spot <= 0 {
			continue
		}
		expirations, err := chain.GetOptionExpirations(ticker)
		if err != nil || len(expirations) == 0 {
			continue
		}
		expiry := expirations[0]
		quotes, err := chain.GetOptionChain(ticker, expiry)
		if err != nil {
			continue
		}

		var atmIV float64
		bestDiff := math.MaxFloat64
		for _, q := range quotes {
			if q.Right != "C" {
				continue
			}
			if diff := math.Abs(q.Strike - spot); diff < bestDiff {
				bestDiff = diff
				atmIV = q.ImpliedVolatility
			}
		}
		if atmIV <= 0 {
			continue
		}
		dte := int(math.Ceil(time.Until(expiry).Hours() / 24))
		if err := store.SaveATMIV(ticker, date, atmIV, dte); err != nil {
			logger.Errorf("record ATM IV %s: %v", ticker, err)
		}
	}
}
