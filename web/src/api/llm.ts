import { getJSON, postJSON } from "./http";

// --- Phase 19: LLM input transparency + news-source blacklist ---
// Mirrors internal/web/llmruns.go/newssources.go. Read routes (list/detail/
// blocked-list) only exist server-side when WEB_LLM_AUDIT is set (see
// Config.llmAuditEnabled) — App.tsx/Sidebar.tsx hide the /llm nav link
// entirely rather than showing it and hitting a 404.

export interface LLMRunSummary {
  id: number;
  kind: string;
  market: string;
  model: string;
  latencyMs: number;
  createdAt: string;
  watchlistCount: number;
  candidateCount: number;
  newsCount: number;
  candleGapCount: number;
}

export interface LLMRunDetail extends LLMRunSummary {
  // input is GenerateRecommendations' full, unprojected recommendationInputs
  // payload (bot.recordLLMRun) — shape mirrors internal/llm's StockData/
  // MarketContext/PastLesson/data.NewsItem Go structs 1:1 (no json tags on
  // most of them, so field names are the exported Go names verbatim).
  input: LLMRunInput;
  outputRaw: string;
}

export interface LLMNewsItem {
  Headline: string;
  Summary: string;
  Source: string;
  URL: string;
  PublishedAt: string;
}

export interface LLMCandle {
  Date: string;
  Open: number;
  High: number;
  Low: number;
  Close: number;
  Volume: number;
}

export interface LLMPastLesson {
  Ticker: string;
  Date: string;
  Lesson: string;
}

export interface LLMStrategyHit {
  Name: string;
  DaysAgo: number;
}

export interface LLMStockData {
  Quote?: Record<string, unknown>;
  News?: LLMNewsItem[];
  CompanyName?: string;
  Fundamentals?: Record<string, unknown>;
  AnalystRating?: Record<string, unknown>;
  InsiderTx?: Record<string, unknown>[];
  InstitutionalFlow?: Record<string, unknown>;
  Position?: Record<string, unknown>;
  ScanReason?: string;
  Technicals?: Record<string, unknown>;
  Candles?: LLMCandle[];
  PrevRec?: Record<string, unknown>;
  PastLessons?: LLMPastLesson[];
  StrategyHits?: LLMStrategyHit[];
  [key: string]: unknown;
}

// Every field is optional because two different Go payloads land in this
// one column: recommend/daily_report runs store bot.recordLLMRun's
// recommendationInputs (watchlist/candidates/marketNews), price_event runs
// store recordPriceEventLLMRun's single-ticker struct (ticker/gapPct/news).
// LlmRunsView's runInput() is the one place that reconciles the two.
export interface LLMRunInput {
  watchlist?: LLMStockData[];
  candidates?: LLMStockData[];
  marketNews?: LLMNewsItem[];
  marketContext?: Record<string, unknown>;
  recentLessons?: LLMPastLesson[];
  isTW?: boolean;
  // price_event only (internal/bot/pipeline.go recordPriceEventLLMRun)
  ticker?: string;
  gapPct?: number;
  changePct?: number;
  cumulativePct?: number;
  news?: LLMNewsItem[];
}

export function fetchLLMRuns(): Promise<{ runs: LLMRunSummary[] }> {
  return getJSON("/api/llm-runs");
}

export function fetchLLMRunDetail(id: number): Promise<LLMRunDetail> {
  return getJSON(`/api/llm-runs/${id}`);
}

export interface BlockedNewsSource {
  source: string;
  createdAt: string;
}

export function fetchBlockedNewsSources(): Promise<{ sources: BlockedNewsSource[] }> {
  return getJSON("/api/news-sources/blocked");
}

export function blockNewsSource(source: string): Promise<{ message: string }> {
  return postJSON("/api/news-sources/block", { source });
}

export function unblockNewsSource(source: string): Promise<{ message: string }> {
  return postJSON("/api/news-sources/unblock", { source });
}
