package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
)

func feePtr(v float64) *float64 { return &v }

// fakeTrade is a TradeExecutor stub recording its last call's arguments.
type fakeTrade struct {
	buyMsg, sellMsg, stopMsg, buyAlertMsg, deleteMsg string
	buyErr, sellErr, stopErr, buyAlertErr, deleteErr error
	lastBuy, lastSell                                tradeRequest
	lastStop                                         stopRequest
	lastBuyAlert                                     buyAlertRequest
	lastDeleteID                                     int64
	notified                                         []string
}

func (f *fakeTrade) Notify(msg string) {
	f.notified = append(f.notified, msg)
}

func (f *fakeTrade) ExecuteBuy(ticker string, shares, price float64, fee *float64, date string) (string, error) {
	f.lastBuy = tradeRequest{Ticker: ticker, Shares: shares, Price: price, Fee: fee, Date: date}
	return f.buyMsg, f.buyErr
}

func (f *fakeTrade) ExecuteSell(_ context.Context, ticker string, shares, price float64, fee *float64, date string) (string, error) {
	f.lastSell = tradeRequest{Ticker: ticker, Shares: shares, Price: price, Fee: fee, Date: date}
	return f.sellMsg, f.sellErr
}

func (f *fakeTrade) ExecuteSetStop(ticker string, price float64) (string, error) {
	f.lastStop = stopRequest{Ticker: ticker, Price: price}
	return f.stopMsg, f.stopErr
}

func (f *fakeTrade) ExecuteAddBuyAlert(ticker string, price float64) (string, error) {
	f.lastBuyAlert = buyAlertRequest{Ticker: ticker, Price: price}
	return f.buyAlertMsg, f.buyAlertErr
}

func (f *fakeTrade) ExecuteDeleteTransaction(id int64) (string, error) {
	f.lastDeleteID = id
	return f.deleteMsg, f.deleteErr
}

// fakeWatchlistDB is a watchlistWriter stub.
type fakeWatchlistDB struct {
	added, removed []string
	err            error
}

func (f *fakeWatchlistDB) AddTicker(ticker string) error {
	f.added = append(f.added, ticker)
	return f.err
}

func (f *fakeWatchlistDB) RemoveTicker(ticker string) error {
	f.removed = append(f.removed, ticker)
	return f.err
}

// fakeBuyAlertDB is a buyAlertWriter stub.
type fakeBuyAlertDB struct {
	removed []int64
	err     error
}

func (f *fakeBuyAlertDB) RemoveBuyAlert(id int64) error {
	f.removed = append(f.removed, id)
	return f.err
}

// fakeThesisDB is a thesisWriter stub.
type fakeThesisDB struct {
	lastTicker, lastText string
	err                  error
}

func (f *fakeThesisDB) SetThesis(ticker, text string) error {
	f.lastTicker, f.lastText = ticker, text
	return f.err
}

// newTradeTestServer builds a Server with the write routes registered
// (mirroring New's own registration for password != "") over fakes, so
// tests don't need a real *db.DB — see testServer's own reasoning above.
func newTradeTestServer(password string, trade TradeExecutor, wl watchlistWriter) *Server {
	s := &Server{
		db:          &fakeDB{},
		watchlistDB: wl,
		buyAlertDB:  &fakeBuyAlertDB{},
		thesisDB:    &fakeThesisDB{},
		quotes:      &fakeQuotes{},
		history:     &fakeHistory{},
		lang:        i18n.EN,
		password:    password,
		trade:       trade,
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/login", s.requireWritable(s.handleLogin))
	s.mux.HandleFunc("POST /api/trade/buy", s.requireWritable(s.requireAuth(s.requireTrade(s.handleTradeBuy))))
	s.mux.HandleFunc("POST /api/trade/sell", s.requireWritable(s.requireAuth(s.requireTrade(s.handleTradeSell))))
	s.mux.HandleFunc("POST /api/trade/delete", s.requireWritable(s.requireAuth(s.requireTrade(s.handleDeleteTransaction))))
	s.mux.HandleFunc("POST /api/stop", s.requireWritable(s.requireAuth(s.requireTrade(s.handleSetStop))))
	s.mux.HandleFunc("POST /api/watchlist/add", s.requireWritable(s.requireAuth(s.handleWatchlistAdd)))
	s.mux.HandleFunc("POST /api/watchlist/remove", s.requireWritable(s.requireAuth(s.handleWatchlistRemove)))
	s.mux.HandleFunc("POST /api/buy-alerts/add", s.requireWritable(s.requireAuth(s.requireTrade(s.handleBuyAlertAdd))))
	s.mux.HandleFunc("POST /api/buy-alerts/remove", s.requireWritable(s.requireAuth(s.handleBuyAlertRemove)))
	s.mux.HandleFunc("POST /api/thesis", s.requireWritable(s.requireAuth(s.handleThesisSet)))
	return s
}

func loginAndGetCookie(t *testing.T, s *Server, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			return c
		}
	}
	t.Fatal("login did not set the auth cookie")
	return nil
}

func TestHandleLogin(t *testing.T) {
	s := newTradeTestServer("secret", &fakeTrade{}, &fakeWatchlistDB{})

	t.Run("wrong password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"password": "wrong"})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("correct password sets a verifiable cookie", func(t *testing.T) {
		cookie := loginAndGetCookie(t, s, "secret")
		if !verifyAuthToken("secret", cookie.Value) {
			t.Errorf("verifyAuthToken(%q) = false, want true for a freshly issued cookie", cookie.Value)
		}
		if verifyAuthToken("wrong-password", cookie.Value) {
			t.Errorf("verifyAuthToken with the wrong password = true, want false")
		}
	})
}

func TestRequireAuth(t *testing.T) {
	trade := &fakeTrade{buyMsg: "bought"}
	s := newTradeTestServer("secret", trade, &fakeWatchlistDB{})
	body, _ := json.Marshal(tradeRequest{Ticker: "AAPL", Shares: 1, Price: 100})

	t.Run("no cookie", func(t *testing.T) {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(body)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("garbage cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: "not-a-valid-token"})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("valid cookie", func(t *testing.T) {
		cookie := loginAndGetCookie(t, s, "secret")
		req := httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleTradeBuy(t *testing.T) {
	trade := &fakeTrade{buyMsg: "bought AAPL"}
	s := newTradeTestServer("secret", trade, &fakeWatchlistDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	t.Run("defaults an omitted date to today", func(t *testing.T) {
		body, _ := json.Marshal(tradeRequest{Ticker: "aapl", Shares: 10, Price: 200, Fee: feePtr(1)})
		req := httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		var got tradeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Message != "bought AAPL" {
			t.Errorf("Message = %q, want %q", got.Message, "bought AAPL")
		}
		if trade.lastBuy.Date == "" {
			t.Errorf("ExecuteBuy() date = %q, want today's date (non-empty)", trade.lastBuy.Date)
		}
		if trade.lastBuy.Ticker != "aapl" {
			t.Errorf("ExecuteBuy() ticker = %q, want %q (uppercasing is bot-layer's job)", trade.lastBuy.Ticker, "aapl")
		}
		// Phase 24 tech debt 3: ExecuteBuy no longer pushes to Telegram on
		// its own, so the handler must call Notify explicitly to preserve
		// the "web trade still gets a Telegram confirmation" decision.
		if len(trade.notified) != 1 || trade.notified[0] != "bought AAPL" {
			t.Errorf("Notify() calls = %v, want exactly [%q]", trade.notified, "bought AAPL")
		}
	})

	t.Run("rejects a malformed date", func(t *testing.T) {
		body, _ := json.Marshal(tradeRequest{Ticker: "AAPL", Shares: 10, Price: 200, Date: "07/01/2026"})
		req := httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("maps an executor error to 400 with its message", func(t *testing.T) {
		trade.buyErr = db.ErrNoPosition
		trade.buyMsg = "no position"
		defer func() { trade.buyErr = nil }()

		body, _ := json.Marshal(tradeRequest{Ticker: "AAPL", Shares: 10, Price: 200})
		req := httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got["error"] != "no position" {
			t.Errorf("error = %q, want %q", got["error"], "no position")
		}
		// Notify still fires on a failed trade, matching the pre-decoupling
		// behavior where ExecuteBuy sent the failure message too.
		if last := trade.notified[len(trade.notified)-1]; last != "no position" {
			t.Errorf("last Notify() call = %q, want %q (failures still notify)", last, "no position")
		}
	})
}

func TestHandleWatchlistAddRemove(t *testing.T) {
	wl := &fakeWatchlistDB{}
	s := newTradeTestServer("secret", &fakeTrade{}, wl)
	cookie := loginAndGetCookie(t, s, "secret")

	add := func(ticker string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(tickerRequest{Ticker: ticker})
		req := httptest.NewRequest(http.MethodPost, "/api/watchlist/add", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		return rec
	}
	remove := func(ticker string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(tickerRequest{Ticker: ticker})
		req := httptest.NewRequest(http.MethodPost, "/api/watchlist/remove", bytes.NewReader(body))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := add("nvda"); rec.Code != http.StatusOK {
		t.Fatalf("add status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if len(wl.added) != 1 || wl.added[0] != "NVDA" {
		t.Errorf("added = %v, want [NVDA] (uppercased)", wl.added)
	}

	if rec := remove("nvda"); rec.Code != http.StatusOK {
		t.Fatalf("remove status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if len(wl.removed) != 1 || wl.removed[0] != "NVDA" {
		t.Errorf("removed = %v, want [NVDA] (uppercased)", wl.removed)
	}

	if rec := add(""); rec.Code != http.StatusBadRequest {
		t.Errorf("add(\"\") status = %d, want 400", rec.Code)
	}
}

func TestHandleThesisSet(t *testing.T) {
	s := newTradeTestServer("secret", &fakeTrade{}, &fakeWatchlistDB{})
	ftd := s.thesisDB.(*fakeThesisDB)
	cookie := loginAndGetCookie(t, s, "secret")

	post := func(req thesisRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(req)
		r := httptest.NewRequest(http.MethodPost, "/api/thesis", bytes.NewReader(body))
		r.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, r)
		return rec
	}

	t.Run("saves and uppercases the ticker", func(t *testing.T) {
		rec := post(thesisRequest{Ticker: "aapl", Text: "  entering on the breakout  "})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		if ftd.lastTicker != "AAPL" || ftd.lastText != "entering on the breakout" {
			t.Errorf("SetThesis(%q, %q), want (AAPL, trimmed text)", ftd.lastTicker, ftd.lastText)
		}
	})

	t.Run("rejects blank text", func(t *testing.T) {
		rec := post(thesisRequest{Ticker: "AAPL", Text: "   "})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("rejects blank ticker", func(t *testing.T) {
		rec := post(thesisRequest{Ticker: "", Text: "some thesis"})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("requires auth", func(t *testing.T) {
		body, _ := json.Marshal(thesisRequest{Ticker: "AAPL", Text: "x"})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/thesis", bytes.NewReader(body)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

func TestNew_WriteRoutesGatedByPassword(t *testing.T) {
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	defer d.Close()

	buyBody, _ := json.Marshal(tradeRequest{Ticker: "AAPL", Shares: 1, Price: 1})

	t.Run("no password: write routes 404", func(t *testing.T) {
		s := New(Config{DB: d, Lang: i18n.EN})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(buyBody)))
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404 when WEB_PASSWORD is unset", rec.Code)
		}
	})

	t.Run("password set: write routes exist but require auth", func(t *testing.T) {
		s := New(Config{DB: d, Lang: i18n.EN, Password: "secret", Trade: &fakeTrade{}})
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trade/buy", bytes.NewReader(buyBody)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 (route exists, no cookie)", rec.Code)
		}
	})
}

// Phase 17 PR1: Telegram is optional, so an authenticated write can now
// reach a Server whose Trade is nil. The trade-backed routes must say so
// (409) instead of panicking, while the DB-backed writes on the same gate
// keep working — that split is the whole point of requireTrade covering
// only four of them.
func TestWriteRoutes_NoTelegramConfigured(t *testing.T) {
	s := newTradeTestServer("secret", nil, &fakeWatchlistDB{})
	cookie := loginAndGetCookie(t, s, "secret")

	post := func(t *testing.T, path string, body any) int {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, tc := range []struct {
		path string
		body any
	}{
		{"/api/trade/buy", tradeRequest{Ticker: "AAPL", Shares: 1, Price: 1}},
		{"/api/trade/sell", tradeRequest{Ticker: "AAPL", Shares: 1, Price: 1}},
		{"/api/stop", stopRequest{Ticker: "AAPL", Price: 1}},
		{"/api/buy-alerts/add", buyAlertRequest{Ticker: "AAPL", Price: 1}},
	} {
		if got := post(t, tc.path, tc.body); got != http.StatusConflict {
			t.Errorf("POST %s status = %d, want 409 when Trade is nil", tc.path, got)
		}
	}

	if got := post(t, "/api/watchlist/add", tickerRequest{Ticker: "AAPL"}); got != http.StatusOK {
		t.Errorf("POST /api/watchlist/add status = %d, want 200 (no TradeExecutor needed)", got)
	}
}
