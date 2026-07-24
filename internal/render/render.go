// Package render holds Telegram/chat-facing text formatting shared between
// internal/bot and internal/mcptools. It depends only on internal/data and
// internal/i18n so internal/mcptools (which must not import internal/bot)
// can use it too — see docs/refactor-internal-bot.md.
package render

import (
	"strconv"
	"strings"

	"argus/internal/data"
	"argus/internal/i18n"
)

// Fundamentals renders the full Fundamentals struct field-by-field.
func Fundamentals(lang i18n.Lang, fd *data.Fundamentals) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyValuationHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyPE, fd.PE))
	sb.WriteString(i18n.T(lang, i18n.KeyPB, fd.PB))
	sb.WriteString(i18n.T(lang, i18n.KeyPS, fd.PS))
	sb.WriteString(i18n.T(lang, i18n.KeyMarketCap, Commaf(fd.MarketCapMillion)))
	sb.WriteString(i18n.T(lang, i18n.KeyBeta, fd.Beta))
	sb.WriteString(i18n.T(lang, i18n.Key52Week, fd.Week52High, fd.Week52Low))

	sb.WriteString(i18n.T(lang, i18n.KeyProfitabilityHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyROE, fd.ROE))
	sb.WriteString(i18n.T(lang, i18n.KeyROA, fd.ROA))
	sb.WriteString(i18n.T(lang, i18n.KeyGrossMargin, fd.GrossMarginPct))
	sb.WriteString(i18n.T(lang, i18n.KeyOperatingMargin, fd.OperatingMarginPct))
	sb.WriteString(i18n.T(lang, i18n.KeyNetMargin, fd.NetMarginPct))

	sb.WriteString(i18n.T(lang, i18n.KeyFinStructureHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyDebtToEquity, fd.DebtToEquity))
	sb.WriteString(i18n.T(lang, i18n.KeyCurrentRatio, fd.CurrentRatio))
	sb.WriteString(i18n.T(lang, i18n.KeyQuickRatio, fd.QuickRatio))

	sb.WriteString(i18n.T(lang, i18n.KeyGrowthHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyRevenueGrowth, fd.RevenueGrowthYoY))
	sb.WriteString(i18n.T(lang, i18n.KeyEPSGrowth, fd.EPSGrowthYoY))
	sb.WriteString(i18n.T(lang, i18n.KeyEPS, fd.EPS))
	sb.WriteString(i18n.T(lang, i18n.KeyBookValue, fd.BookValuePerShare))
	sb.WriteString(i18n.T(lang, i18n.KeyDividendYield, fd.DividendYieldPct))
	return sb.String()
}

// FinancialStatement renders a single 10-K/10-Q FinancialStatement.
func FinancialStatement(lang i18n.Lang, st *data.FinancialStatement) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyStatementTitle, st.Form, st.FiscalYear, st.PeriodEnd))

	sb.WriteString(i18n.T(lang, i18n.KeyIncomeStatementHeader))
	sb.WriteString(i18n.T(lang, i18n.KeyRevenue, Commaf(st.Revenue/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyGrossProfit, Commaf(st.GrossProfit/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyOperatingIncome, Commaf(st.OperatingIncome/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyNetIncome, Commaf(st.NetIncome/1e6)))
	sb.WriteString(i18n.T(lang, i18n.KeyDilutedEPS, st.DilutedEPS))

	// TW filings (FinMind, Phase 6 PR3) carry income-statement figures only —
	// no balance sheet or cash flow data at all — so each section is
	// skipped outright when every one of its fields is 0 rather than
	// rendering a misleading "$0M" trio. A real US filing's figures are
	// never exactly 0, so this is a no-op for every pre-existing caller.
	if st.TotalAssets != 0 || st.TotalLiabilities != 0 || st.TotalEquity != 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyBalanceSheetHeader))
		sb.WriteString(i18n.T(lang, i18n.KeyTotalAssets, Commaf(st.TotalAssets/1e6)))
		sb.WriteString(i18n.T(lang, i18n.KeyTotalLiabilities, Commaf(st.TotalLiabilities/1e6)))
		sb.WriteString(i18n.T(lang, i18n.KeyTotalEquity, Commaf(st.TotalEquity/1e6)))
	}

	if st.OperatingCashFlow != 0 || st.CapEx != 0 || st.FreeCashFlow != 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyCashFlowHeader))
		sb.WriteString(i18n.T(lang, i18n.KeyOperatingCashFlow, Commaf(st.OperatingCashFlow/1e6)))
		sb.WriteString(i18n.T(lang, i18n.KeyCapEx, Commaf(st.CapEx/1e6)))
		sb.WriteString(i18n.T(lang, i18n.KeyFreeCashFlow, Commaf(st.FreeCashFlow/1e6)))
	}
	return sb.String()
}

// Commaf formats a float as a rounded integer with thousands separators
// (e.g. 4321020 -> "4,321,020"), for human-facing Telegram output.
func Commaf(v float64) string {
	n := int64(v + 0.5)
	if v < 0 {
		n = int64(v - 0.5)
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
