package web

import (
	"net/http"
	"strings"
	"time"

	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// handleWealthInsureGet backs GET /api/wealth/insure — the have-vs-need
// gap analysis (§8.16.1), computed by service.ComputeInsuranceGap.
func (s *Server) handleWealthInsureGet(w http.ResponseWriter, r *http.Request) {
	resp, err := service.ComputeInsuranceGap(s.db, s.fxDB, s.quotes, time.Now().Format("2006-01-02"))
	if err != nil {
		logger.Errorf("web: wealth insure: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// wealthInsureCreateRequest backs the add-policy form. PerPeriod is
// deliberately not a request field — it's derived from Kind server-side
// (service.InsuranceKindPerPeriod) so a coverage can never be filed under the wrong
// unit, unlike the design template's drawer which offers 保障類型/給付單位
// as two independent selects (§8.18.2) and trusts the combination.
type wealthInsureCreateRequest struct {
	Insurer       string   `json:"insurer"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Amount        float64  `json:"amount"`
	AnnualPremium *float64 `json:"annualPremium"`
	PremiumYears  *int64   `json:"premiumYears"`
	Insured       string   `json:"insured"`
}

func (s *Server) handleWealthInsureCreate(w http.ResponseWriter, r *http.Request) {
	var req wealthInsureCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Insurer, req.Name, req.Insured = strings.TrimSpace(req.Insurer), strings.TrimSpace(req.Name), strings.TrimSpace(req.Insured)
	perPeriod, validKind := service.InsuranceKindPerPeriod(req.Kind)
	if !validKind {
		writeError(w, http.StatusBadRequest, "kind is invalid")
		return
	}
	if req.Insurer == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "insurer and name are required")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	if req.AnnualPremium != nil && *req.AnnualPremium < 0 {
		writeError(w, http.StatusBadRequest, "annualPremium must be >= 0")
		return
	}
	if req.PremiumYears != nil && *req.PremiumYears < 0 {
		writeError(w, http.StatusBadRequest, "premiumYears must be >= 0")
		return
	}

	// Venue = insurer, per §8.18.2 note 4: the policy drawer's "保險公司"
	// field doubles as the asset's venue, it has no separate venue field.
	// AssetGroup is left "" — the policy drawer has no 歸類 select either
	// (§8.18.2's field table), and it's functionally inert here anyway: an
	// insurance-type asset never gets an asset_snapshots row (its "value"
	// is coverage amount, not market value), so no group-total ever reads it.
	a := db.NewAsset{Name: req.Name, Venue: req.Insurer, Currency: "TWD", Source: "manual"}
	det := db.InsuranceDetails{Insurer: req.Insurer, Insured: req.Insured, AnnualPremium: req.AnnualPremium, PremiumYears: req.PremiumYears}
	cov := db.InsuranceCoverage{Kind: req.Kind, Amount: req.Amount, PerPeriod: perPeriod}
	id, err := s.wealthDB.CreateInsuranceAsset(a, det, cov)
	if err != nil {
		logger.Errorf("web: create insurance asset: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}
