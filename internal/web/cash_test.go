package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/market"
	"argus/internal/service"
)

type fakeSettingWriter struct{ settings map[string]string }

func (f *fakeSettingWriter) SetSetting(key, value string) error {
	f.settings[key] = value
	return nil
}

func TestHandleCashSet(t *testing.T) {
	store := &fakeSettingWriter{settings: map[string]string{}}
	s := &Server{db: &fakeDB{}, cashDB: store, lang: i18n.EN, password: "secret"}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("POST /api/cash", s.requireWritable(s.requireAuth(s.handleCashSet)))
	cookie := loginAndGetCookie(t, s, "secret")

	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/cash", bytes.NewBufferString(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := post(`{"market":"tw","amount":386000.5}`); code != http.StatusOK {
		t.Fatalf("tw: status = %d, want 200", code)
	}
	if got := store.settings[service.CashSettingKeyTWD]; got != "386000.50" {
		t.Errorf("TWD cash = %q, want 386000.50", got)
	}
	if code := post(`{"market":"us","amount":0}`); code != http.StatusOK {
		t.Fatalf("us zero: status = %d, want 200", code)
	}
	if got, ok := store.settings[service.CashSettingKeyUSD]; !ok || got != "0.00" {
		t.Errorf("USD cash = %q (set %v), want 0.00 — zero is a real balance, not unset", got, ok)
	}
	for _, bad := range []string{`{"market":"us","amount":-1}`, `{"market":"jp","amount":5}`, `{"market":"us","amount":"x"}`} {
		if code := post(bad); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", bad, code)
		}
	}
}

func TestBuildRisk_HasCash(t *testing.T) {
	quotes := &fakeQuotes{quotes: map[string]*data.Quote{"NVDA": {Ticker: "NVDA", Price: 100}}}
	positions := []db.Position{{Ticker: "NVDA", Shares: 10, AvgCost: 90}}

	unset, err := buildRisk(&fakeDB{positions: positions}, quotes, market.US, 6)
	if err != nil {
		t.Fatal(err)
	}
	if unset.HasCash || unset.AccountValue != 1000 {
		t.Errorf("unset: HasCash = %v, AccountValue = %v, want false, 1000", unset.HasCash, unset.AccountValue)
	}

	zero, err := buildRisk(&fakeDB{positions: positions, settings: map[string]string{service.CashSettingKeyUSD: "0"}}, quotes, market.US, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !zero.HasCash {
		t.Error("declared 0: HasCash = false, want true")
	}
}
