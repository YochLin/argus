package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFinnhubGetInsiderTransactions_TWGuard(t *testing.T) {
	f := NewFinnhub("")
	if _, err := f.GetInsiderTransactions("2330", 10); err != errTWNotSupported {
		t.Errorf("GetInsiderTransactions(2330) error = %v, want errTWNotSupported", err)
	}
}

func testInsiderRows() []finnhubInsiderTransaction {
	return []finnhubInsiderTransaction{
		{Name: "COOK TIMOTHY D", Change: -59751, TransactionDate: "2025-10-02", TransactionCode: "S", TransactionPrice: 257.57},
		{Name: "Khan Sabih", Change: -49390, TransactionDate: "2025-10-01", TransactionCode: "F"},
		{Name: "Khan Sabih", Change: 92403, TransactionDate: "2025-10-01", TransactionCode: "M"},
	}
}

func TestBuildInsiderTransactions_WithLimit(t *testing.T) {
	got := buildInsiderTransactions("AAPL", testInsiderRows(), 2)
	want := []InsiderTransaction{
		{Ticker: "AAPL", Name: "COOK TIMOTHY D", Change: -59751, TransactionDate: "2025-10-02", TransactionCode: "S", TransactionPrice: 257.57},
		{Ticker: "AAPL", Name: "Khan Sabih", Change: -49390, TransactionDate: "2025-10-01", TransactionCode: "F"},
	}
	assert.Equal(t, want, got)
}

func TestBuildInsiderTransactions_NoLimit(t *testing.T) {
	got := buildInsiderTransactions("AAPL", testInsiderRows(), 0)
	want := []InsiderTransaction{
		{Ticker: "AAPL", Name: "COOK TIMOTHY D", Change: -59751, TransactionDate: "2025-10-02", TransactionCode: "S", TransactionPrice: 257.57},
		{Ticker: "AAPL", Name: "Khan Sabih", Change: -49390, TransactionDate: "2025-10-01", TransactionCode: "F"},
		{Ticker: "AAPL", Name: "Khan Sabih", Change: 92403, TransactionDate: "2025-10-01", TransactionCode: "M"},
	}
	assert.Equal(t, want, got)
}
