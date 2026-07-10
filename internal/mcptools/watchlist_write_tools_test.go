package mcptools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"argus/internal/db"
	"argus/internal/i18n"
)

func TestAddToWatchlist(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, writeDB: d, cache: newTTLCache()}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "add_to_watchlist", map[string]any{"ticker": "aapl"})
	if isError {
		t.Fatalf("add_to_watchlist returned an error result: %s", text)
	}
	if !strings.Contains(text, "AAPL") {
		t.Errorf("add_to_watchlist result missing ticker, got: %s", text)
	}

	got, err := d.GetWatchlist()
	if err != nil || len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("GetWatchlist() = %v, %v; want [AAPL], nil", got, err)
	}
}

func TestAddToWatchlistEmptyTicker(t *testing.T) {
	d := newTestDB(t)
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, writeDB: d}
	session := connectTool(t, ts)

	_, isError := callText(t, session, "add_to_watchlist", map[string]any{"ticker": "  "})
	if !isError {
		t.Fatal("add_to_watchlist with a blank ticker should return IsError")
	}
}

func TestAddToWatchlistInvalidatesGetWatchlistCache(t *testing.T) {
	// Same underlying file opened through two connections, mirroring
	// production: ts.db is the read-only connection get_watchlist reads
	// through, ts.writeDB is the separate writable one add_to_watchlist
	// writes through.
	path := filepath.Join(t.TempDir(), "test.db")
	rw, err := db.New(path)
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { rw.Close() })
	if err := rw.AddTicker("MSFT"); err != nil {
		t.Fatal(err)
	}

	ro, err := db.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("db.OpenReadOnly() error = %v", err)
	}
	t.Cleanup(func() { ro.Close() })

	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, db: ro, writeDB: rw, cache: newTTLCache()}
	session := connectTool(t, ts)

	// Prime the cache with the pre-add state.
	primed, isError := callText(t, session, "get_watchlist", map[string]any{})
	if isError {
		t.Fatalf("get_watchlist returned an error result: %s", primed)
	}
	if strings.Contains(primed, "AAPL") {
		t.Fatalf("get_watchlist should not see AAPL before it's added, got: %s", primed)
	}

	if _, isError := callText(t, session, "add_to_watchlist", map[string]any{"ticker": "AAPL"}); isError {
		t.Fatal("add_to_watchlist returned an error result")
	}

	after, isError := callText(t, session, "get_watchlist", map[string]any{})
	if isError {
		t.Fatalf("get_watchlist returned an error result: %s", after)
	}
	if !strings.Contains(after, "AAPL") {
		t.Errorf("get_watchlist after add_to_watchlist should see the new ticker (cache should have been invalidated), got: %s", after)
	}
}

func TestRemoveFromWatchlist(t *testing.T) {
	d := newTestDB(t)
	if err := d.AddTicker("AAPL"); err != nil {
		t.Fatal(err)
	}

	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}, writeDB: d, cache: newTTLCache()}
	session := connectTool(t, ts)

	text, isError := callText(t, session, "remove_from_watchlist", map[string]any{"ticker": "aapl"})
	if isError {
		t.Fatalf("remove_from_watchlist returned an error result: %s", text)
	}

	got, err := d.GetWatchlist()
	if err != nil || len(got) != 0 {
		t.Errorf("GetWatchlist() = %v, %v; want empty, nil", got, err)
	}
}

func TestWriteToolsNotRegisteredWithoutWriteDB(t *testing.T) {
	ts := &toolset{lang: i18n.EN, provider: &fakeProvider{}, history: &fakeHistory{}}
	session := connectTool(t, ts)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	names := make(map[string]bool)
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	for _, notWant := range []string{"add_to_watchlist", "remove_from_watchlist"} {
		if names[notWant] {
			t.Errorf("tools/list should not advertise %q when writeDB is nil", notWant)
		}
	}
}
