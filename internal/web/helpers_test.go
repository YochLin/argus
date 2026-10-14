package web

import "argus/internal/db"

func tx(ticker, side string, shares, price float64, date string) db.Transaction {
	return db.Transaction{Ticker: ticker, Side: side, Shares: shares, Price: price, Date: date}
}

func snap(ticker, date string, close float64) db.DailySnapshot {
	return db.DailySnapshot{Ticker: ticker, Date: date, Close: close}
}
