package sinopac

import (
	"context"
	"fmt"
	"net/http"
)

// DailyQuote is one ticker's OHLCV for one trading day, as served by
// /api/v1/data/daily_quotes.
//
// Two properties matter for anything historical built on this, and neither
// is true of the Yahoo path in internal/data (live-verified 2026-08-25):
//
//   - It is POINT-IN-TIME. The response lists whatever was actually listed
//     that day, so names that later delisted, merged, or changed code are
//     present — 2016-06-15 returns 1,614 codes of which 126 no longer exist
//     in 2026. A study driven off this is not subject to the survivorship
//     bias of screening today's index membership backwards.
//   - Prices are RAW, not adjusted for dividends or splits (2330 on
//     2016-06-15 comes back as 163.0, Yahoo's unadjusted close, against an
//     adjusted 123.0). An ex-dividend day therefore shows a mechanical
//     gap down. internal/data's Yahoo GetHistory reads the same unadjusted
//     `close` series, so this is not a regression relative to it — but it
//     is a real modelling error in both, and TW pays most of its cash
//     dividends in one lump between July and September, so it clusters.
type DailyQuote struct {
	Date   string
	Code   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
	Amount float64
}

// dailyQuoteColumns is the parallel-arrays shape this endpoint returns
// (pandas DataFrame.to_dict("list"), the same quirk as the two regulatory
// endpoints — see Client's doc comment), not an array of row objects.
type dailyQuoteColumns struct {
	Date   []string  `json:"Date"`
	Code   []string  `json:"Code"`
	Open   []float64 `json:"Open"`
	High   []float64 `json:"High"`
	Low    []float64 `json:"Low"`
	Close  []float64 `json:"Close"`
	Volume []int64   `json:"Volume"`
	Amount []float64 `json:"Amount"`
}

// DailyQuotes returns every listed ticker's OHLCV for date (YYYY-MM-DD).
// A non-trading day (weekend, holiday, or a 彈性放假 bridge day) is not an
// error — it comes back as an empty slice, which is how callers walking a
// date range are expected to skip it.
//
// The "exclude" flag drops warrants, and it is ALWAYS sent true here. Two
// traps make that worth stating (live-verified 2026-08-25 on 2016-06-15):
// the REST field is named "exclude", not "exclude_warrant" like the CLI's
// flag, and its REST default is the opposite of the CLI's — omitting it
// returns 14,036 rows against 1,614 with it set, since Taiwan lists roughly
// eight warrants for every ordinary stock. Omitting it silently makes a
// whole-market history fetch 8.7x larger and slower for no added coverage.
//
// ETFs are NOT excluded by this flag, so a caller wanting only ordinary
// equities still has to filter by code itself.
func (c *Client) DailyQuotes(ctx context.Context, date string) ([]DailyQuote, error) {
	var cols dailyQuoteColumns
	req := map[string]any{"date": date, "exclude": true}
	if err := c.do(ctx, http.MethodPost, "/api/v1/data/daily_quotes", req, &cols); err != nil {
		return nil, err
	}
	n := len(cols.Code)
	if n == 0 {
		return nil, nil
	}
	// Every column has to be the same length or the row pivot below would
	// silently pair the wrong ticker with the wrong price.
	for name, got := range map[string]int{
		"Date": len(cols.Date), "Open": len(cols.Open), "High": len(cols.High),
		"Low": len(cols.Low), "Close": len(cols.Close), "Volume": len(cols.Volume),
	} {
		if got != n {
			return nil, fmt.Errorf("daily_quotes %s: column %s has %d rows, Code has %d", date, name, got, n)
		}
	}
	out := make([]DailyQuote, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, DailyQuote{
			Date: cols.Date[i], Code: cols.Code[i],
			Open: cols.Open[i], High: cols.High[i], Low: cols.Low[i], Close: cols.Close[i],
			Volume: cols.Volume[i], Amount: amountAt(cols.Amount, i),
		})
	}
	return out, nil
}

func amountAt(vals []float64, i int) float64 {
	if i < len(vals) {
		return vals[i]
	}
	return 0
}
