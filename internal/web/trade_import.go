package web

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
)

// csvWriter is web's narrow view of *db.DB for CSV import (Phase 5's
// optional §B item, docs/phase-5-web-dashboard.md) — the two weighted-
// average-cost writes, bypassing bot.ExecuteBuy/ExecuteSell's Telegram send
// + auto-watchlist-add + sell-review trigger: a bulk historical backfill
// isn't a live trade action, and firing one Telegram message per imported
// row would be spam. Same "core function, not handler" seam as watchlistWriter.
type csvWriter interface {
	RecordBuy(ticker string, shares, price, fee float64, date string) (db.Position, error)
	RecordSell(ticker string, shares, price, fee float64, date string) (db.Position, float64, error)
}

type importRow struct {
	Line    int     `json:"line"`
	Date    string  `json:"date"`
	Ticker  string  `json:"ticker"`
	Action  string  `json:"action"`
	Shares  float64 `json:"shares"`
	Price   float64 `json:"price"`
	Fee     float64 `json:"fee"`
	Status  string  `json:"status"` // "ok" | "warning" | "duplicate" | "error" | "applied"
	Message string  `json:"message"`
}

type importRequest struct {
	CSV    string `json:"csv"`
	DryRun bool   `json:"dryRun"`
}

type importResponse struct {
	Rows    []importRow `json:"rows"`
	Applied int         `json:"applied"`
	DryRun  bool        `json:"dryRun"`
}

// txKey is the duplicate-detection heuristic the design doc calls for —
// transactions has no unique key, so "same date+ticker+action+shares+price
// already on record" is the closest available signal.
func txKey(date, ticker, action string, shares, price float64) string {
	return fmt.Sprintf("%s|%s|%s|%.6f|%.6f", date, ticker, action, shares, price)
}

// parseImportCSV parses the generic date,ticker,action,shares,price,fee
// template into rows in file order. A header row is always present and
// always skipped (single-user tool, no need to sniff for one). Duplicate/
// warning detection and the oldest-to-newest apply order both happen later,
// since they need existing DB state this function doesn't have.
func parseImportCSV(csvText string) ([]importRow, error) {
	reader := csv.NewReader(strings.NewReader(csvText))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) <= 1 {
		return nil, nil
	}

	rows := make([]importRow, 0, len(records)-1)
	for i, rec := range records[1:] {
		row := importRow{Line: i + 2} // 1-indexed, +1 for the skipped header
		if len(rec) < 5 {
			row.Status = "error"
			row.Message = "expected at least 5 columns: date,ticker,action,shares,price[,fee]"
			rows = append(rows, row)
			continue
		}

		row.Date = strings.TrimSpace(rec[0])
		row.Ticker = strings.ToUpper(strings.TrimSpace(rec[1]))
		row.Action = strings.ToUpper(strings.TrimSpace(rec[2]))

		var perr error
		if row.Shares, perr = strconv.ParseFloat(strings.TrimSpace(rec[3]), 64); perr != nil {
			row.Status = "error"
			row.Message = "invalid shares: " + rec[3]
			rows = append(rows, row)
			continue
		}
		if row.Price, perr = strconv.ParseFloat(strings.TrimSpace(rec[4]), 64); perr != nil {
			row.Status = "error"
			row.Message = "invalid price: " + rec[4]
			rows = append(rows, row)
			continue
		}
		if len(rec) > 5 && strings.TrimSpace(rec[5]) != "" {
			if row.Fee, perr = strconv.ParseFloat(strings.TrimSpace(rec[5]), 64); perr != nil {
				row.Status = "error"
				row.Message = "invalid fee: " + rec[5]
				rows = append(rows, row)
				continue
			}
		}

		if _, perr = time.Parse("2006-01-02", row.Date); perr != nil {
			row.Status = "error"
			row.Message = "invalid date, expected YYYY-MM-DD: " + row.Date
			rows = append(rows, row)
			continue
		}
		if row.Ticker == "" || (row.Action != "BUY" && row.Action != "SELL") || row.Shares <= 0 || row.Price <= 0 || row.Fee < 0 {
			row.Status = "error"
			row.Message = "invalid row: ticker/action/shares/price/fee out of range"
			rows = append(rows, row)
			continue
		}

		row.Status = "ok"
		rows = append(rows, row)
	}
	return rows, nil
}

// annotateImportRows flags duplicates (already on record, see txKey) and
// out-of-order warnings (a row dated earlier than that ticker's latest
// existing transaction — the design doc's §B constraint: RecordSell bakes
// realized_pnl off avg_cost as it stands at call time, so a late-arriving
// earlier buy can't retroactively fix an already-computed sell) in place,
// leaving rows already marked "error" by the parser untouched.
func annotateImportRows(rows []importRow, existing []db.Transaction) {
	seen := make(map[string]bool, len(existing))
	latestByTicker := make(map[string]string)
	for _, tx := range existing {
		seen[txKey(tx.Date, tx.Ticker, tx.Side, tx.Shares, tx.Price)] = true
		if tx.Date > latestByTicker[tx.Ticker] {
			latestByTicker[tx.Ticker] = tx.Date
		}
	}

	seenInBatch := make(map[string]bool)
	for i := range rows {
		row := &rows[i]
		if row.Status == "error" {
			continue
		}
		key := txKey(row.Date, row.Ticker, row.Action, row.Shares, row.Price)
		if seen[key] || seenInBatch[key] {
			row.Status = "duplicate"
			row.Message = "matches an existing transaction (same date/ticker/action/shares/price) — skipped"
			continue
		}
		seenInBatch[key] = true
		if latest, ok := latestByTicker[row.Ticker]; ok && row.Date < latest {
			row.Status = "warning"
			row.Message = fmt.Sprintf(
				"dated before %s's latest recorded transaction (%s) — will still apply, but %s's realized P&L already on record won't be recomputed",
				row.Ticker, latest, row.Ticker,
			)
		}
	}
}

// applyImportRows writes every "ok"/"warning" row via RecordBuy/RecordSell
// in ascending date order — regardless of the rows slice's own order — per
// the design doc's §B hard constraint (RecordSell's realized_pnl depends on
// avg_cost as it stands at call time). Rows are updated in place to "applied"
// or "error" (with the write failure's message); "duplicate"/pre-existing
// "error" rows are left untouched. A single row's write failure doesn't
// abort the batch — earlier writes in the same call already committed and
// can't be un-applied by bailing out.
func applyImportRows(w csvWriter, rows []importRow) int {
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return rows[order[a]].Date < rows[order[b]].Date })

	applied := 0
	for _, idx := range order {
		row := &rows[idx]
		if row.Status != "ok" && row.Status != "warning" {
			continue
		}
		var err error
		if row.Action == "BUY" {
			_, err = w.RecordBuy(row.Ticker, row.Shares, row.Price, row.Fee, row.Date)
		} else {
			_, _, err = w.RecordSell(row.Ticker, row.Shares, row.Price, row.Fee, row.Date)
		}
		if err != nil {
			row.Status = "error"
			row.Message = err.Error()
			continue
		}
		row.Status = "applied"
		applied++
	}
	return applied
}

// handleImport backs POST /api/import (dryRun=true previews without
// writing, dryRun=false applies) — one endpoint for both, since preview and
// confirm run the identical parse+annotate pipeline and only differ in
// whether applyImportRows actually gets called.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rows, err := parseImportCSV(req.CSV)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := s.db.GetAllTransactions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read existing transactions")
		return
	}
	annotateImportRows(rows, existing)

	applied := 0
	if !req.DryRun {
		applied = applyImportRows(s.csvDB, rows)
	}
	writeJSON(w, http.StatusOK, importResponse{Rows: rows, Applied: applied, DryRun: req.DryRun})
}
