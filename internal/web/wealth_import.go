package web

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"argus/internal/assets"
	"argus/internal/db"
	"argus/internal/logger"
)

// wealthImportRow is one parsed/annotated CSV row — Bank/AccountNote,
// Lender/RatePct/OriginalPrincipal/RemainingMonths, or FundCode/
// FundPlatform/FundMonthlyAmount/FundNextContributionDate are populated only
// when Type routes to that detail table (same depositAssetTypes/
// assets.LoanTypes/fundAssetTypes split handleWealthAssetCreate uses); an
// "other"-type row leaves them all zero/nil. Fund is the only type with no
// quick-add drawer equivalent — /w/funds has no add form (§9.4 PR10), CSV
// import is its one write path — but the columns still sit at the same
// fixed positions as the deposit/loan ones so the template stays one shape.
type wealthImportRow struct {
	Line     int     `json:"line"`
	Side     string  `json:"side"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Group    string  `json:"group"`
	Venue    string  `json:"venue,omitempty"`
	Currency string  `json:"currency,omitempty"`
	Value    float64 `json:"value"`
	Date     string  `json:"date"`

	Bank              string   `json:"bank,omitempty"`
	AccountNote       string   `json:"accountNote,omitempty"`
	Lender            string   `json:"lender,omitempty"`
	RatePct           *float64 `json:"ratePct,omitempty"`
	OriginalPrincipal *float64 `json:"originalPrincipal,omitempty"`
	RemainingMonths   *int64   `json:"remainingMonths,omitempty"`

	FundCode                 string   `json:"fundCode,omitempty"`
	FundPlatform             string   `json:"fundPlatform,omitempty"`
	FundMonthlyAmount        *float64 `json:"fundMonthlyAmount,omitempty"`
	FundNextContributionDate string   `json:"fundNextContributionDate,omitempty"`

	Status  string `json:"status"` // "ok" | "warning" | "duplicate" | "error" | "applied"
	Message string `json:"message"`
}

type wealthImportRequest struct {
	CSV    string `json:"csv"`
	DryRun bool   `json:"dryRun"`
}

type wealthImportResponse struct {
	Rows    []wealthImportRow `json:"rows"`
	Applied int               `json:"applied"`
	DryRun  bool              `json:"dryRun"`
}

// parseWealthImportCSV parses the initial-data-entry template (§8.15.1):
// side,type,name,group,venue,currency,value,date,bank,accountNote,lender,
// ratePct,originalPrincipal,remainingMonths,fundCode,fundPlatform,
// fundMonthlyAmount,fundNextContributionDate — the last ten columns are only
// read when Type routes to deposit_details/loan_details/fund_details, blank
// otherwise. A header row is always present and always skipped, same
// convention as parseImportCSV's trade template.
func parseWealthImportCSV(csvText string) ([]wealthImportRow, error) {
	reader := csv.NewReader(strings.NewReader(csvText))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) <= 1 {
		return nil, nil
	}

	col := func(rec []string, i int) string {
		if i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	rows := make([]wealthImportRow, 0, len(records)-1)
	for i, rec := range records[1:] {
		row := wealthImportRow{Line: i + 2} // 1-indexed, +1 for the skipped header
		if len(rec) < 8 {
			row.Status = "error"
			row.Message = "expected at least 8 columns: side,type,name,group,venue,currency,value,date[,bank,accountNote,lender,ratePct,originalPrincipal,remainingMonths]"
			rows = append(rows, row)
			continue
		}

		row.Side = strings.ToLower(col(rec, 0))
		row.Type = strings.ToLower(col(rec, 1))
		row.Name = col(rec, 2)
		row.Group = strings.ToLower(col(rec, 3))
		row.Venue = col(rec, 4)
		row.Currency = strings.ToUpper(col(rec, 5))

		if row.Side != "asset" && row.Side != "liability" {
			row.Status = "error"
			row.Message = "side must be \"asset\" or \"liability\": " + row.Side
			rows = append(rows, row)
			continue
		}
		if row.Type == "" || row.Name == "" {
			row.Status = "error"
			row.Message = "type and name are required"
			rows = append(rows, row)
			continue
		}
		if row.Group != "liquid" && row.Group != "growth" && row.Group != "income" && row.Group != "hard" {
			row.Status = "error"
			row.Message = "group must be one of liquid/growth/income/hard: " + row.Group
			rows = append(rows, row)
			continue
		}

		var perr error
		if row.Value, perr = strconv.ParseFloat(col(rec, 6), 64); perr != nil {
			row.Status = "error"
			row.Message = "invalid value: " + col(rec, 6)
			rows = append(rows, row)
			continue
		}
		date, ok := resolveTradeDate(col(rec, 7))
		if !ok {
			row.Status = "error"
			row.Message = "invalid date, expected YYYY-MM-DD: " + col(rec, 7)
			rows = append(rows, row)
			continue
		}
		row.Date = date

		switch {
		case depositAssetTypes[row.Type]:
			row.Bank = col(rec, 8)
			row.AccountNote = col(rec, 9)
		case assets.LoanTypes[row.Type]:
			row.Lender = col(rec, 10)
			if v := col(rec, 11); v != "" {
				f, perr := strconv.ParseFloat(v, 64)
				if perr != nil {
					row.Status = "error"
					row.Message = "invalid ratePct: " + v
					rows = append(rows, row)
					continue
				}
				row.RatePct = &f
			}
			if v := col(rec, 12); v != "" {
				f, perr := strconv.ParseFloat(v, 64)
				if perr != nil {
					row.Status = "error"
					row.Message = "invalid originalPrincipal: " + v
					rows = append(rows, row)
					continue
				}
				row.OriginalPrincipal = &f
			}
			if v := col(rec, 13); v != "" {
				n, perr := strconv.ParseInt(v, 10, 64)
				if perr != nil {
					row.Status = "error"
					row.Message = "invalid remainingMonths: " + v
					rows = append(rows, row)
					continue
				}
				row.RemainingMonths = &n
			}
		case fundAssetTypes[row.Type]:
			row.FundCode = col(rec, 14)
			row.FundPlatform = col(rec, 15)
			if v := col(rec, 16); v != "" {
				f, perr := strconv.ParseFloat(v, 64)
				if perr != nil {
					row.Status = "error"
					row.Message = "invalid fundMonthlyAmount: " + v
					rows = append(rows, row)
					continue
				}
				row.FundMonthlyAmount = &f
			}
			if v := col(rec, 17); v != "" {
				fundDate, fok := resolveTradeDate(v)
				if !fok {
					row.Status = "error"
					row.Message = "invalid fundNextContributionDate, expected YYYY-MM-DD: " + v
					rows = append(rows, row)
					continue
				}
				row.FundNextContributionDate = fundDate
			}
		}

		row.Status = "ok"
		rows = append(rows, row)
	}
	return rows, nil
}

// wealthAssetKey identifies "the same asset" for duplicate detection —
// assets have no natural unique key (unlike a trade's date+ticker+action+
// shares+price), so name+type (case-insensitive) is the closest available
// signal: a re-pasted batch shouldn't create a second "玉山活存"/deposit row.
func wealthAssetKey(name, typ string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + strings.ToLower(strings.TrimSpace(typ))
}

// annotateWealthImportRows flags rows that match an existing non-archived
// asset (by name+type) or an earlier row in the same batch as "duplicate" —
// skipped on apply, same as annotateImportRows' trade-duplicate handling.
// Rows already marked "error" by the parser are left untouched.
func annotateWealthImportRows(rows []wealthImportRow, existing []db.AssetWithValue) {
	seen := make(map[string]bool, len(existing))
	for _, a := range existing {
		seen[wealthAssetKey(a.Name, a.Type)] = true
	}

	seenInBatch := make(map[string]bool)
	for i := range rows {
		row := &rows[i]
		if row.Status == "error" {
			continue
		}
		key := wealthAssetKey(row.Name, row.Type)
		if seen[key] || seenInBatch[key] {
			row.Status = "duplicate"
			row.Message = "matches an existing asset (same name/type) — skipped"
			continue
		}
		seenInBatch[key] = true
	}
}

// applyWealthImportRows writes every "ok" row via the same
// CreateAsset/CreateDepositAsset/CreateLoanAsset + UpsertAssetSnapshot path
// handleWealthAssetCreate uses, with Source "import" (§9.1's one real
// producer of that value in this phase). A single row's write failure
// doesn't abort the batch — earlier writes already committed.
func applyWealthImportRows(w wealthWriter, rows []wealthImportRow) int {
	applied := 0
	for i := range rows {
		row := &rows[i]
		if row.Status != "ok" {
			continue
		}
		na := db.NewAsset{
			Side: row.Side, Type: row.Type, Name: row.Name, AssetGroup: row.Group,
			Venue: row.Venue, Currency: row.Currency, Source: "import",
		}

		var id int64
		var err error
		switch {
		case depositAssetTypes[row.Type]:
			id, err = w.CreateDepositAsset(na, db.DepositDetails{Bank: row.Bank, AccountNote: row.AccountNote})
		case assets.LoanTypes[row.Type]:
			id, err = w.CreateLoanAsset(na, db.LoanDetails{
				Lender: row.Lender, RatePct: row.RatePct,
				OriginalPrincipal: row.OriginalPrincipal, RemainingMonths: row.RemainingMonths,
			})
		case fundAssetTypes[row.Type]:
			id, err = w.CreateFundAsset(na, db.FundDetails{
				Code: row.FundCode, Platform: row.FundPlatform,
				MonthlyAmount: row.FundMonthlyAmount, NextContributionDate: row.FundNextContributionDate,
			})
		default:
			id, err = w.CreateAsset(na)
		}
		if err != nil {
			row.Status = "error"
			row.Message = err.Error()
			continue
		}
		if err := w.UpsertAssetSnapshot(db.AssetSnapshot{AssetID: id, Date: row.Date, Value: row.Value, Source: "import"}); err != nil {
			row.Status = "error"
			row.Message = "asset created but failed to record its value: " + err.Error()
			continue
		}
		row.Status = "applied"
		applied++
	}
	return applied
}

// handleWealthImport backs POST /api/wealth/import (dryRun=true previews
// without writing, dryRun=false applies) — same one-endpoint-for-both shape
// as handleImport's trade CSV.
func (s *Server) handleWealthImport(w http.ResponseWriter, r *http.Request) {
	var req wealthImportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rows, err := parseWealthImportCSV(req.CSV)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: wealth import: list existing assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to read existing assets")
		return
	}
	annotateWealthImportRows(rows, existing)

	applied := 0
	if !req.DryRun {
		applied = applyWealthImportRows(s.wealthDB, rows)
	}
	writeJSON(w, http.StatusOK, wealthImportResponse{Rows: rows, Applied: applied, DryRun: req.DryRun})
}
