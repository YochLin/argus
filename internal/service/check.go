package service

import (
	"argus/internal/data"
	"argus/internal/llm"
	"argus/internal/logger"
)

// CheckStockData assembles the data llm.CheckStock needs for /check TICKER:
// quote, up to 5 recent news items, company name, and (when a provider is
// configured) fundamentals/financial statements/analyst rating — the
// single-ticker counterpart of LoadQuoteHighlights (which serves a whole
// watchlist/movers list for the lighter morning-briefing narrative) with the
// extra attach-and-render fields /check's deeper single-ticker analysis
// calls for. Only the quote fetch can fail the call outright; fundamentals/
// statements/analyst-rating are nil-checked and log-only on failure, same
// degrade-per-field convention as llm.StockData's other optional fields, so
// a Finnhub outage narrows the prompt's context instead of failing /check.
// Technicals/candles/strategy hits are deliberately not attached here —
// ComputeTechnicals needs a benchmark ticker resolved from the caller's own
// market-of(ticker) policy (see bot.benchmarkFor), so that attach stays the
// caller's job, same split fetchStockData already has from computeTechnicals.
func CheckStockData(quotes QuoteNewsReader, names data.CompanyNameProvider, fundamentals data.FundamentalsProvider, analystRating data.AnalystRatingProvider, ticker string) (llm.StockData, error) {
	q, err := quotes.GetQuote(ticker)
	if err != nil {
		return llm.StockData{}, err
	}
	news, _ := quotes.GetNews(ticker, 5)
	stock := llm.StockData{Quote: q, News: news, CompanyName: companyNameFor(names, ticker)}

	if fundamentals != nil {
		if fd, err := fundamentals.GetFundamentals(ticker); err != nil {
			logger.Errorf("check: fundamentals %s: %v", ticker, err)
		} else {
			stock.Fundamentals = fd
		}
		if st, err := fundamentals.GetFinancialStatements(ticker, "annual"); err != nil {
			logger.Errorf("check: financial statements %s: %v", ticker, err)
		} else {
			stock.Statement = st
		}
	}
	if analystRating != nil {
		if ar, err := analystRating.GetAnalystRating(ticker); err != nil {
			logger.Errorf("check: analyst rating %s: %v", ticker, err)
		} else {
			stock.AnalystRating = ar
		}
	}
	return stock, nil
}
