// Mirrors the JSON response structs in internal/web (dashboard.go, rounds.go, chart.go, risk.go, ...) exactly — the Go
// side sends only raw numbers/dates/tickers (no display strings), so this
// files and those structs must be kept in sync by hand the same
// way any other cross-language API contract in this project is (there's
// no shared schema generator here).
//
// One file per page or area, named after the internal/web file it mirrors
// (wealth* for the wealth platform); this barrel re-exports them all, so
// importers keep using `from "../api"`.
export * from "./assistant";
export * from "./chart";
export * from "./common";
export * from "./dashboard";
export * from "./import";
export * from "./llm";
export * from "./options";
export * from "./paper";
export * from "./reports";
export * from "./risk";
export * from "./rounds";
export * from "./sectorflow";
export * from "./settings";
export * from "./trade";
export * from "./watchlist";
export * from "./wealthAlloc";
export * from "./wealthAssets";
export * from "./wealthBalance";
export * from "./wealthCash";
export * from "./wealthFunds";
export * from "./wealthGoals";
export * from "./wealthInsure";
export * from "./wealthProfile";
export * from "./wealthRetire";
export { ApiError } from "./http";
export { onNetWorthChange } from "./netWorth";
