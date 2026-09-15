package web

import (
	"net/http"
	"strings"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
)

// wealthWriter is web's narrow view of *db.DB for Phase 9's wealth-platform
// writes (docs/phase-9-asset-platform.md §9) — the quick-add drawer's
// create, the balance-sheet's in-place edit, and soft-delete. Reads go
// through dbReader's ListAssetsWithValue instead, same GET-vs-write split as
// researchNotesWriter/thesisWriter.
type wealthWriter interface {
	CreateAsset(a db.NewAsset) (int64, error)
	CreateDepositAsset(a db.NewAsset, det db.DepositDetails) (int64, error)
	CreateLoanAsset(a db.NewAsset, det db.LoanDetails) (int64, error)
	UpsertAssetSnapshot(s db.AssetSnapshot) error
	ArchiveAsset(id int64) error
	// SetSetting backs wealth_balance.go's profile.annual_salary write (the
	// health-metric ratios' one denominator, §9.3) — the same settings
	// table/method service.PortfolioService's cash_balance uses.
	SetSetting(key, value string) error
}

// assetResponse mirrors db.AssetWithValue for JSON — Value/Cost stay nil
// (omitted as null, not 0) when the asset has no snapshot yet, per
// AssetWithValue's doc comment: a freshly created asset must render "—".
type assetResponse struct {
	ID         int64    `json:"id"`
	Side       string   `json:"side"`
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	AssetGroup string   `json:"assetGroup"`
	Venue      string   `json:"venue"`
	Currency   string   `json:"currency"`
	Source     string   `json:"source"`
	CreatedAt  string   `json:"createdAt"`
	ArchivedAt string   `json:"archivedAt,omitempty"`
	Value      *float64 `json:"value"`
	Cost       *float64 `json:"cost,omitempty"`
	AsOf       string   `json:"asOf,omitempty"`
}

func toAssetResponse(a db.AssetWithValue) assetResponse {
	return assetResponse{
		ID: a.ID, Side: a.Side, Type: a.Type, Name: a.Name, AssetGroup: a.AssetGroup,
		Venue: a.Venue, Currency: a.Currency, Source: a.Source, CreatedAt: a.CreatedAt,
		ArchivedAt: a.ArchivedAt, Value: a.Value, Cost: a.Cost, AsOf: a.AsOf,
	}
}

// handleWealthAssetsList backs GET /api/wealth/assets?archived=1 — ungated,
// same "reads don't need auth" convention as every other GET route
// (docs/phase-10-web-trade-input.md §4.1).
func (s *Server) handleWealthAssetsList(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthAssetsList: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	includeArchived := r.URL.Query().Get("archived") == "1"
	assets, err := s.db.ListAssetsWithValue(includeArchived)
	if err != nil {
		logger.Errorf("web: list wealth assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load assets")
		return
	}
	resp := make([]assetResponse, 0, len(assets))
	for _, a := range assets {
		resp = append(resp, toAssetResponse(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": resp})
}

// wealthAssetDepositRequest/wealthAssetLoanRequest are the quick-add
// drawer's per-type detail fields (§8.14.1) — a create request carries at
// most one of these, chosen by matching Type against depositAssetTypes/
// loanAssetTypes below.
type wealthAssetDepositRequest struct {
	Bank        string `json:"bank"`
	AccountNote string `json:"accountNote"`
}

type wealthAssetLoanRequest struct {
	Lender            string   `json:"lender"`
	RatePct           *float64 `json:"ratePct"`
	OriginalPrincipal *float64 `json:"originalPrincipal"`
	RemainingMonths   *int64   `json:"remainingMonths"`
	SecuredAssetID    *int64   `json:"securedAssetId"`
}

// wealthAssetCreateRequest is the quick-add drawer's "填表" step submission
// — spine fields plus the day-one value (the drawer's isMoney field), so
// creation always leaves the asset with a real snapshot rather than a blank
// one the balance sheet would render as "—" for no reason.
type wealthAssetCreateRequest struct {
	Side         string                     `json:"side"`
	Type         string                     `json:"type"`
	Name         string                     `json:"name"`
	AssetGroup   string                     `json:"assetGroup"`
	Venue        string                     `json:"venue"`
	Currency     string                     `json:"currency"`
	InitialValue float64                    `json:"initialValue"`
	Date         string                     `json:"date"`
	Deposit      *wealthAssetDepositRequest `json:"deposit"`
	Loan         *wealthAssetLoanRequest    `json:"loan"`
}

// depositAssetTypes picks which detail table a create request writes to;
// the loan-side equivalent is assets.LoanTypes (shared with
// internal/service's balance-sheet/debt-payoff assembly).
var depositAssetTypes = map[string]bool{"deposit": true}

// handleWealthAssetCreate backs POST /api/wealth/assets — the quick-add
// drawer's single write path, gated like every other write route. It always
// writes an initial asset_snapshots row in the same request as the spine
// (see wealthAssetCreateRequest's doc comment); that write isn't wrapped in
// the same DB transaction as the spine+detail insert (db.CreateDepositAsset/
// CreateLoanAsset already commit on their own), so a snapshot-write failure
// after a successful create leaves an asset with no value yet — recoverable
// by the balance sheet's in-place edit, not worth a bigger transaction for a
// single-user tool.
func (s *Server) handleWealthAssetCreate(w http.ResponseWriter, r *http.Request) {
	var req wealthAssetCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Side = strings.TrimSpace(req.Side)
	req.Type = strings.TrimSpace(req.Type)
	req.Name = strings.TrimSpace(req.Name)
	req.AssetGroup = strings.TrimSpace(req.AssetGroup)
	if req.Side != "asset" && req.Side != "liability" {
		writeError(w, http.StatusBadRequest, "side must be \"asset\" or \"liability\"")
		return
	}
	if req.Type == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "type and name are required")
		return
	}
	if req.AssetGroup != "liquid" && req.AssetGroup != "growth" && req.AssetGroup != "income" && req.AssetGroup != "hard" {
		writeError(w, http.StatusBadRequest, "assetGroup must be one of liquid/growth/income/hard")
		return
	}
	date, ok := resolveTradeDate(req.Date)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}

	na := db.NewAsset{
		Side: req.Side, Type: req.Type, Name: req.Name, AssetGroup: req.AssetGroup,
		Venue: strings.TrimSpace(req.Venue), Currency: strings.TrimSpace(req.Currency), Source: "manual",
	}

	var id int64
	var err error
	switch {
	case depositAssetTypes[req.Type]:
		det := db.DepositDetails{}
		if req.Deposit != nil {
			det = db.DepositDetails{Bank: req.Deposit.Bank, AccountNote: req.Deposit.AccountNote}
		}
		id, err = s.wealthDB.CreateDepositAsset(na, det)
	case assets.LoanTypes[req.Type]:
		det := db.LoanDetails{}
		if req.Loan != nil {
			det = db.LoanDetails{
				Lender: req.Loan.Lender, RatePct: req.Loan.RatePct,
				OriginalPrincipal: req.Loan.OriginalPrincipal, RemainingMonths: req.Loan.RemainingMonths,
				SecuredAssetID: req.Loan.SecuredAssetID,
			}
		}
		id, err = s.wealthDB.CreateLoanAsset(na, det)
	default:
		id, err = s.wealthDB.CreateAsset(na)
	}
	if err != nil {
		logger.Errorf("web: create wealth asset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create asset")
		return
	}

	if err := s.wealthDB.UpsertAssetSnapshot(db.AssetSnapshot{AssetID: id, Date: date, Value: req.InitialValue, Source: "manual"}); err != nil {
		logger.Errorf("web: create wealth asset initial snapshot: %v", err)
		writeError(w, http.StatusInternalServerError, "asset created but failed to record its initial value")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

type wealthSnapshotRequest struct {
	AssetID int64   `json:"assetId"`
	Date    string  `json:"date"`
	Value   float64 `json:"value"`
}

// handleWealthAssetSnapshot backs POST /api/wealth/assets/snapshot — the
// balance sheet's in-place edit (wEditCell). It always upserts today (or
// the given date) with Source "manual", per §9.1 rule 2: this writes
// asset_snapshots, never assets — an edit is "today's value is now X", not
// a correction to the asset's identity.
func (s *Server) handleWealthAssetSnapshot(w http.ResponseWriter, r *http.Request) {
	var req wealthSnapshotRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "assetId is required")
		return
	}
	date, ok := resolveTradeDate(req.Date)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}
	if err := s.wealthDB.UpsertAssetSnapshot(db.AssetSnapshot{AssetID: req.AssetID, Date: date, Value: req.Value, Source: "manual"}); err != nil {
		logger.Errorf("web: upsert wealth asset snapshot: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save value")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
}

type wealthArchiveRequest struct {
	AssetID int64 `json:"assetId"`
}

// handleWealthAssetArchive backs POST /api/wealth/assets/archive — soft
// delete only, per §9.1 (hard delete would orphan snapshot history).
func (s *Server) handleWealthAssetArchive(w http.ResponseWriter, r *http.Request) {
	var req wealthArchiveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "assetId is required")
		return
	}
	if err := s.wealthDB.ArchiveAsset(req.AssetID); err != nil {
		logger.Errorf("web: archive wealth asset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to archive asset")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "archived"})
}
