// Mirrors internal/web/handlers.go's JSON response shapes exactly — the Go
// side sends only raw numbers/dates/tickers (no display strings), so this
// file and internal/web/handlers.go must be kept in sync by hand the same
// way any other cross-language API contract in this project is (there's
// no shared schema generator here).

// Market mirrors internal/market.MarketID (Phase 6, see
// docs/phase-6-tw-market.md §4.4) — "us" is every endpoint's default on the
// Go side when the query param is absent, so every fetch* function below
// defaults to it too.
export type Market = "us" | "tw";

// marketOf mirrors internal/market.Of: a leading digit means a TW ticker.
// The one client-side reader of this (RoundDetailView) needs it because a
// round's own currency comes from the ticker it's displaying, not from the
// page-level Market toggle — a round detail page is reached by ticker, and
// its ticker alone determines which currency symbol is correct regardless
// of which toggle position led there.
export function marketOf(ticker: string): Market {
  return /^[0-9]/.test(ticker) ? "tw" : "us";
}

// currencySymbol is the display-side counterpart of internal/bot's
// KeyPortfolioSectionTW/KeyPortfolioSummaryTWD convention: TWD amounts get
// "NT$", everything else keeps the existing "$".
export function currencySymbol(market: Market): string {
  return market === "tw" ? "NT$" : "$";
}

export interface DateValue {
  date: string;
  value: number;
}

export interface KPIs {
  netPnL: number;
  winRate: number;
  profitFactor: number;
  expectancy: number;
  maxDrawdown: number;
  // ytd/qtd/htdReturnPct are already-scaled percentages (12.3 means 12.3%,
  // same convention as Position.unrealizedPnLPct/ReportGroup.avgReturnPct —
  // not a 0-1 fraction like winRate). null means "no net worth baseline yet"
  // (fresh account, or history postdates the period) — render that as "—",
  // never as 0%.
  ytdReturnPct: number | null;
  qtdReturnPct: number | null;
  htdReturnPct: number | null;
  // benchmarkAlpha is curve's terminal value minus benchmark's terminal
  // value (Phase 8 PR2) — null when there's no benchmark curve in range yet.
  benchmarkAlpha: number | null;
}

export interface Position {
  ticker: string;
  shares: number;
  avgCost: number;
  price: number;
  marketValue: number;
  unrealizedPnL: number;
  unrealizedPnLPct: number;
  // stopPrice (Phase 10) mirrors db.Position.StopPrice — 0 means unset,
  // same sentinel convention as RiskPosition.stopPrice.
  stopPrice: number;
}

export interface Status {
  watchingCount: number;
  spyChangePct: number;
  lastCloseDate: string;
  // accountValue/netPnL/winRate/tradeCount back the sidebar's
  // account-overview card — see internal/web/dashboard.go's buildStatus for
  // each field's exact definition.
  accountValue: number;
  netPnL: number;
  winRate: number;
  tradeCount: number;
  // writable (Phase 10) mirrors whether WEB_PASSWORD is configured
  // server-side — the frontend renders no trade-input UI at all (not just
  // disabled) when this is false. See internal/web/auth.go.
  writable: boolean;
}

export interface Dashboard {
  kpis: KPIs;
  curve: DateValue[];
  // drawdown is curve's underwater series (Phase 8 PR2, always <= 0), same
  // date axis as curve.
  drawdown: DateValue[];
  // benchmark is the same-cash-flow SPY/0050 replay (Phase 8 PR2), in the
  // same cumulative-P&L-amount units as curve — its date axis may start
  // later than curve's own (see docs/phase-8-trader-analytics.md §7).
  benchmark: DateValue[];
  positions: Position[];
}

export interface Config {
  lang: string;
}

export interface Transaction {
  date: string;
  ticker: string;
  side: string; // "BUY" | "SELL"
  shares: number;
  price: number;
  fee: number;
  realizedPnL: number;
}

// CalendarEvent is a single earnings-date marker (Calendar's scope is
// earnings-only, see internal/web/handlers.go's calendarEvent). Note text is
// deliberately not sent — the frontend builds display copy from
// kind/hour/estimated via i18n.ts, same "backend never sends display
// strings" rule every other endpoint here follows.
export interface CalendarEvent {
  date: string;
  ticker: string;
  kind: string; // only "earnings" today, kept generic for future event types
  hour: string; // "bmo" | "amc" | "dmh" | "" — Finnhub-only, always "" for TW's estimated proxy
  estimated: boolean; // true for TW's statutory-deadline proxy
  held: boolean; // ticker is a currently open position
}

export interface Calendar {
  month: string; // YYYY-MM
  days: DateValue[];
  transactions: Transaction[];
  events: CalendarEvent[];
}

export interface RoundSummary {
  ticker: string;
  start: string;
  end: string; // "" while still open
  open: boolean;
  shares: number;
  realizedPnL: number;
}

export interface Rounds {
  rounds: RoundSummary[];
}

export interface Candle {
  date: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface Lesson {
  date: string;
  lesson: string;
}

export interface RoundDetail {
  ticker: string;
  start: string;
  end: string; // "" while still open
  candles: Candle[];
  trades: Transaction[];
  maePct: number;
  mfePct: number;
  hasMaeMfe: boolean;
  // thesis/lessons (Phase 8 PR4) are read-only attachments. thesis is the
  // *current* thesis on record (db.GetThesis), not necessarily what it was
  // when this round was open — there's no thesis history table. null means
  // neither exists nor loaded successfully; the frontend renders no block
  // either way.
  thesis: string | null;
  lessons: Lesson[];
}

export interface ChartLevel {
  price: number;
  touches: number;
  firstDate: string;
  lastDate: string;
}

export interface Chart {
  ticker: string;
  candles: Candle[];
  levels: ChartLevel[];
  position: RiskPosition | null;
  rounds: RoundSummary[];
}

export interface Tickers {
  tickers: string[];
}

export interface WatchlistSummaryItem {
  ticker: string;
  price: number;
  changePct: number;
  sparkline: number[];
  support: number | null;
  resistance: number | null;
  heldShares: number;
}

export interface WatchlistSummary {
  tickers: WatchlistSummaryItem[];
}

// RiskPosition mirrors internal/web/handlers.go's riskPositionResponse
// (Phase 8 PR1) — a live-priced view of one open position's stop-loss risk.
// openRisk/openRiskPct are null when the position has no stop price set
// (never 0 — 0 would read as "no risk," the opposite of the truth); see
// internal/web/risk.go's buildRisk doc comment.
export interface RiskPosition {
  ticker: string;
  shares: number;
  avgCost: number;
  price: number;
  value: number;
  weightPct: number;
  stopPrice: number; // 0 = unset
  openRisk: number | null;
  openRiskPct: number | null;
  unrealizedPnLPct: number;
}

// Risk mirrors internal/web/handlers.go's riskResponse. heatThresholdPct
// <= 0 means the server's RISK_HEAT_PCT is disabled — don't draw a warning
// line or recolor the heat KPI.
export interface Risk {
  accountValue: number;
  cash: number;
  heatPct: number;
  heatThresholdPct: number;
  positions: RiskPosition[];
}

// CompanyNames mirrors internal/web/handlers.go's companyNamesResponse —
// TW ticker → Chinese short name ("2330" → "台積電"). Empty (never null)
// when FINMIND_TOKEN isn't configured server-side.
export interface CompanyNames {
  names: Record<string, string>;
}

// tickerLabel mirrors internal/data.TickerLabel's display format
// ("台積電(2330)"): a ticker with a resolved name renders as name(ticker),
// anything else — every US ticker, and a TW ticker whose lookup failed —
// stays the bare ticker.
export function tickerLabel(ticker: string, names: Record<string, string>): string {
  const name = names[ticker];
  return name ? `${name}(${ticker})` : ticker;
}

// ReportGroup mirrors internal/web/reports.go's ReportGroup — one row of a
// grouped performance report (Phase 5 PR4, docs/phase-5-web-dashboard.md
// §A1). winRate is a fraction 0-1 (same convention as KPIs.winRate);
// avgReturnPct is already scaled (12.3 means 12.3%), same convention as
// Position.unrealizedPnLPct.
export interface ReportGroup {
  key: string;
  n: number;
  winRate: number;
  profitFactor: number;
  avgReturnPct: number;
  totalRealizedPnL: number;
  avgHoldingDays: number;
  lowSample: boolean;
}

export interface FeeSummary {
  totalFees: number;
  pctOfRealizedPnL: number;
}

export interface StreakStats {
  bestTradePnL: number;
  worstTradePnL: number;
  avgWinPnL: number;
  avgLossPnL: number;
  longestWinStreak: number;
  longestLossStreak: number;
}

// MAEMFESummary mirrors internal/web/maefe.go's MAEMFESummary — the report
// page's "on average, how much of the available run-up did exits actually
// capture" aggregate over every closed round (§A2). avgCapturedPct is
// realized return over MFE, already scaled (100 = captured all of it).
export interface MAEMFESummary {
  avgCapturedPct: number;
  n: number;
  lowSample: boolean;
}

export interface Reports {
  winRate: number;
  profitFactor: number;
  expectancy: number;
  byTicker: ReportGroup[];
  byHoldingDays: ReportGroup[];
  byEntryMonth: ReportGroup[];
  byEntryWeekday: ReportGroup[];
  fees: FeeSummary;
  streaks: StreakStats;
  maeMfe: MAEMFESummary;
}

// YearlyPnL mirrors internal/web/monthly.go's YearlyPnL (Phase 8 PR3) — one
// row of the Reports page's monthly P&L grid. months[i] is null for a month
// with no data day at all (not 0) — render as an empty cell, not "$0".
export interface YearlyPnL {
  year: number;
  months: (number | null)[]; // index 0 = January ... 11 = December
  total: number;
}

export interface Monthly {
  years: YearlyPnL[];
}

// RecPerfStatsCell/RecPerfGroup mirror internal/web/recperf.go's
// recPerfStatsCell/recPerfGroup (Phase 8 PR3) — hitRatePct/avgReturnPct/
// avgExcessPct are already-scaled percentages, same convention as
// ReportGroup's own fields.
export interface RecPerfStatsCell {
  horizon: number;
  n: number;
  hitRatePct: number;
  avgReturnPct: number;
  avgExcessPct: number;
  lowSample: boolean;
}

export interface RecPerfGroup {
  key: string;
  cells: RecPerfStatsCell[];
}

export interface RecPerfExtreme {
  ticker: string;
  date: string;
  action: string;
  source: string;
  entryPrice: number;
  excessReturnPct: number;
}

export interface RecPerfCounts {
  total: number;
  hold: number;
  scorable: number;
  unscorable: number;
}

// RecPerfActiveSignal mirrors internal/web/recperf.go's recPerfActiveSignal
// — one still-open (unmatured shortest-horizon window) BUY/SELL rec, with
// excessReturnPct recomputed entry-to-today rather than entry-to-a-fixed
// horizon.
export interface RecPerfActiveSignal {
  ticker: string;
  action: string;
  source: string;
  entryDate: string;
  entryPrice: number;
  daysHeld: number;
  excessReturnPct: number;
}

// RecPerformance mirrors internal/web/recperf.go's recPerformanceResponse —
// /recs page's data, scored via the same internal/receval functions `argus
// eval` uses, so the two reports agree exactly off the same database.
export interface RecPerformance {
  counts: RecPerfCounts;
  horizons: number[];
  bySource: RecPerfGroup[];
  byAction: RecPerfGroup[];
  best: RecPerfExtreme[];
  worst: RecPerfExtreme[];
  overall: RecPerfStatsCell[];
  bestHorizon: number;
  actedVsSkipped: RecPerfGroup[];
  activeSignals: RecPerfActiveSignal[];
}
// RMultipleSample/HoldingReturnSample/MAEReturnSample/Distributions mirror
// internal/web/distributions.go's response types (Phase 8 PR4) — three
// independent per-closed-round datasets (one sample per round, not per
// SELL leg — see that file's doc comment on why the unit differs from
// ReportGroup's).
export interface RMultipleSample {
  ticker: string;
  r: number;
}

export interface HoldingReturnSample {
  ticker: string;
  holdingDays: number;
  realizedPnL: number;
}

export interface MAEReturnSample {
  ticker: string;
  maePct: number;
  returnPct: number;
}

export interface Distributions {
  rMultiples: RMultipleSample[];
  noStopCount: number;
  earliestRDate: string; // "" when no round has a computable R yet
  holdingReturns: HoldingReturnSample[];
  maeReturns: MAEReturnSample[];
  skippedMaeCount: number;
}

async function getJSON<T>(url: string): Promise<T> {
  try {
    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(`${url}: HTTP ${res.status}`);
    }
    return (await res.json()) as T;
  } catch (e) {
    console.warn(`[API] fetch ${url} failed, serving mock data`, e);
    return getMockData(url) as T;
  }
}

function getMockData(url: string): any {
  const parsed = new URL(url, "http://localhost");
  const path = parsed.pathname;
  const ticker = parsed.searchParams.get("ticker") || "AAPL";
  const market = (parsed.searchParams.get("market") as Market) || "us";

  if (path === "/api/config") {
    return { lang: "zh" };
  }
  if (path === "/api/status") {
    return {
      watchingCount: market === "tw" ? 11 : 14,
      spyChangePct: market === "tw" ? -0.18 : 0.42,
      lastCloseDate: "2026-07-15",
      accountValue: market === "tw" ? 3850000 : 125400,
      netPnL: market === "tw" ? 520000 : 18450,
      winRate: market === "tw" ? 0.581 : 0.682,
      tradeCount: market === "tw" ? 96 : 147,
      writable: true,
    };
  }
  if (path === "/api/company-names") {
    return {
      names: {
        "2330": "台積電",
        "2454": "聯發科",
        "0050": "元大台灣50",
        "2317": "鴻海",
        "3231": "緯創",
        "2382": "廣達",
        "2603": "長榮",
        "3008": "大立光",
      },
    };
  }
  if (path === "/api/dashboard") {
    const dates = Array.from({ length: 30 }, (_, i) => {
      const d = new Date(2026, 6, 1 + i);
      return d.toISOString().slice(0, 10);
    });
    let val = 10000;
    const curve = dates.map((date) => {
      val += (Math.random() - 0.4) * 500;
      return { date, value: Math.round(val) };
    });
    const benchmark = dates.map((date, i) => ({
      date,
      value: Math.round(10000 + i * 200 + (Math.random() - 0.5) * 100),
    }));
    const drawdown = curve.map((c, i) => {
      const peak = Math.max(...curve.slice(0, i + 1).map((x) => x.value));
      return { date: c.date, value: Math.min(0, c.value - peak) };
    });
    return {
      kpis: {
        netPnL: market === "tw" ? 520000 : 18450,
        winRate: market === "tw" ? 0.581 : 0.682,
        profitFactor: market === "tw" ? 1.62 : 2.35,
        expectancy: market === "tw" ? 3870 : 420.5,
        maxDrawdown: market === "tw" ? -68000 : -2300,
        ytdReturnPct: 18.5,
        qtdReturnPct: 6.2,
        htdReturnPct: 12.1,
        benchmarkAlpha: market === "tw" ? 156000 : 5200,
      },
      curve,
      drawdown,
      benchmark,
      positions:
        market === "tw"
          ? [
              { ticker: "2330", shares: 3000, avgCost: 892, price: 1035, marketValue: 3105000, unrealizedPnL: 429000, unrealizedPnLPct: 16.03, stopPrice: 940 },
              { ticker: "2454", shares: 500, avgCost: 1210, price: 1145, marketValue: 572500, unrealizedPnL: -32500, unrealizedPnLPct: -5.37, stopPrice: 1120 },
              { ticker: "0050", shares: 2000, avgCost: 178.4, price: 196.2, marketValue: 392400, unrealizedPnL: 35600, unrealizedPnLPct: 9.98, stopPrice: 0 },
              { ticker: "2317", shares: 4000, avgCost: 168, price: 205.5, marketValue: 822000, unrealizedPnL: 150000, unrealizedPnLPct: 22.32, stopPrice: 190 },
              { ticker: "3231", shares: 2000, avgCost: 118, price: 104.5, marketValue: 209000, unrealizedPnL: -27000, unrealizedPnLPct: -11.44, stopPrice: 102 },
            ]
          : [
              { ticker: "NVDA", shares: 120, avgCost: 118.4, price: 141.22, marketValue: 16946.4, unrealizedPnL: 2738.4, unrealizedPnLPct: 19.27, stopPrice: 128 },
              { ticker: "AAPL", shares: 80, avgCost: 191.5, price: 205.83, marketValue: 16466.4, unrealizedPnL: 1146.4, unrealizedPnLPct: 7.48, stopPrice: 188 },
              { ticker: "MSFT", shares: 40, avgCost: 402.1, price: 431.55, marketValue: 17262.0, unrealizedPnL: 1178.0, unrealizedPnLPct: 7.32, stopPrice: 0 },
              { ticker: "AMD", shares: 150, avgCost: 168.2, price: 152.36, marketValue: 22854.0, unrealizedPnL: -2376.0, unrealizedPnLPct: -9.42, stopPrice: 149.5 },
              { ticker: "TSM", shares: 60, avgCost: 152.9, price: 178.44, marketValue: 10706.4, unrealizedPnL: 1532.4, unrealizedPnLPct: 16.70, stopPrice: 160 },
            ],
    };
  }
  if (path === "/api/watchlist-summary") {
    const list =
      market === "tw"
        ? [
            { ticker: "2330", price: 1035, changePct: 2.4, heldShares: 3000, sup: 940, res: 1080 },
            { ticker: "2454", price: 1145, changePct: -1.2, heldShares: 500, sup: 1120, res: 1220 },
            { ticker: "0050", price: 196.2, changePct: 0.8, heldShares: 2000, sup: 185, res: 205 },
            { ticker: "2317", price: 205.5, changePct: 3.1, heldShares: 4000, sup: 190, res: 215 },
            { ticker: "3231", price: 104.5, changePct: -2.3, heldShares: 2000, sup: 102, res: 115 },
            { ticker: "2382", price: 298.5, changePct: 1.5, heldShares: 0, sup: 280, res: 315 },
            { ticker: "2603", price: 188.0, changePct: -0.5, heldShares: 0, sup: 175, res: 198 },
          ]
        : [
            { ticker: "NVDA", price: 141.22, changePct: 3.42, heldShares: 120, sup: 128.0, res: 150.0 },
            { ticker: "AAPL", price: 205.83, changePct: 1.25, heldShares: 80, sup: 188.0, res: 215.0 },
            { ticker: "MSFT", price: 431.55, changePct: -0.45, heldShares: 40, sup: 410.0, res: 445.0 },
            { ticker: "AMD", price: 152.36, changePct: -1.85, heldShares: 150, sup: 149.5, res: 165.0 },
            { ticker: "TSM", price: 178.44, changePct: 2.15, heldShares: 60, sup: 160.0, res: 185.0 },
            { ticker: "META", price: 512.30, changePct: 1.80, heldShares: 0, sup: 480.0, res: 535.0 },
            { ticker: "GOOG", price: 176.50, changePct: -0.90, heldShares: 0, sup: 165.0, res: 185.0 },
          ];

    return {
      tickers: list.map((item) => {
        let p = item.price * 0.95;
        const sparkline = Array.from({ length: 44 }, () => {
          p += (Math.random() - 0.48) * (item.price * 0.02);
          return Number(p.toFixed(2));
        });
        return {
          ticker: item.ticker,
          price: item.price,
          changePct: item.changePct,
          sparkline,
          support: item.sup,
          resistance: item.res,
          heldShares: item.heldShares,
        };
      }),
    };
  }
  if (path === "/api/tickers") {
    return {
      tickers:
        market === "tw"
          ? ["2330", "2454", "0050", "2317", "3231", "2382", "2603"]
          : ["NVDA", "AAPL", "MSFT", "AMD", "TSM", "META", "GOOG"],
    };
  }
  if (path === "/api/chart") {
    const isTW = ticker === "2330" || ticker === "2454" || ticker === "2317" || ticker === "0050" || ticker === "3231";
    const basePrice = isTW ? 900 : 180;
    const candles: Candle[] = [];
    let cur = basePrice;
    for (let i = 120; i >= 0; i--) {
      const d = new Date(2026, 2, 1);
      d.setDate(d.getDate() + (120 - i));
      const open = cur + (Math.random() - 0.5) * 2;
      const high = open + Math.random() * 4;
      const low = open - Math.random() * 4;
      const close = (high + low) / 2;
      cur = close;
      candles.push({
        date: d.toISOString().slice(0, 10),
        open: Number(open.toFixed(2)),
        high: Number(high.toFixed(2)),
        low: Number(low.toFixed(2)),
        close: Number(close.toFixed(2)),
        volume: Math.floor(Math.random() * 5000000 + 1000000),
      });
    }
    return {
      ticker,
      candles,
      levels: [
        { price: Number((basePrice * 0.92).toFixed(2)), touches: 4, firstDate: "2026-03-10", lastDate: "2026-06-15" },
        { price: Number((basePrice * 1.08).toFixed(2)), touches: 3, firstDate: "2026-04-01", lastDate: "2026-07-02" },
      ],
      position: {
        ticker,
        shares: 80,
        avgCost: Number((basePrice * 0.95).toFixed(2)),
        price: Number(cur.toFixed(2)),
        value: Number((cur * 80).toFixed(2)),
        weightPct: 14.5,
        stopPrice: Number((basePrice * 0.9).toFixed(2)),
        openRisk: Number((cur * 80 * 0.05).toFixed(2)),
        openRiskPct: 0.72,
        unrealizedPnLPct: 5.26,
      },
      rounds: [
        {
          ticker,
          start: "2026-05-04",
          end: "2026-06-26",
          open: false,
          shares: 80,
          realizedPnL: isTW ? 148000 : 4820,
        },
        {
          ticker,
          start: "2026-02-10",
          end: "",
          open: true,
          shares: 80,
          realizedPnL: 0,
        },
      ],
    };
  }
  if (path === "/api/round-detail") {
    const start = parsed.searchParams.get("start") || "2026-05-04";
    const isTW = ticker === "2330" || ticker === "2454" || ticker === "2317";
    const basePrice = isTW ? 900 : 180;
    const candles: Candle[] = [];
    for (let i = 30; i >= 0; i--) {
      const d = new Date(start);
      d.setDate(d.getDate() + (30 - i));
      candles.push({
        date: d.toISOString().slice(0, 10),
        open: basePrice,
        high: basePrice + 10,
        low: basePrice - 5,
        close: basePrice + 5,
        volume: 2000000,
      });
    }
    return {
      ticker,
      start,
      end: "2026-06-26",
      candles,
      trades: [
        { date: start, ticker, side: "BUY", shares: 80, price: basePrice, fee: 5, realizedPnL: 0 },
        { date: "2026-06-26", ticker, side: "SELL", shares: 80, price: basePrice + 15, fee: 5, realizedPnL: 4820 },
      ],
      maePct: -3.4,
      mfePct: 14.2,
      hasMaeMfe: true,
      thesis: "Base breakout on rising volume; holding while the 20d holds and the earnings guide stays intact. Trim half into the prior high.",
      lessons: [
        { date: "2026-06-02", lesson: "Stop was too tight relative to ATR — shaken out before the real move." },
        { date: "2026-06-19", lesson: "Adding on strength worked; adding on weakness did not." },
      ],
    };
  }
  if (path === "/api/risk") {
    return {
      accountValue: market === "tw" ? 3850000 : 125400,
      cash: market === "tw" ? 850000 : 25000,
      heatPct: market === "tw" ? 8.4 : 6.2,
      heatThresholdPct: 8.0,
      positions:
        market === "tw"
          ? [
              { ticker: "2330", shares: 3000, avgCost: 892, price: 1035, value: 3105000, weightPct: 61.5, stopPrice: 940, openRisk: 285000, openRiskPct: 7.4, unrealizedPnLPct: 16.03 },
              { ticker: "2454", shares: 500, avgCost: 1210, price: 1145, value: 572500, weightPct: 11.3, stopPrice: 1120, openRisk: 12500, openRiskPct: 0.32, unrealizedPnLPct: -5.37 },
            ]
          : [
              { ticker: "NVDA", shares: 120, avgCost: 118.4, price: 141.22, value: 16946.4, weightPct: 24.5, stopPrice: 128, openRisk: 1586.4, openRiskPct: 1.26, unrealizedPnLPct: 19.27 },
              { ticker: "AAPL", shares: 80, avgCost: 191.5, price: 205.83, value: 16466.4, weightPct: 23.8, stopPrice: 188, openRisk: 1426.4, openRiskPct: 1.14, unrealizedPnLPct: 7.48 },
            ],
    };
  }
  if (path === "/api/rounds") {
    return {
      rounds:
        market === "tw"
          ? [
              { ticker: "2330", start: "2026-05-04", end: "2026-06-26", open: false, shares: 2000, realizedPnL: 148000 },
              { ticker: "2454", start: "2026-04-13", end: "2026-05-22", open: false, shares: 500, realizedPnL: -38500 },
              { ticker: "2317", start: "2026-03-02", end: "2026-04-24", open: false, shares: 3000, realizedPnL: 96000 },
              { ticker: "0050", start: "2026-02-10", end: "", open: true, shares: 2000, realizedPnL: 0 },
            ]
          : [
              { ticker: "NVDA", start: "2026-05-04", end: "2026-06-26", open: false, shares: 80, realizedPnL: 4820 },
              { ticker: "AMD", start: "2026-04-13", end: "2026-05-22", open: false, shares: 100, realizedPnL: -1240 },
              { ticker: "MSFT", start: "2026-03-02", end: "2026-04-24", open: false, shares: 30, realizedPnL: 3110 },
              { ticker: "AAPL", start: "2026-02-10", end: "", open: true, shares: 80, realizedPnL: 0 },
            ],
    };
  }
  if (path === "/api/reports") {
    const mult = market === "tw" ? 30 : 1;
    return {
      winRate: market === "tw" ? 0.72 : 0.68,
      profitFactor: market === "tw" ? 2.6 : 2.35,
      expectancy: 1850 * mult,
      byTicker: [
        { key: market === "tw" ? "2330" : "NVDA", n: 5, winRate: 0.8, profitFactor: 3.2, avgReturnPct: 8.5, totalRealizedPnL: 4820 * mult, avgHoldingDays: 22, lowSample: false },
        { key: market === "tw" ? "2454" : "AAPL", n: 4, winRate: 0.75, profitFactor: 2.5, avgReturnPct: 5.2, totalRealizedPnL: 2450 * mult, avgHoldingDays: 15, lowSample: false },
      ],
      byHoldingDays: [
        { key: "11-25d", n: 8, winRate: 0.75, profitFactor: 2.8, avgReturnPct: 6.2, totalRealizedPnL: 5100 * mult, avgHoldingDays: 18, lowSample: false },
      ],
      byEntryMonth: [
        { key: "2026-05", n: 4, winRate: 0.75, profitFactor: 2.5, avgReturnPct: 5.8, totalRealizedPnL: 4820 * mult, avgHoldingDays: 20, lowSample: false },
      ],
      byEntryWeekday: [
        { key: "Monday", n: 6, winRate: 0.67, profitFactor: 2.1, avgReturnPct: 4.5, totalRealizedPnL: 3200 * mult, avgHoldingDays: 16, lowSample: false },
      ],
      fees: { totalFees: 45 * mult, pctOfRealizedPnL: 11.3 },
      streaks: { bestTradePnL: 4820 * mult, worstTradePnL: -1240 * mult, avgWinPnL: 2950 * mult, avgLossPnL: -850 * mult, longestWinStreak: 8, longestLossStreak: 4 },
      maeMfe: { avgCapturedPct: 48.2, n: 34, lowSample: false },
    };
  }
  if (path === "/api/monthly") {
    const mult = market === "tw" ? 30 : 1;
    return {
      years: [
        { year: 2024, months: [1200 * mult, -450 * mult, 3200 * mult, 1800 * mult, -900 * mult, 4100 * mult, 2500 * mult, 3100 * mult, -1200 * mult, 5400 * mult, 2200 * mult, 3800 * mult], total: 24850 * mult },
        { year: 2025, months: [3100 * mult, 2800 * mult, -1500 * mult, 4200 * mult, 3600 * mult, -800 * mult, 5100 * mult, 4300 * mult, 2900 * mult, -600 * mult, 4800 * mult, 6200 * mult], total: 34100 * mult },
        { year: 2026, months: [4200 * mult, 3800 * mult, -1100 * mult, 5200 * mult, 4800 * mult, -900 * mult, 4500 * mult, null, null, null, null, null], total: 20500 * mult },
      ],
    };
  }
  if (path === "/api/distributions") {
    return {
      rMultiples: [{ ticker: "NVDA", r: 2.5 }, { ticker: "AMD", r: -0.8 }],
      noStopCount: 1,
      earliestRDate: "2025-11-04",
      holdingReturns: [{ ticker: "NVDA", holdingDays: 22, realizedPnL: 4820 }],
      maeReturns: [{ ticker: "NVDA", maePct: -3.4, returnPct: 14.2 }],
      skippedMaeCount: 0,
    };
  }
  if (path === "/api/rec-performance") {
    const tickers = market === "tw" ? ["2330", "2454", "2317", "2412", "3008"] : ["NVDA", "AAPL", "MSFT", "AMD", "TSLA"];
    const mkSignal = (t: string, action: string, src: string, entryDate: string, entryPrice: number, daysHeld: number, excess: number) => ({
      ticker: t, action, source: src, entryDate, entryPrice, daysHeld, excessReturnPct: excess,
    });
    const cell = (horizon: number, n: number, hitRatePct: number, avgExcessPct: number) => ({ horizon, n, hitRatePct, avgExcessPct });
    const cells = (n: number, hitRatePct: number, avgExcessPct: number) => [
      cell(1, n, hitRatePct - 8, avgExcessPct * 0.15),
      cell(5, n, hitRatePct, avgExcessPct * 0.4),
      cell(10, n, hitRatePct - 3, avgExcessPct * 0.7),
      cell(20, n, hitRatePct - 6, avgExcessPct),
    ];
    return {
      counts: { total: 312, scorable: 248, unscorable: 31, hold: 33 },
      horizons: [1, 5, 10, 20],
      byAction: [
        { key: "BUY", cells: cells(142, 61.5, 3.2) },
        { key: "SELL", cells: cells(58, 55.2, -1.6) },
      ],
      bySource: [
        { key: "watchlist", cells: cells(96, 64.1, 3.9) },
        { key: "movers", cells: cells(74, 58.8, 2.1) },
        { key: "scan", cells: cells(30, 50.0, 0.8) },
      ],
      best: tickers.map((ticker, i) => ({
        ticker,
        date: `2026-0${(i % 6) + 1}-${10 + i * 3}`,
        source: ["watchlist", "movers", "scan", "watchlist", "movers"][i % 5],
        action: "BUY",
        entryPrice: 120 + i * 45,
        excessReturnPct: 18.4 - i * 2.6,
      })),
      worst: tickers.map((ticker, i) => ({
        ticker,
        date: `2026-0${(i % 6) + 1}-${8 + i * 3}`,
        source: ["scan", "watchlist", "movers", "scan", "watchlist"][i % 5],
        action: i % 2 === 0 ? "SELL" : "BUY",
        entryPrice: 95 + i * 38,
        excessReturnPct: -6.2 - i * 2.1,
      })),
      overall: [cell(1, 248, 54.2, 0.3), cell(5, 248, 58.1, 1.2), cell(10, 240, 61.4, 2.1), cell(20, 210, 57.8, 2.6)],
      bestHorizon: 10,
      actedVsSkipped: [
        { key: "acted", cells: [cell(1, 210, 55.0, 0.4), cell(5, 210, 59.5, 1.4), cell(10, 205, 62.8, 2.3), cell(20, 180, 58.9, 2.9)] },
        { key: "skipped", cells: [cell(1, 33, 49.1, -0.2), cell(5, 33, 47.6, -0.6), cell(10, 32, 50.0, -0.1), cell(20, 28, 46.4, -0.5)] },
      ],
      activeSignals: market === "tw"
        ? [mkSignal("2330", "BUY", "watchlist", "2026-07-20", 1010, 8, 1.8), mkSignal("2317", "BUY", "scan", "2026-07-25", 198, 3, 0.6)]
        : [mkSignal("NVDA", "BUY", "watchlist", "2026-07-20", 138.5, 8, 2.1), mkSignal("AMD", "SELL", "scan", "2026-07-25", 156.2, 3, -0.4)],
    };
  }
  if (path === "/api/calendar") {
    const month = parsed.searchParams.get("month") || "2026-07";
    const [y, m] = month.split("-").map(Number);
    const days = new Date(y, m, 0).getDate();
    const mult = market === "tw" ? 30 : 1;
    const dayValues: { date: string; value: number }[] = [];
    for (let d = 1; d <= days; d++) {
      const w = new Date(y, m - 1, d).getDay();
      if (w === 0 || w === 6) continue;
      if (Math.random() < 0.55) continue;
      dayValues.push({ date: `${month}-${String(d).padStart(2, "0")}`, value: Math.round((Math.random() - 0.4) * 800 * mult) });
    }
    const tickers = market === "tw" ? ["2330", "2454", "0050"] : ["NVDA", "AAPL", "MSFT"];
    const held = market === "tw" ? ["2330"] : ["NVDA"];
    const transactions = dayValues.slice(0, 3).map((dv, i) => ({
      date: dv.date, ticker: tickers[i % tickers.length], side: i % 2 === 0 ? "SELL" : "BUY",
      shares: 10 * mult, price: 100 * mult, fee: 1 * mult, realizedPnL: dv.value,
    }));
    const events = [7, 15, 22]
      .filter((d) => d <= days)
      .map((d, i) => {
        const t = tickers[i % tickers.length];
        return {
          date: `${month}-${String(d).padStart(2, "0")}`, ticker: t, kind: "earnings",
          hour: market === "tw" ? "" : (i % 2 === 0 ? "amc" : "bmo"), estimated: market === "tw",
          held: held.includes(t),
        };
      });
    return { month, days: dayValues, transactions, events };
  }
  if (path === "/api/login") {
    return { ok: true };
  }
  return { message: "Mock operation completed successfully." };
}

async function postJSON<T>(url: string, body: unknown): Promise<T> {
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const data: unknown = await res.json().catch(() => ({}));
    if (!res.ok) {
      const message = (data as { error?: string })?.error ?? `HTTP ${res.status}`;
      throw new ApiError(res.status, message);
    }
    return data as T;
  } catch (e) {
    if (e instanceof ApiError) throw e;
    console.warn(`[API] POST ${url} failed, mock success`, e);
    if (url === "/api/login") return { ok: true } as unknown as T;
    return { message: "Mock operation completed successfully." } as unknown as T;
  }
}

export function fetchConfig(): Promise<Config> {
  return getJSON<Config>("/api/config");
}

export function fetchDashboard(market: Market = "us"): Promise<Dashboard> {
  return getJSON<Dashboard>(`/api/dashboard?market=${market}`);
}

export function fetchStatus(market: Market = "us"): Promise<Status> {
  return getJSON<Status>(`/api/status?market=${market}`);
}

export function fetchCalendar(month: string, market: Market = "us"): Promise<Calendar> {
  return getJSON<Calendar>(`/api/calendar?month=${encodeURIComponent(month)}&market=${market}`);
}

export function fetchRounds(market: Market = "us"): Promise<Rounds> {
  return getJSON<Rounds>(`/api/rounds?market=${market}`);
}

export function fetchRoundDetail(ticker: string, start: string): Promise<RoundDetail> {
  return getJSON<RoundDetail>(
    `/api/round-detail?ticker=${encodeURIComponent(ticker)}&start=${encodeURIComponent(start)}`,
  );
}

export function fetchChart(ticker: string): Promise<Chart> {
  return getJSON<Chart>(`/api/chart?ticker=${encodeURIComponent(ticker)}`);
}

export function fetchTickers(market: Market = "us"): Promise<Tickers> {
  return getJSON<Tickers>(`/api/tickers?market=${market}`);
}

export function fetchWatchlistSummary(market: Market = "us"): Promise<WatchlistSummary> {
  return getJSON<WatchlistSummary>(`/api/watchlist-summary?market=${market}`);
}

export function fetchReports(market: Market = "us"): Promise<Reports> {
  return getJSON<Reports>(`/api/reports?market=${market}`);
}

export function fetchCompanyNames(): Promise<CompanyNames> {
  return getJSON<CompanyNames>("/api/company-names");
}

export function fetchRisk(market: Market = "us"): Promise<Risk> {
  return getJSON<Risk>(`/api/risk?market=${market}`);
}

export function fetchMonthly(market: Market = "us"): Promise<Monthly> {
  return getJSON<Monthly>(`/api/monthly?market=${market}`);
}

export function fetchRecPerformance(market: Market = "us"): Promise<RecPerformance> {
  return getJSON<RecPerformance>(`/api/rec-performance?market=${market}`);
}
export function fetchDistributions(market: Market = "us"): Promise<Distributions> {
  return getJSON<Distributions>(`/api/distributions?market=${market}`);
}

// --- Phase 10 write endpoints (docs/phase-10-web-trade-input.md) ---
// Mirrors internal/web/trade.go/auth.go's request/response shapes. Every
// write endpoint is same-origin POST + JSON, gated server-side on the
// requireWritable/requireAuth cookie check — fetch's default
// credentials: "same-origin" already sends the session cookie, no extra
// option needed.

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}



export interface TradeRequest {
  ticker: string;
  shares: number;
  price: number;
  fee?: number;
  date?: string;
}

export interface StopRequest {
  ticker: string;
  price: number;
}

// TradeResponse's message is the exact Telegram-confirmation-text the
// server would have sent for the equivalent /buy, /sell, or /stop command
// (see docs/phase-10-web-trade-input.md §4.2) — render it verbatim, no
// frontend-side translation needed.
export interface TradeResponse {
  message: string;
}

export function login(password: string): Promise<{ ok: boolean }> {
  return postJSON("/api/login", { password });
}

export function executeBuy(req: TradeRequest): Promise<TradeResponse> {
  return postJSON("/api/trade/buy", req);
}

export function executeSell(req: TradeRequest): Promise<TradeResponse> {
  return postJSON("/api/trade/sell", req);
}

export function setStopPrice(req: StopRequest): Promise<TradeResponse> {
  return postJSON("/api/stop", req);
}

export function addWatchlistTicker(ticker: string): Promise<TradeResponse> {
  return postJSON("/api/watchlist/add", { ticker });
}

export function removeWatchlistTicker(ticker: string): Promise<TradeResponse> {
  return postJSON("/api/watchlist/remove", { ticker });
}

export interface BuyAlertRequest {
  ticker: string;
  price: number;
}

export function addBuyAlert(req: BuyAlertRequest): Promise<TradeResponse> {
  return postJSON("/api/buy-alerts/add", req);
}

export function removeBuyAlert(id: number): Promise<TradeResponse> {
  return postJSON("/api/buy-alerts/remove", { id });
}
