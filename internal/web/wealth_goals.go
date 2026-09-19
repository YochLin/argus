package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// goalAssetItem is one earmarked asset on a goal's card, resolved with the
// asset's own name/venue so the page doesn't need a second lookup.
type goalAssetItem struct {
	AssetID int64    `json:"assetId"`
	Name    string   `json:"name"`
	Venue   string   `json:"venue,omitempty"`
	Ratio   float64  `json:"ratio"`
	Value   *float64 `json:"value"` // TWD, nil if the asset itself has no value yet or FX couldn't be priced
}

// goalItem backs one row of /w/goals — Saved/ProgressPct are nil together
// when any earmarked asset's currency couldn't be priced to TWD (§8.17.1's
// whole-metric-degrades rule, same as /w/alloc and /w/cash). Status/MarkPct
// are unset when TargetDate is unset — there's nothing to be ahead of or
// behind. MarkPct is where the design template's progress bar draws its
// "expected progress" tick (§8.9 point 5's 應有進度標記) — the straight-line
// expected position goalStatus classified Status against, not just the
// classification itself, so the bar can render the same reference point the
// status label describes.
type goalItem struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	TargetAmount float64  `json:"targetAmount"`
	Currency     string   `json:"currency"`
	TargetDate   string   `json:"targetDate,omitempty"`
	Note         string   `json:"note,omitempty"`
	Saved        *float64 `json:"saved"`
	ProgressPct  *float64 `json:"progressPct"`
	Status       string   `json:"status,omitempty"` // "ahead" | "onTrack" | "behind"
	MarkPct      *float64 `json:"markPct,omitempty"`
	// MonthlyContribution is set only for the kind="retirement" row, read
	// from the same profile.retirement_monthly_contribution setting /w/retire
	// uses (§9.2's goals table has no monthly-contribution column for general
	// goals — the design template's per-row "每月投入" figure is otherwise
	// unknown data, not a value this page just forgot to render).
	MonthlyContribution *float64        `json:"monthlyContribution,omitempty"`
	Assets              []goalAssetItem `json:"assets"`
}

type goalsResponse struct {
	AsOf  string     `json:"asOf"`
	Goals []goalItem `json:"goals"`
}

// goalStatus compares actual progress against a straight-line expectation
// from the goal's creation date to its target date — a pure function, no
// LLM involvement, same "health metrics never go through the LLM" rule as
// /w/alloc's drift math. A 5pp band around the expected line counts as
// on-track rather than flagging every goal as barely ahead/behind on
// floating-point noise. ok=false (both other return values unset) when
// there's no target date to compare against, or CreatedAt fails to parse.
func goalStatus(createdAt, targetDate string, progressPct float64, today time.Time) (status string, expectedPct float64, ok bool) {
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		created, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return "", 0, false
		}
	}
	target, err := time.Parse("2006-01-02", targetDate)
	if err != nil {
		return "", 0, false
	}
	total := target.Sub(created)
	if total <= 0 {
		return "", 0, false
	}
	elapsed := today.Sub(created)
	if elapsed < 0 {
		elapsed = 0
	}
	expectedPct = elapsed.Seconds() / total.Seconds() * 100
	switch {
	case progressPct >= expectedPct+5:
		status = "ahead"
	case progressPct <= expectedPct-5:
		status = "behind"
	default:
		status = "onTrack"
	}
	return status, expectedPct, true
}

// earmarkedSavedTWD sums one goal's earmarked assets converted to TWD as of
// today — nil (not a partial sum) if any earmarked asset's currency can't be
// priced, the same whole-metric-degrades rule as everywhere else in wealth.
// Shared by handleWealthGoalsList and the retirement page (PR7): the
// retirement pool is exactly this number for the kind="retirement" goal.
func (s *Server) earmarkedSavedTWD(earmarks []db.GoalAsset, assetByID map[int64]db.AssetWithValue, asOf string) (items []goalAssetItem, saved *float64) {
	items = []goalAssetItem{}
	var sum float64
	ok := true
	for _, ga := range earmarks {
		a, found := assetByID[ga.AssetID]
		row := goalAssetItem{AssetID: ga.AssetID, Ratio: ga.Ratio}
		if found {
			row.Name, row.Venue = a.Name, a.Venue
		}
		if found && a.Value != nil {
			rate, rok := service.RateToTWD(a.Currency, asOf, true, s.quotes, s.fxDB)
			if !rok {
				ok = false
			} else {
				v := *a.Value * ga.Ratio * rate
				row.Value = &v
				sum += v
			}
		}
		items = append(items, row)
	}
	if !ok {
		return items, nil
	}
	return items, &sum
}

// handleWealthGoalsList backs GET /api/wealth/goals — ungated read, same
// convention as every other wealth GET route.
func (s *Server) handleWealthGoalsList(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger.Errorf("web: panic in handleWealthGoalsList: %v", p)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
	}()

	now := time.Now()
	today := now.Format("2006-01-02")

	goals, err := s.db.ListGoals()
	if err != nil {
		logger.Errorf("web: wealth goals: list: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load goals")
		return
	}
	earmarks, err := s.db.ListAllGoalAssets()
	if err != nil {
		logger.Errorf("web: wealth goals: list earmarks: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load goals")
		return
	}
	assetList, err := s.db.ListAssetsWithValue(true)
	if err != nil {
		logger.Errorf("web: wealth goals: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load goals")
		return
	}
	assetByID := make(map[int64]db.AssetWithValue, len(assetList))
	for _, a := range assetList {
		assetByID[a.ID] = a
	}
	earmarksByGoal := make(map[int64][]db.GoalAsset, len(goals))
	for _, ga := range earmarks {
		earmarksByGoal[ga.GoalID] = append(earmarksByGoal[ga.GoalID], ga)
	}

	resp := goalsResponse{AsOf: today, Goals: []goalItem{}}
	for _, g := range goals {
		item := goalItem{
			ID: g.ID, Name: g.Name, Kind: g.Kind, TargetAmount: g.TargetAmount,
			Currency: g.Currency, TargetDate: g.TargetDate, Note: g.Note, Assets: []goalAssetItem{},
		}

		item.Assets, item.Saved = s.earmarkedSavedTWD(earmarksByGoal[g.ID], assetByID, today)

		if item.Saved != nil {
			saved := *item.Saved
			if g.TargetAmount > 0 {
				// Capped at 100 (matching the design template's goalRow, which
				// does the same) — an overfunded goal reads as "done", not as a
				// number that invites a second-guess about the math.
				pct := saved / g.TargetAmount * 100
				if pct > 100 {
					pct = 100
				}
				item.ProgressPct = &pct
				if g.TargetDate != "" {
					if status, expectedPct, ok := goalStatus(g.CreatedAt, g.TargetDate, pct, now); ok {
						item.Status = status
						item.MarkPct = &expectedPct
					}
				}
			}
		}
		if g.Kind == "retirement" {
			if raw, ok, err := s.db.GetSetting(retirementContribSettingKey); err == nil && ok {
				if v, perr := strconv.ParseFloat(raw, 64); perr == nil {
					item.MonthlyContribution = &v
				}
			}
		}
		resp.Goals = append(resp.Goals, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

type wealthGoalCreateRequest struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	TargetAmount float64 `json:"targetAmount"`
	Currency     string  `json:"currency"`
	TargetDate   string  `json:"targetDate"`
	Note         string  `json:"note"`
}

// handleWealthGoalCreate backs POST /api/wealth/goals — the add-goal form's
// write path, gated like every other wealth write route.
func (s *Server) handleWealthGoalCreate(w http.ResponseWriter, r *http.Request) {
	var req wealthGoalCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.TargetAmount <= 0 {
		writeError(w, http.StatusBadRequest, "name and a positive targetAmount are required")
		return
	}
	id, err := s.wealthDB.CreateGoal(db.NewGoal{
		Name: req.Name, Kind: strings.TrimSpace(req.Kind), TargetAmount: req.TargetAmount,
		Currency: strings.TrimSpace(req.Currency), TargetDate: strings.TrimSpace(req.TargetDate), Note: strings.TrimSpace(req.Note),
	})
	if err != nil {
		logger.Errorf("web: create goal: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create goal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

type wealthGoalDeleteRequest struct {
	ID int64 `json:"id"`
}

// handleWealthGoalDelete backs POST /api/wealth/goals/delete — a hard
// delete (goals have no soft-delete column, see db.DeleteGoal's doc
// comment), cascading to the goal's earmarks.
func (s *Server) handleWealthGoalDelete(w http.ResponseWriter, r *http.Request) {
	var req wealthGoalDeleteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ID <= 0 {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.wealthDB.DeleteGoal(req.ID); err != nil {
		logger.Errorf("web: delete goal: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete goal")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "deleted"})
}

type wealthGoalEarmarkRequest struct {
	GoalID  int64   `json:"goalId"`
	AssetID int64   `json:"assetId"`
	Ratio   float64 `json:"ratio"`
}

// handleWealthGoalEarmark backs POST /api/wealth/goals/earmark — sets (or,
// with ratio<=0, removes) one asset's earmark against one goal.
func (s *Server) handleWealthGoalEarmark(w http.ResponseWriter, r *http.Request) {
	var req wealthGoalEarmarkRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GoalID <= 0 || req.AssetID <= 0 {
		writeError(w, http.StatusBadRequest, "goalId and assetId are required")
		return
	}
	if req.Ratio < 0 || req.Ratio > 1 {
		writeError(w, http.StatusBadRequest, "ratio must be between 0 and 1")
		return
	}
	if err := s.wealthDB.SetGoalAsset(req.GoalID, req.AssetID, req.Ratio); err != nil {
		logger.Errorf("web: set goal earmark: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update earmark")
		return
	}
	writeJSON(w, http.StatusOK, tradeResponse{Message: "ok"})
}
