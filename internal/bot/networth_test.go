package bot

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"argus/internal/db"
	"argus/internal/i18n"
)

func newNetworthTestBot(t *testing.T) (*Bot, *db.DB, *fakeChannel) {
	t.Helper()
	d, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.New() error = %v", err)
	}
	t.Cleanup(func() { d.Close() })
	ch := &fakeChannel{}
	return &Bot{db: d, lang: i18n.EN, provider: quoteProvider{}, channel: ch, now: time.Now}, d, ch
}

// TestHandleNetworthFreshInstallShowsRealZero pins the boundary of
// §8.17.1's degrade rule: zero assets is a genuine $0 net worth (same as
// /w's web page on a fresh install), not "unknown" — it must render the
// summary, never the KeyNetworthNoData message.
func TestHandleNetworthFreshInstallShowsRealZero(t *testing.T) {
	b, _, ch := newNetworthTestBot(t)
	b.handleNetworth()

	if len(ch.sent) != 1 {
		t.Fatalf("Send() calls = %d, want 1", len(ch.sent))
	}
	got := ch.sent[0]
	if !strings.Contains(got, "NT$0") {
		t.Errorf("summary = %q, want it to contain a real NT$0 net worth", got)
	}
	if strings.Contains(got, i18n.T(i18n.EN, i18n.KeyNetworthNoData)) {
		t.Errorf("summary = %q, an empty portfolio must not degrade to KeyNetworthNoData", got)
	}
}

// TestHandleNetworthDegradesWhenFXUnavailable covers the actual degrade
// case KeyNetworthNoData is for: a held currency's FX rate can't be
// resolved (no cache, and the live quote fetch fails), so the whole
// summary must degrade rather than silently pricing the USD asset as 0.
func TestHandleNetworthDegradesWhenFXUnavailable(t *testing.T) {
	b, d, ch := newNetworthTestBot(t)

	id, err := d.CreateAsset(db.NewAsset{Side: "asset", Type: "other", Name: "海外券商", AssetGroup: "growth", Currency: "USD"})
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := d.UpsertAssetSnapshot(db.AssetSnapshot{AssetID: id, Date: today, Value: 1000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() error = %v", err)
	}

	b.handleNetworth()

	if len(ch.sent) != 1 || ch.sent[0] != i18n.T(i18n.EN, i18n.KeyNetworthNoData) {
		t.Fatalf("Send() = %v, want one KeyNetworthNoData message", ch.sent)
	}
}

// TestHandleNetworthRendersTotalsAndDashesForMissingHealthData covers the
// common path: a TWD-only deposit asset gives a real net worth (no FX
// needed), but with no salary set (profile.annual_salary) and no month-ago
// snapshot, every health metric must render as "—" rather than a
// fabricated ratio.
func TestHandleNetworthRendersTotalsAndDashesForMissingHealthData(t *testing.T) {
	b, d, ch := newNetworthTestBot(t)

	id, err := d.CreateDepositAsset(db.NewAsset{Side: "asset", Type: "deposit", Name: "活存", AssetGroup: "liquid", Currency: "TWD"}, db.DepositDetails{})
	if err != nil {
		t.Fatalf("CreateDepositAsset() error = %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if err := d.UpsertAssetSnapshot(db.AssetSnapshot{AssetID: id, Date: today, Value: 100000}); err != nil {
		t.Fatalf("UpsertAssetSnapshot() error = %v", err)
	}

	b.handleNetworth()

	if len(ch.sent) != 1 {
		t.Fatalf("Send() calls = %d, want 1", len(ch.sent))
	}
	got := ch.sent[0]
	if !strings.Contains(got, "NT$100,000") {
		t.Errorf("summary = %q, want it to contain net worth NT$100,000", got)
	}
	if !strings.Contains(got, "—") {
		t.Errorf("summary = %q, want at least one \"—\" for missing health metrics", got)
	}
}
