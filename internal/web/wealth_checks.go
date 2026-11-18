package web

import (
	"net/http"
	"time"

	"argus/internal/logger"
	"argus/internal/service"
)

// handleWealthChecks backs GET /api/wealth/checks, the home page's
// "本月該做的事". The rules live in service.EvaluateWealthChecks; this only
// gathers their inputs from the same functions the wealth pages render, so an
// item and the page it links to never disagree about a number. An input that
// fails to load drops its rule (logged), it doesn't fail the list.
func (s *Server) handleWealthChecks(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	today := now.Format("2006-01-02")
	resp := service.WealthChecks{AsOf: today, Rules: service.CheckRuleCount, Items: []service.CheckItem{}}

	list, err := s.db.ListAssetsWithValue(false)
	if err != nil {
		logger.Errorf("web: wealth checks: list assets: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load assets")
		return
	}
	if len(list) == 0 {
		resp.New = true
		writeJSON(w, http.StatusOK, resp)
		return
	}

	var in service.CheckInputs
	var cardTWD float64
	for _, a := range list {
		if a.Side == "asset" {
			in.AssetCount++
		}
		if _, stale := staleInfo(a.Source, a.AsOf, now); stale {
			in.Stale = append(in.Stale, a.Name)
		}
		if a.Type != "credit_card" || a.Value == nil || *a.Value <= 0 {
			continue
		}
		rate, ok := service.RateToTWD(a.Currency, today, true, s.quotes, s.fxDB)
		if !ok || *a.Value*rate <= cardTWD {
			continue
		}
		cardTWD = *a.Value * rate
		in.CardBalance, in.CardName, in.CardRatePct = &cardTWD, a.Name, nil
		if det, err := s.db.GetLoanDetails(a.ID); err == nil && det != nil {
			in.CardRatePct = det.RatePct
		}
	}

	p := service.LoadWealthProfile(s.db)
	in.Dependents = p.Dependents
	in.HasSalary = p.AnnualSalary != nil && *p.AnnualSalary > 0
	in.HasAge = p.BirthYear != nil
	in.ModelPicked = p.AllocModel != nil

	hm := service.ComputeHealthMetrics(s.db, s.fxDB, s.quotes, now)
	in.LiquidityMonths, in.MonthlyExpense, in.DebtRatioPct = hm.LiquidityMonths, hm.MonthlyExpense, hm.DebtRatioPct

	if gap, err := service.ComputeInsuranceGap(s.db, s.fxDB, s.quotes, today); err != nil {
		logger.Errorf("web: wealth checks: insurance: %v", err)
	} else {
		in.CoverageKind, in.CoveragePct, in.CoverageGap = gap.WorstKind, gap.WorstPct, gap.WorstGap
	}

	if plan, err := service.ComputeRetirementPlan(s.db, s.fxDB, s.quotes, now, service.RetirementOverrides{}); err != nil {
		logger.Errorf("web: wealth checks: retirement: %v", err)
	} else if plan.HasBirthYear && plan.Baseline != nil {
		pct := plan.Baseline.AchievementPct
		in.RetirementPct, in.RetirementGap = &pct, plan.Baseline.GapAmount
	}

	if goals, err := service.ComputeGoals(s.db, s.fxDB, s.quotes, now); err != nil {
		logger.Errorf("web: wealth checks: goals: %v", err)
	} else {
		for _, g := range goals.Goals {
			if g.Status == "behind" {
				in.GoalsBehind++
			}
		}
	}

	if in.ModelPicked {
		if categories, _, total, ok, err := s.allocCategories(today, now); err != nil {
			logger.Errorf("web: wealth checks: alloc: %v", err)
		} else if ok && total > 0 {
			_, orders := categoryDrift(categories, total, *p.AllocModel)
			var sum float64
			for _, o := range orders {
				sum += o.Amount
			}
			in.OffTarget, in.Rebalance = len(orders), &sum
		}
	}

	if flows, err := s.db.ListRecurringCashflows(true); err != nil {
		logger.Errorf("web: wealth checks: cash flows: %v", err)
	} else if inTWD, outTWD, ok := service.CashflowTotalsTWD(flows, today, s.quotes, s.fxDB); ok && inTWD > 0 {
		rate := (inTWD - outTWD) / inTWD * 100
		in.SavingsRatePct = &rate
	}

	resp.Items = service.EvaluateWealthChecks(in)
	writeJSON(w, http.StatusOK, resp)
}
