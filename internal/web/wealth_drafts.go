package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// The wealth assistant's drafts (Phase 28 B2, §5.1–5.2): the model proposes
// them through `argus mcp wealth`, the user edits, deletes and writes them
// here. A draft's status is computed on every read (service.CheckWealthDraft)
// — the same verdict the model gets — and a written draft is deleted.

// assistantDraft is one draft on the wire. Payload is the kind's JSON as
// stored (assets.AssetDraft / BalanceDraft, snake_case like the model's
// tools). Current is a balance draft's asset as it is now, for the card's
// "old → new".
type assistantDraft struct {
	ID       int64           `json:"id"`
	Kind     string          `json:"kind"`
	TargetID *int64          `json:"targetId,omitempty"`
	Payload  json.RawMessage `json:"payload"`
	service.DraftCheck
	Current   *draftAsset `json:"current,omitempty"`
	CreatedAt string      `json:"createdAt"`
}

type draftAsset struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Currency string   `json:"currency"`
	Value    *float64 `json:"value"`
}

// applyOutcome is a sys message's result: Applied drafts written, Kept left
// in the drafts (failed, pending or invalid).
type applyOutcome struct {
	Applied int `json:"applied"`
	Kept    int `json:"kept"`
}

func today() string { return time.Now().Format("2006-01-02") }

// draftViews renders drafts with their computed status.
func draftViews(drafts []db.WealthDraft, existing []db.AssetWithValue) []assistantDraft {
	day := today()
	out := make([]assistantDraft, 0, len(drafts))
	for _, d := range drafts {
		v := assistantDraft{
			ID: d.ID, Kind: d.Kind, TargetID: d.TargetID, Payload: json.RawMessage(d.Payload),
			DraftCheck: service.CheckWealthDraft(d, existing, day), CreatedAt: d.CreatedAt,
		}
		if d.TargetID != nil {
			if a := service.FindAsset(existing, *d.TargetID); a != nil {
				v.Current = &draftAsset{Name: a.Name, Type: a.Type, Currency: a.Currency, Value: a.Value}
			}
		}
		out = append(out, v)
	}
	return out
}

// loadDrafts reads every draft with its status, writing the error response
// itself on failure.
func (s *Server) loadDrafts(w http.ResponseWriter) ([]assistantDraft, bool) {
	drafts, existing, ok := s.draftsAndAssets(w)
	if !ok {
		return nil, false
	}
	return draftViews(drafts, existing), true
}

func (s *Server) draftsAndAssets(w http.ResponseWriter) ([]db.WealthDraft, []db.AssetWithValue, bool) {
	drafts, err := s.assistantDB.ListWealthDrafts()
	if err != nil {
		logger.Errorf("wealth drafts: list: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load the drafts")
		return nil, nil, false
	}
	existing, err := s.assistantDB.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("wealth drafts: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load the drafts")
		return nil, nil, false
	}
	return drafts, existing, true
}

func (s *Server) writeDrafts(w http.ResponseWriter) {
	if drafts, ok := s.loadDrafts(w); ok {
		writeJSON(w, http.StatusOK, map[string]any{"drafts": drafts})
	}
}

type draftUpdateRequest struct {
	ID      int64           `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// handleWealthDraftUpdate backs the edit drawer: it replaces the payload
// (and so clears a failed write's error). An incomplete or invalid payload is
// still saved — the card then shows what is missing or wrong.
func (s *Server) handleWealthDraftUpdate(w http.ResponseWriter, r *http.Request) {
	var req draftUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	drafts, err := s.assistantDB.ListWealthDrafts()
	if err != nil {
		logger.Errorf("wealth drafts: list: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load the drafts")
		return
	}
	var kind string
	for _, d := range drafts {
		if d.ID == req.ID {
			kind = d.Kind
		}
	}
	// Decode into the kind's type and re-encode, so only its fields are kept.
	var payload any
	switch kind {
	case "":
		writeError(w, http.StatusNotFound, db.ErrDraftNotFound.Error())
		return
	case assets.DraftAsset:
		var d assets.AssetDraft
		if err := json.Unmarshal(req.Payload, &d); err != nil {
			writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		d.Normalize()
		payload = d
	case assets.DraftBalance:
		var d assets.BalanceDraft
		if err := json.Unmarshal(req.Payload, &d); err != nil {
			writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		payload = d
	case assets.DraftCashflow:
		var d assets.CashflowDraft
		if err := json.Unmarshal(req.Payload, &d); err != nil {
			writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		d.Normalize()
		payload = d
	case assets.DraftInsurance:
		var d assets.InsuranceDraft
		if err := json.Unmarshal(req.Payload, &d); err != nil {
			writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		d.Normalize()
		payload = d
	default:
		writeError(w, http.StatusBadRequest, "this draft can't be edited")
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the draft")
		return
	}
	if !draftWriteOK(w, s.assistantDB.UpdateWealthDraft(req.ID, string(b))) {
		return
	}
	s.writeDrafts(w)
}

type draftIDRequest struct {
	ID int64 `json:"id"`
}

func (s *Server) handleWealthDraftDelete(w http.ResponseWriter, r *http.Request) {
	var req draftIDRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !draftWriteOK(w, s.assistantDB.DeleteWealthDraft(req.ID)) {
		return
	}
	s.writeDrafts(w)
}

func (s *Server) handleWealthDraftClear(w http.ResponseWriter, r *http.Request) {
	if !draftWriteOK(w, s.assistantDB.ClearWealthDrafts()) {
		return
	}
	s.writeDrafts(w)
}

func draftWriteOK(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, db.ErrDraftNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		logger.Errorf("wealth drafts: write: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save the draft")
	}
	return false
}

// handleWealthDraftApply is 確認寫入: every draft whose status is ok is
// written through the same path as the manual forms, one at a time (§4 point
// 5 — the db writes each commit on their own, so a batch can't be all or
// nothing). A written draft is deleted; a failed one keeps the reason. A sys
// message records the outcome in the conversation. It waits for no turn in
// progress: the model may be replacing a draft this would write.
func (s *Server) handleWealthDraftApply(w http.ResponseWriter, r *http.Request) {
	if !s.assistantMu.TryLock() {
		writeError(w, http.StatusConflict, "the assistant is still answering")
		return
	}
	defer s.assistantMu.Unlock()

	drafts, existing, ok := s.draftsAndAssets(w)
	if !ok {
		return
	}
	views := draftViews(drafts, existing)
	if !anyOK(views) {
		writeError(w, http.StatusBadRequest, "no draft is ready to save")
		return
	}
	var out applyOutcome
	for i, d := range drafts {
		if views[i].Status != "ok" {
			out.Kept++
			continue
		}
		if err := s.applyDraft(d); err != nil {
			logger.Errorf("wealth drafts: apply %d: %v", d.ID, err)
			if err := s.assistantDB.SetWealthDraftError(d.ID, err.Error()); err != nil {
				logger.Errorf("wealth drafts: record apply error %d: %v", d.ID, err)
			}
			out.Kept++
			continue
		}
		if err := s.assistantDB.DeleteWealthDraft(d.ID); err != nil {
			// Written but still listed: the next apply would write it twice.
			// The duplicate-name check stops an asset; say so in the log.
			logger.Errorf("wealth drafts: delete applied draft %d: %v", d.ID, err)
		}
		out.Applied++
	}

	meta, _ := json.Marshal(assistantMeta{Result: &out})
	added, err := s.assistantDB.AddAssistantMessages([]db.AssistantMessage{{Role: "sys", Meta: string(meta)}})
	if err != nil {
		logger.Errorf("wealth drafts: save sys message: %v", err)
		added = nil // the writes happened; the note is the only loss
	}
	left, ok := s.loadDrafts(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": out, "messages": toAssistantMessages(added), "drafts": left})
}

func anyOK(views []assistantDraft) bool {
	for _, v := range views {
		if v.Status == "ok" {
			return true
		}
	}
	return false
}

// applyDraft writes one ok draft. A new asset goes through the CSV import's
// write (Source "import", §5) so detail rows land exactly as an imported one
// would; a balance goes through LogAssetValue, the value drawer's path; a
// cash flow through CreateRecurringCashflow, /w/cash's add form's; a policy
// through CreateInsuranceAsset, /w/insure's.
func (s *Server) applyDraft(d db.WealthDraft) error {
	switch d.Kind {
	case assets.DraftAsset:
		var a assets.AssetDraft
		if err := json.Unmarshal([]byte(d.Payload), &a); err != nil {
			return err
		}
		date, _ := resolveTradeDate(a.Date)
		rows := []wealthImportRow{{
			Status: "ok", Side: a.Side(), Type: a.Type, Name: a.Name, Group: a.Group,
			Venue: a.Venue, Currency: a.Currency, Value: *a.Value, Date: date,
			Bank: a.Bank, AccountNote: a.AccountNote,
			Lender: a.Lender, RatePct: a.RatePct, OriginalPrincipal: a.OriginalPrincipal, RemainingMonths: a.RemainingMonths,
			FundCode: a.FundCode, FundPlatform: a.FundPlatform, FundMonthlyAmount: a.FundMonthlyAmount, FundNextContributionDate: a.FundNextContributionDate,
		}}
		applyWealthImportRows(s.wealthDB, rows)
		if rows[0].Status != "applied" {
			return errors.New(rows[0].Message)
		}
		return nil
	case assets.DraftBalance:
		var b assets.BalanceDraft
		if err := json.Unmarshal([]byte(d.Payload), &b); err != nil {
			return err
		}
		day := today()
		date := b.Date
		if date == "" {
			date = day
		}
		return s.wealthDB.LogAssetValue(*d.TargetID, date, *b.Value, day)
	case assets.DraftCashflow:
		var c assets.CashflowDraft
		if err := json.Unmarshal([]byte(d.Payload), &c); err != nil {
			return err
		}
		_, err := s.wealthDB.CreateRecurringCashflow(db.NewRecurringCashflow{
			Direction: c.Direction, Name: c.Name, Amount: *c.Amount, Currency: c.Currency,
			DayOfMonth: c.DayOfMonth, Category: c.Category,
		})
		return err
	case assets.DraftInsurance:
		var p assets.InsuranceDraft
		if err := json.Unmarshal([]byte(d.Payload), &p); err != nil {
			return err
		}
		// Venue = insurer and no group, as handleWealthInsureCreate does.
		perPeriod, _ := assets.InsurancePerPeriod(p.Kind)
		_, err := s.wealthDB.CreateInsuranceAsset(
			db.NewAsset{Name: p.Name, Venue: p.Insurer, Currency: "TWD", Source: "import"},
			db.InsuranceDetails{Insurer: p.Insurer, Insured: p.Insured, AnnualPremium: p.AnnualPremium, PremiumYears: p.PremiumYears},
			db.InsuranceCoverage{Kind: p.Kind, Amount: *p.Amount, PerPeriod: perPeriod},
		)
		return err
	}
	return fmt.Errorf("unknown draft kind %q", d.Kind)
}
