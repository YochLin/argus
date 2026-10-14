package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/db"
)

func TestParseImportCSV(t *testing.T) {
	t.Run("valid rows, header skipped, fee optional", func(t *testing.T) {
		csv := "date,ticker,action,shares,price,fee\n" +
			"2026-01-05,AAPL,buy,10,150,1.5\n" +
			"2026-01-10,aapl,sell,5,160\n"
		rows, err := parseImportCSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		if rows[0].Status != "ok" || rows[0].Ticker != "AAPL" || rows[0].Action != "BUY" || rows[0].Fee != 1.5 {
			t.Errorf("row[0] = %+v, want normalized AAPL/BUY with fee 1.5", rows[0])
		}
		if rows[1].Status != "ok" || rows[1].Ticker != "AAPL" || rows[1].Action != "SELL" || rows[1].Fee != 0 {
			t.Errorf("row[1] = %+v, want normalized AAPL/SELL with fee defaulting to 0", rows[1])
		}
		if rows[0].Line != 2 || rows[1].Line != 3 {
			t.Errorf("line numbers = %d,%d, want 2,3 (1-indexed, header skipped)", rows[0].Line, rows[1].Line)
		}
	})

	t.Run("no data rows", func(t *testing.T) {
		rows, err := parseImportCSV("date,ticker,action,shares,price,fee\n")
		if err != nil || rows != nil {
			t.Errorf("got rows=%v err=%v, want nil,nil", rows, err)
		}
	})

	t.Run("invalid rows are marked error, not dropped", func(t *testing.T) {
		csv := "date,ticker,action,shares,price,fee\n" +
			"bad-date,AAPL,BUY,10,150,0\n" +
			"2026-01-05,AAPL,HOLD,10,150,0\n" +
			"2026-01-05,AAPL,BUY,-1,150,0\n" +
			"2026-01-05,AAPL,BUY,10,150,-1\n" +
			"2026-01-05,AAPL\n"
		rows, err := parseImportCSV(csv)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 5 {
			t.Fatalf("len(rows) = %d, want 5", len(rows))
		}
		for i, row := range rows {
			if row.Status != "error" {
				t.Errorf("row[%d].Status = %q, want %q (message: %s)", i, row.Status, "error", row.Message)
			}
		}
	})
}

func TestAnnotateImportRows(t *testing.T) {
	existing := []db.Transaction{
		{Date: "2026-01-10", Ticker: "AAPL", Side: "BUY", Shares: 10, Price: 150},
		{Date: "2026-01-20", Ticker: "AAPL", Side: "SELL", Shares: 5, Price: 160, RealizedPnL: 48},
	}

	t.Run("exact duplicate of an existing transaction", func(t *testing.T) {
		rows := []importRow{
			{Status: "ok", Date: "2026-01-10", Ticker: "AAPL", Action: "BUY", Shares: 10, Price: 150},
		}
		annotateImportRows(rows, existing)
		if rows[0].Status != "duplicate" {
			t.Errorf("Status = %q, want duplicate", rows[0].Status)
		}
	})

	t.Run("duplicate within the same batch", func(t *testing.T) {
		rows := []importRow{
			{Status: "ok", Date: "2026-02-01", Ticker: "MSFT", Action: "BUY", Shares: 10, Price: 400},
			{Status: "ok", Date: "2026-02-01", Ticker: "MSFT", Action: "BUY", Shares: 10, Price: 400},
		}
		annotateImportRows(rows, nil)
		if rows[0].Status != "ok" {
			t.Errorf("rows[0].Status = %q, want ok (first occurrence keeps)", rows[0].Status)
		}
		if rows[1].Status != "duplicate" {
			t.Errorf("rows[1].Status = %q, want duplicate (second occurrence)", rows[1].Status)
		}
	})

	t.Run("dated before the ticker's latest existing transaction warns but doesn't block", func(t *testing.T) {
		rows := []importRow{
			{Status: "ok", Date: "2026-01-15", Ticker: "AAPL", Action: "BUY", Shares: 3, Price: 145},
		}
		annotateImportRows(rows, existing)
		if rows[0].Status != "warning" {
			t.Errorf("Status = %q, want warning", rows[0].Status)
		}
		if rows[0].Message == "" {
			t.Error("expected a non-empty warning message")
		}
	})

	t.Run("new ticker with no history is untouched", func(t *testing.T) {
		rows := []importRow{
			{Status: "ok", Date: "2020-01-01", Ticker: "NVDA", Action: "BUY", Shares: 1, Price: 50},
		}
		annotateImportRows(rows, existing)
		if rows[0].Status != "ok" {
			t.Errorf("Status = %q, want ok", rows[0].Status)
		}
	})

	t.Run("rows already marked error are left alone", func(t *testing.T) {
		rows := []importRow{
			{Status: "error", Message: "invalid date", Date: "2026-01-10", Ticker: "AAPL", Action: "BUY", Shares: 10, Price: 150},
		}
		annotateImportRows(rows, existing)
		if rows[0].Status != "error" || rows[0].Message != "invalid date" {
			t.Errorf("row mutated: %+v", rows[0])
		}
	})
}

// fakeCSVWriter is a csvWriter stub recording call order, so tests can
// assert applyImportRows sorts by date regardless of input slice order.
type fakeCSVWriter struct {
	calls   []string
	sellErr map[string]error // keyed by ticker, for forcing one row to fail
}

func (f *fakeCSVWriter) RecordBuy(ticker string, shares, price, fee float64, date string) (db.Position, error) {
	f.calls = append(f.calls, "BUY "+date+" "+ticker)
	return db.Position{Ticker: ticker}, nil
}

func (f *fakeCSVWriter) RecordSell(ticker string, shares, price, fee float64, date string) (db.Position, float64, error) {
	f.calls = append(f.calls, "SELL "+date+" "+ticker)
	if err, ok := f.sellErr[ticker]; ok {
		return db.Position{}, 0, err
	}
	return db.Position{Ticker: ticker}, 0, nil
}

func TestApplyImportRows(t *testing.T) {
	t.Run("applies in ascending date order regardless of slice order", func(t *testing.T) {
		rows := []importRow{
			{Status: "ok", Date: "2026-03-01", Ticker: "MSFT", Action: "SELL"},
			{Status: "ok", Date: "2026-01-01", Ticker: "AAPL", Action: "BUY"},
			{Status: "warning", Date: "2026-02-01", Ticker: "AAPL", Action: "BUY"},
		}
		w := &fakeCSVWriter{}
		applied := applyImportRows(w, rows)
		if applied != 3 {
			t.Fatalf("applied = %d, want 3", applied)
		}
		want := []string{"BUY 2026-01-01 AAPL", "BUY 2026-02-01 AAPL", "SELL 2026-03-01 MSFT"}
		if len(w.calls) != len(want) {
			t.Fatalf("calls = %v, want %v", w.calls, want)
		}
		for i := range want {
			if w.calls[i] != want[i] {
				t.Errorf("calls[%d] = %q, want %q (chronological order not respected)", i, w.calls[i], want[i])
			}
		}
		for i, row := range rows {
			if row.Status != "applied" {
				t.Errorf("rows[%d].Status = %q, want applied", i, row.Status)
			}
		}
	})

	t.Run("skips duplicate and error rows", func(t *testing.T) {
		rows := []importRow{
			{Status: "duplicate", Date: "2026-01-01", Ticker: "AAPL", Action: "BUY"},
			{Status: "error", Date: "2026-01-02", Ticker: "AAPL", Action: "BUY"},
		}
		w := &fakeCSVWriter{}
		applied := applyImportRows(w, rows)
		if applied != 0 || len(w.calls) != 0 {
			t.Errorf("applied = %d, calls = %v, want 0 and none", applied, w.calls)
		}
	})

	t.Run("a write failure marks that row error without aborting the rest", func(t *testing.T) {
		rows := []importRow{
			{Status: "ok", Date: "2026-01-01", Ticker: "AAPL", Action: "SELL"},
			{Status: "ok", Date: "2026-01-02", Ticker: "MSFT", Action: "BUY"},
		}
		w := &fakeCSVWriter{sellErr: map[string]error{"AAPL": errors.New("no position")}}
		applied := applyImportRows(w, rows)
		if applied != 1 {
			t.Errorf("applied = %d, want 1", applied)
		}
		if rows[0].Status != "error" || rows[0].Message != "no position" {
			t.Errorf("rows[0] = %+v, want error/no position", rows[0])
		}
		if rows[1].Status != "applied" {
			t.Errorf("rows[1].Status = %q, want applied", rows[1].Status)
		}
	})
}

func newImportTestServer(existing []db.Transaction, w csvWriter) *Server {
	s := &Server{
		db:    &fakeDB{txs: existing},
		csvDB: w,
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/import", s.handleImport)
	return s
}

func TestHandleImport(t *testing.T) {
	csvBody := "date,ticker,action,shares,price,fee\n2026-01-05,AAPL,BUY,10,150,0\n"

	t.Run("dry run previews without writing", func(t *testing.T) {
		w := &fakeCSVWriter{}
		s := newImportTestServer(nil, w)
		body, _ := json.Marshal(importRequest{CSV: csvBody, DryRun: true})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resp importResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Applied != 0 || !resp.DryRun || len(w.calls) != 0 {
			t.Errorf("resp = %+v, calls = %v, want no writes on a dry run", resp, w.calls)
		}
		if len(resp.Rows) != 1 || resp.Rows[0].Status != "ok" {
			t.Errorf("resp.Rows = %+v, want one ok row", resp.Rows)
		}
	})

	t.Run("confirm applies and reports the write", func(t *testing.T) {
		w := &fakeCSVWriter{}
		s := newImportTestServer(nil, w)
		body, _ := json.Marshal(importRequest{CSV: csvBody, DryRun: false})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(body)))
		var resp importResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Applied != 1 || len(w.calls) != 1 {
			t.Errorf("resp = %+v, calls = %v, want one applied write", resp, w.calls)
		}
	})

	t.Run("existing duplicate is skipped on confirm", func(t *testing.T) {
		existing := []db.Transaction{{Date: "2026-01-05", Ticker: "AAPL", Side: "BUY", Shares: 10, Price: 150}}
		w := &fakeCSVWriter{}
		s := newImportTestServer(existing, w)
		body, _ := json.Marshal(importRequest{CSV: csvBody, DryRun: false})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(body)))
		var resp importResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Applied != 0 || len(w.calls) != 0 {
			t.Errorf("resp = %+v, calls = %v, want the duplicate skipped entirely", resp, w.calls)
		}
		if resp.Rows[0].Status != "duplicate" {
			t.Errorf("Rows[0].Status = %q, want duplicate", resp.Rows[0].Status)
		}
	})
}
