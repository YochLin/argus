package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"argus/internal/db"
)

type draftsResponse struct {
	Drafts   []assistantDraft   `json:"drafts"`
	Messages []assistantMessage `json:"messages"`
	Result   applyOutcome       `json:"result"`
}

func decodeDrafts(t *testing.T, rec *httptest.ResponseRecorder) draftsResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var r draftsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
	}
	return r
}

func addDraft(t *testing.T, d *db.DB, kind string, target *int64, payload string) int64 {
	t.Helper()
	id, err := d.AddWealthDraft(kind, target, payload)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func statuses(ds []assistantDraft) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Status)
	}
	return out
}

func TestWealthDrafts_ChatReportsTheTurnsDrafts(t *testing.T) {
	chat := &fakeChatter{reply: "Added to the drafts."}
	s, cookie := newAssistantTestServer(t, chat)
	d := s.assistantDB.(*db.DB)
	old := addDraft(t, d, "asset", nil, `{"type":"deposit","name":"Old","group":"liquid","currency":"TWD","value":1}`)
	var added int64
	chat.during = func() {
		added = addDraft(t, d, "asset", nil, `{"type":"loan","name":"Mortgage","group":"hard","currency":"TWD"}`)
	}

	r := decodeDrafts(t, postAssistant(t, s, cookie, "/api/wealth/assistant/chat", assistantChatRequest{Text: "I have a mortgage"}))
	if ai := r.Messages[len(r.Messages)-1]; !slices.Equal(ai.DraftIDs, []int64{added}) {
		t.Errorf("ai draftIds = %v, want only the one added this turn (%d), not %d", ai.DraftIDs, added, old)
	}
	if !slices.Equal(statuses(r.Drafts), []string{"ok", "pending"}) || !slices.Equal(r.Drafts[1].Missing, []string{"value"}) {
		t.Errorf("drafts = %+v, want ok + pending missing value", r.Drafts)
	}
	// The chips survive a reload.
	if got := getConversation(t, s); !slices.Equal(got[len(got)-1].DraftIDs, []int64{added}) {
		t.Errorf("stored ai message = %+v, want draftIds [%d]", got[len(got)-1], added)
	}
}

func TestWealthDrafts_EditDeleteClear(t *testing.T) {
	s, cookie := newAssistantTestServer(t, &fakeChatter{})
	d := s.assistantDB.(*db.DB)
	loan := addDraft(t, d, "asset", nil, `{"type":"loan","name":"Mortgage","group":"hard","currency":"TWD"}`)
	other := addDraft(t, d, "asset", nil, `{"type":"gold","name":"Gold","group":"hard","currency":"TWD","value":5}`)
	if err := d.SetWealthDraftError(other, "boom"); err != nil {
		t.Fatal(err)
	}

	r := decodeDrafts(t, postAssistant(t, s, cookie, "/api/wealth/assistant/drafts/update", map[string]any{
		"id": loan, "payload": map[string]any{"type": " LOAN", "name": "Mortgage", "group": "hard", "currency": "twd", "value": 6000000, "bogus": 1},
	}))
	if !slices.Equal(statuses(r.Drafts), []string{"ok", "error"}) || r.Drafts[1].Error != "boom" || !r.Drafts[1].Failed {
		t.Fatalf("after edit = %+v, want the loan ok and the failed one still an error", r.Drafts)
	}
	if string(r.Drafts[0].Payload) != `{"type":"loan","name":"Mortgage","group":"hard","currency":"TWD","value":6000000}` {
		t.Errorf("stored payload = %s, want normalized with unknown fields dropped", r.Drafts[0].Payload)
	}
	// Editing is the fix: it clears the failed write's error.
	r = decodeDrafts(t, postAssistant(t, s, cookie, "/api/wealth/assistant/drafts/update", map[string]any{
		"id": other, "payload": map[string]any{"type": "gold", "name": "Gold", "group": "hard", "currency": "TWD", "value": 6},
	}))
	if r.Drafts[1].Status != "ok" {
		t.Errorf("edited failed draft = %+v, want ok", r.Drafts[1])
	}

	r = decodeDrafts(t, postAssistant(t, s, cookie, "/api/wealth/assistant/drafts/delete", map[string]any{"id": loan}))
	if len(r.Drafts) != 1 || r.Drafts[0].ID != other {
		t.Errorf("after delete = %+v, want only %d", r.Drafts, other)
	}
	if rec := postAssistant(t, s, cookie, "/api/wealth/assistant/drafts/delete", map[string]any{"id": loan}); rec.Code != http.StatusNotFound {
		t.Errorf("deleting twice = %d, want 404", rec.Code)
	}
	if r = decodeDrafts(t, postAssistant(t, s, cookie, "/api/wealth/assistant/drafts/clear", nil)); len(r.Drafts) != 0 {
		t.Errorf("after clear = %+v, want none", r.Drafts)
	}
}

func TestWealthDrafts_Apply(t *testing.T) {
	s, cookie := newAssistantTestServer(t, &fakeChatter{})
	d := s.assistantDB.(*db.DB)

	if rec := postAssistant(t, s, cookie, "/api/wealth/assistant/apply", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("apply with no drafts = %d, want 400", rec.Code)
	}

	esun, err := d.CreateAsset(db.NewAsset{Side: "asset", Type: "deposit", Name: "Esun", AssetGroup: "liquid", Currency: "TWD"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertAssetSnapshot(db.AssetSnapshot{AssetID: esun, Date: "2026-01-02", Value: 100}); err != nil {
		t.Fatal(err)
	}
	addDraft(t, d, "asset", nil, `{"type":"loan","name":"Mortgage","group":"hard","currency":"TWD","value":6000000,"rate_pct":2.1}`)
	addDraft(t, d, "asset", nil, `{"type":"loan","name":"Car","group":"hard","currency":"TWD"}`) // pending: stays
	addDraft(t, d, "balance", &esun, `{"value":150}`)
	// History is read-only: a past day that already has a value is refused.
	failing := addDraft(t, d, "balance", &esun, `{"value":90,"date":"2026-01-02"}`)

	r := decodeDrafts(t, postAssistant(t, s, cookie, "/api/wealth/assistant/apply", nil))
	if r.Result != (applyOutcome{Applied: 2, Kept: 2}) {
		t.Errorf("result = %+v, want 2 applied, 2 kept", r.Result)
	}
	if len(r.Messages) != 1 || r.Messages[0].Role != "sys" || r.Messages[0].Result == nil || *r.Messages[0].Result != r.Result {
		t.Errorf("messages = %+v, want one sys message carrying the result", r.Messages)
	}
	if !slices.Equal(statuses(r.Drafts), []string{"pending", "error"}) || r.Drafts[1].ID != failing || r.Drafts[1].Error == "" || !r.Drafts[1].Failed {
		t.Errorf("left = %+v, want the pending car and the refused balance with its reason", r.Drafts)
	}

	list, err := d.ListAssetsWithValue(false)
	if err != nil {
		t.Fatal(err)
	}
	var mortgage *db.AssetWithValue
	for i, a := range list {
		if a.Name == "Mortgage" {
			mortgage = &list[i]
		}
		if a.ID == esun && (a.Value == nil || *a.Value != 150) {
			t.Errorf("Esun value = %v, want 150", a.Value)
		}
	}
	if mortgage == nil || mortgage.Side != "liability" || mortgage.Source != "import" || mortgage.Value == nil || *mortgage.Value != 6000000 {
		t.Fatalf("mortgage = %+v, want an imported liability worth 6000000", mortgage)
	}
	if det, err := d.GetLoanDetails(mortgage.ID); err != nil || det == nil || det.RatePct == nil || *det.RatePct != 2.1 {
		t.Errorf("mortgage loan details = %+v (%v), want rate 2.1", det, err)
	}
}
