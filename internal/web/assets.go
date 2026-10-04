package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

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
	// LogAssetValue/DeleteAssetSnapshot back the value-history drawer: log (or
	// correct today's) value, and undo today's. Both enforce db's "history is
	// read-only" rule; UpsertAssetSnapshot stays the unchecked create/import path.
	LogAssetValue(assetID int64, date string, value float64, today string) error
	DeleteAssetSnapshot(assetID int64, date, today string) error
	ArchiveAsset(id int64) error
	// UnarchiveAsset/UpdateAsset back the edit-asset flow: restoring a
	// soft-deleted asset and changing its name/group/venue/detail fields
	// (never value, type or currency — see db.AssetEdit).
	UnarchiveAsset(id int64) error
	UpdateAsset(id int64, e db.AssetEdit) error
	// SetSetting backs wealth_balance.go's profile.annual_salary write (the
	// health-metric ratios' one denominator, §9.3) — the same settings
	// table/method service.PortfolioService's cash_balance uses.
	SetSetting(key, value string) error
	// CreateRecurringCashflow/DeactivateRecurringCashflow back
	// wealth_cash.go's add-flow form and pause action (Phase 9 波次2 PR5).
	CreateRecurringCashflow(c db.NewRecurringCashflow) (int64, error)
	DeactivateRecurringCashflow(id int64) error
	// UpdateRecurringCashflow backs wealth_cash.go's edit route (name/amount/
	// day/category only — see db.RecurringCashflowEdit).
	UpdateRecurringCashflow(id int64, e db.RecurringCashflowEdit) error
	// CreateGoal/DeleteGoal/SetGoalAsset back wealth_goals.go's
	// create/delete/earmark write routes (Phase 9 波次3 PR6). The goals
	// drawer calls create/update/delete; earmark has no UI, the route stays
	// as API surface.
	CreateGoal(g db.NewGoal) (int64, error)
	// UpdateGoal backs the goals drawer's edit path (general goals only).
	UpdateGoal(id int64, g db.NewGoal) error
	DeleteGoal(id int64) error
	SetGoalAsset(goalID, assetID int64, ratio float64) error
	// UpsertRetirementGoal backs wealth_retire.go's quick-switch save (Phase
	// 9 波次3 PR7) — creates or updates the single kind="retirement" goal row.
	UpsertRetirementGoal(name string, targetAmount float64, targetDate string) (int64, error)
	// CreateInsuranceAsset backs wealth_insure.go's add-policy form (Phase 9
	// 波次3 PR8) — one call creates the asset + insurance_details +
	// insurance_coverages row atomically.
	CreateInsuranceAsset(a db.NewAsset, det db.InsuranceDetails, cov db.InsuranceCoverage) (int64, error)
	// CreateFundAsset backs wealth_import.go's fund-type CSV rows (Phase 9
	// 波次4 PR10) — /w/funds itself has no add form (§9.4 PR10: fund rows
	// only ever arrive via /w/import), so this is CSV-only, unlike the other
	// CreateXxxAsset methods above.
	CreateFundAsset(a db.NewAsset, det db.FundDetails) (int64, error)
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
// internal/service's balance-sheet/debt-payoff assembly). fundAssetTypes is
// the fund-side equivalent, used only by wealth_import.go — /w/funds has no
// quick-add drawer (§9.4 PR10), so handleWealthAssetCreate never checks it.
var depositAssetTypes = map[string]bool{"deposit": true}
var fundAssetTypes = map[string]bool{"fund": true}

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
	if !slices.Contains(assets.AssetGroups, req.AssetGroup) {
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
// balance sheet's in-place edit (wEditCell) and the drawer's "log new value".
// It writes today (or the given past date) with Source "manual", per §9.1
// rule 2: this writes asset_snapshots, never assets — an edit is "today's
// value is now X", not a correction to the asset's identity. A past date that
// already has a record is refused (409): history is read-only, see
// db.LogAssetValue.
func (s *Server) handleWealthAssetSnapshot(w http.ResponseWriter, r *http.Request) {
	var req wealthSnapshotRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "assetId is required")
		return
	}
	if req.Value < 0 {
		writeError(w, http.StatusBadRequest, "value can't be negative")
		return
	}
	today := time.Now().Format("2006-01-02")
	date, ok := resolveTradeDate(req.Date)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}
	if date > today {
		writeError(w, http.StatusBadRequest, "date can't be in the future")
		return
	}
	if !writeSnapshotErr(w, s.wealthDB.LogAssetValue(req.AssetID, date, req.Value, today), "save value") {
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
}

// writeSnapshotErr maps the snapshot write paths' db errors onto HTTP and
// reports whether err was nil (the caller carries on only then).
func writeSnapshotErr(w http.ResponseWriter, err error, what string) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, db.ErrAssetNotFound), errors.Is(err, db.ErrSnapshotNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, db.ErrAssetArchived), errors.Is(err, db.ErrSnapshotLocked):
		writeError(w, http.StatusConflict, err.Error())
	default:
		logger.Errorf("web: %s: %v", what, err)
		writeError(w, http.StatusInternalServerError, "failed to "+what)
	}
	return false
}

type wealthSnapshotDeleteRequest struct {
	AssetID int64  `json:"assetId"`
	Date    string `json:"date"`
}

// handleWealthAssetSnapshotDelete backs POST /api/wealth/assets/snapshot/delete
// — undoing a value logged by mistake. Only today's record can go (409
// otherwise); older ones are history.
func (s *Server) handleWealthAssetSnapshotDelete(w http.ResponseWriter, r *http.Request) {
	var req wealthSnapshotDeleteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "assetId is required")
		return
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		writeError(w, http.StatusBadRequest, "invalid date")
		return
	}
	today := time.Now().Format("2006-01-02")
	if !writeSnapshotErr(w, s.wealthDB.DeleteAssetSnapshot(req.AssetID, req.Date, today), "delete value record") {
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "deleted"})
}

type wealthSnapshotRow struct {
	Date   string  `json:"date"`
	Value  float64 `json:"value"`
	Source string  `json:"source"`
}

// handleWealthAssetHistory backs GET /api/wealth/assets/history?assetId= — one
// asset's value records, newest first, in the asset's own currency. today is
// the server's date, the one the write routes judge "today's record" by, so
// the drawer doesn't have to trust the browser's clock for it.
func (s *Server) handleWealthAssetHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("assetId"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "assetId is required")
		return
	}
	snaps, err := s.db.ListAssetSnapshots(id)
	if err != nil {
		logger.Errorf("web: list wealth asset history: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load value history")
		return
	}
	rows := make([]wealthSnapshotRow, 0, len(snaps))
	for _, sn := range snaps {
		rows = append(rows, wealthSnapshotRow{Date: sn.Date, Value: sn.Value, Source: sn.Source})
	}
	writeJSON(w, http.StatusOK, map[string]any{"today": time.Now().Format("2006-01-02"), "snapshots": rows})
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

// handleWealthAssetUnarchive backs POST /api/wealth/assets/unarchive — the
// undo of the archive route above, same request shape.
func (s *Server) handleWealthAssetUnarchive(w http.ResponseWriter, r *http.Request) {
	var req wealthArchiveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "assetId is required")
		return
	}
	if err := s.wealthDB.UnarchiveAsset(req.AssetID); err != nil {
		logger.Errorf("web: unarchive wealth asset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to unarchive asset")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "unarchived"})
}

// wealthAssetUpdateRequest is the edit-asset flow's submission. Side, type,
// currency and value are deliberately not here (see db.AssetEdit): value has
// its own snapshot route, the rest stay "archive and recreate". A deposit/
// loan block replaces that detail row wholesale, so the client sends every
// field it wants kept; omit the block to leave the details alone.
type wealthAssetUpdateRequest struct {
	AssetID    int64                      `json:"assetId"`
	Name       string                     `json:"name"`
	AssetGroup string                     `json:"assetGroup"`
	Venue      string                     `json:"venue"`
	Deposit    *wealthAssetDepositRequest `json:"deposit"`
	Loan       *wealthAssetLoanRequest    `json:"loan"`
}

// handleWealthAssetUpdate backs POST /api/wealth/assets/update. It never
// writes asset_snapshots, so editing an asset can't rewrite its value
// history.
func (s *Server) handleWealthAssetUpdate(w http.ResponseWriter, r *http.Request) {
	var req wealthAssetUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.AssetGroup = strings.TrimSpace(req.AssetGroup)
	if req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "assetId is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !slices.Contains(assets.AssetGroups, req.AssetGroup) {
		writeError(w, http.StatusBadRequest, "assetGroup must be one of liquid/growth/income/hard")
		return
	}
	if req.Deposit != nil && req.Loan != nil {
		writeError(w, http.StatusBadRequest, "send either deposit or loan details, not both")
		return
	}

	edit := db.AssetEdit{Name: req.Name, AssetGroup: req.AssetGroup, Venue: strings.TrimSpace(req.Venue)}
	if req.Deposit != nil {
		edit.Deposit = &db.DepositDetails{Bank: req.Deposit.Bank, AccountNote: req.Deposit.AccountNote}
	}
	if l := req.Loan; l != nil {
		if l.SecuredAssetID != nil && *l.SecuredAssetID == req.AssetID {
			writeError(w, http.StatusBadRequest, "a loan can't be secured by itself")
			return
		}
		edit.Loan = &db.LoanDetails{
			Lender: l.Lender, RatePct: l.RatePct, OriginalPrincipal: l.OriginalPrincipal,
			RemainingMonths: l.RemainingMonths, SecuredAssetID: l.SecuredAssetID,
		}
	}

	switch err := s.wealthDB.UpdateAsset(req.AssetID, edit); {
	case errors.Is(err, db.ErrAssetNotFound):
		writeError(w, http.StatusNotFound, "asset not found")
	case errors.Is(err, db.ErrAssetNoDetails):
		writeError(w, http.StatusBadRequest, "this asset has no such details to edit")
	case err != nil:
		logger.Errorf("web: update wealth asset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update asset")
	default:
		writeJSON(w, http.StatusOK, tradeResponse{Message: "saved"})
	}
}
