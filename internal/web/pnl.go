package web

import (
	"argus/internal/service"
)

// DateValue is one point in a date-ordered time series (YYYY-MM-DD, a dollar amount).
// Re-exported from internal/service for web backwards compatibility.
type DateValue = service.DateValue

var (
	DailyPnL          = service.DailyPnL
	CumulativeCurve   = service.CumulativeCurve
	MaxDrawdownAbs    = service.MaxDrawdownAbs
	DrawdownSeries    = service.DrawdownSeries
	FilterSells       = service.FilterSells
	WinRate           = service.WinRate
	ProfitFactor      = service.ProfitFactor
	Expectancy        = service.Expectancy
	YTDStart          = service.YTDStart
	QTDStart          = service.QTDStart
	HTDStart          = service.HTDStart
	curveValueBefore  = service.CurveValueBefore
)

// PeriodReturnPct computes a period's cash-flow-neutral return %.
func PeriodReturnPct(curve []DateValue, periodStart string, baseline float64, haveBaseline bool) (pct float64, ok bool) {
	return service.CurvePeriodReturnPct(curve, periodStart, baseline, haveBaseline)
}
