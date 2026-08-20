import { useEffect, useState } from "react";
import {
  ApiError,
  blockNewsSource,
  fetchBlockedNewsSources,
  fetchLLMRunDetail,
  fetchLLMRuns,
  tickerLabel,
  unblockNewsSource,
  type BlockedNewsSource,
  type LLMNewsItem,
  type LLMRunDetail,
  type LLMRunSummary,
  type LLMStockData,
} from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  names?: Record<string, string>;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  runId: number | null;
  onOpenRun: (id: number) => void;
}

// SQLite's CURRENT_TIMESTAMP has no "T"/timezone suffix and is always UTC.
function parseSqliteUTC(s: string): number {
  return new Date(s.replace(" ", "T") + "Z").getTime();
}

function normTitle(s: string): string {
  return s.trim().toLowerCase();
}

function isStale(n: LLMNewsItem, createdAt: string): boolean {
  return parseSqliteUTC(createdAt) - new Date(n.PublishedAt).getTime() > 72 * 3600 * 1000;
}

function duplicateTitleSet(news: LLMNewsItem[]): Set<string> {
  const counts = new Map<string, number>();
  for (const n of news) counts.set(normTitle(n.Headline), (counts.get(normTitle(n.Headline)) ?? 0) + 1);
  const dupes = new Set<string>();
  for (const [title, c] of counts) if (c > 1) dupes.add(title);
  return dupes;
}

function allNews(detail: LLMRunDetail): LLMNewsItem[] {
  const stocks = [...detail.input.watchlist, ...detail.input.candidates];
  return [...detail.input.marketNews, ...stocks.flatMap((s) => s.News ?? [])];
}

function countLowQuality(news: LLMNewsItem[], blocked: Set<string>): number {
  return news.filter((n) => blocked.has(normTitle(n.Source))).length;
}

function countStale(news: LLMNewsItem[], createdAt: string): number {
  return news.filter((n) => isStale(n, createdAt)).length;
}

function countDuplicateTitles(news: LLMNewsItem[]): number {
  const dupes = duplicateTitleSet(news);
  return news.filter((n) => dupes.has(normTitle(n.Headline))).length;
}

// Renders MarketContext's flat key/value struct (Bench/SPYPrice/VIX/...) as
// a mockup-style " · "-joined line instead of a raw JSON dump — still no
// server-side reshaping (design doc's "pass through as-is" rule), just a
// friendlier join of the same fields, none dropped.
function formatMarketContext(value: Record<string, unknown>): string {
  return Object.entries(value)
    .map(([k, v]) => `${k} ${formatKV(v)}`)
    .join(" · ");
}

function formatKV(value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value)) return value.length === 0 ? "—" : JSON.stringify(value);
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

// Flattens the object-shaped StockData fields (Quote/Fundamentals/
// AnalystRating/...) into one label/value list, mockup-style — avoids
// hand-typing every nested Go struct's field list, and stays correct as
// those structs grow new fields (design doc's "never trim a field" rule).
function stockFields(stock: LLMStockData): { label: string; value: string }[] {
  const objs: Record<string, unknown>[] = [
    stock.Quote,
    stock.Fundamentals,
    stock.AnalystRating,
    stock.InstitutionalFlow,
    stock.Technicals,
    stock.Position,
    stock.PrevRec,
  ].filter((o): o is Record<string, unknown> => !!o);
  const fields: { label: string; value: string }[] = [];
  for (const obj of objs) {
    for (const [k, v] of Object.entries(obj)) {
      if (v === null || v === undefined || v === "") continue;
      fields.push({ label: k, value: formatKV(v) });
    }
  }
  return fields;
}

function StatBlock({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="dash-kpi-strip-item">
      <div className="dash-kpi-strip-label">{label}</div>
      <div className="dash-kpi-strip-value">{value}</div>
    </div>
  );
}

// Design mockup's per-metric warn thresholds (its llmModel()'s `flags`
// array) — a source name found in the blacklist or a candle gap is bad at
// any count, stale/duplicate news only stands out past a couple of items.
function QualityBlock({ label, value, warn }: { label: string; value: number; warn: boolean }) {
  return (
    <div className={`llm-quality-item${warn ? " warn" : ""}`}>
      <span className="llm-quality-value">{value}</span>
      <span className="llm-quality-label">{label}</span>
    </div>
  );
}

function NewsRow({
  dict,
  news,
  scope,
  createdAt,
  isDuplicate,
  blocked,
  writable,
  onBlock,
}: {
  dict: Dictionary;
  news: LLMNewsItem;
  scope?: string;
  createdAt: string;
  isDuplicate: boolean;
  blocked: Set<string>;
  writable: boolean;
  onBlock: (source: string) => void;
}) {
  const isBlocked = blocked.has(normTitle(news.Source));
  const stale = isStale(news, createdAt);
  return (
    <tr className={isBlocked ? "llm-row-dim" : ""}>
      <td>{news.Source}</td>
      {scope !== undefined && <td className="mono">{scope}</td>}
      <td className="mono">{news.PublishedAt}</td>
      <td style={{ textAlign: "left" }}>
        {news.URL ? (
          <a href={news.URL} target="_blank" rel="noreferrer">
            {news.Headline}
          </a>
        ) : (
          news.Headline
        )}
        {stale && <span className="tag">{dict.llmStaleNews}</span>}
        {isDuplicate && <span className="tag">{dict.llmDuplicateTitles}</span>}
        {isBlocked && <span className="tag warn">{dict.llmBlocked}</span>}
      </td>
      <td>
        {writable && !isBlocked && (
          <button className="btn-sm" onClick={() => onBlock(news.Source)}>
            {dict.llmBlockSource}
          </button>
        )}
      </td>
    </tr>
  );
}

function StockCard({ dict, names, stock }: { dict: Dictionary; names: Record<string, string>; stock: LLMStockData }) {
  const ticker = (stock.Quote?.Ticker as string) || "?";
  const fields = stockFields(stock);
  const newsCount = stock.News?.length ?? 0;
  return (
    <div className="card">
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span className="mono" style={{ fontWeight: 600 }}>
          {tickerLabel(ticker, names)}
        </span>
        {stock.Position && <span className="tag warn">{dict.eventHeld}</span>}
        <span className="mono" style={{ marginLeft: "auto", fontSize: 10.5, color: "var(--ink-3)" }}>
          {newsCount} {dict.llmNews}
        </span>
      </div>
      {stock.ScanReason && (
        <div className="stat-note">
          {dict.llmScanReason}: {stock.ScanReason}
        </div>
      )}
      <div style={{ display: "flex", flexDirection: "column", gap: 4, marginTop: 8 }}>
        {fields.map((f) => (
          <div key={f.label} style={{ display: "flex", gap: 10 }}>
            <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", width: 104, flexShrink: 0 }}>
              {f.label}
            </span>
            <span style={{ fontSize: 11.5 }}>{f.value}</span>
          </div>
        ))}
        {stock.InsiderTx && stock.InsiderTx.length > 0 && (
          <div style={{ display: "flex", gap: 10 }}>
            <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", width: 104, flexShrink: 0 }}>
              {dict.llmInsiderTx}
            </span>
            <span style={{ fontSize: 11.5 }}>{formatKV(stock.InsiderTx)}</span>
          </div>
        )}
        {stock.Candles && stock.Candles.length > 0 && (
          <div style={{ display: "flex", gap: 10 }}>
            <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", width: 104, flexShrink: 0 }}>
              {dict.llmCandlesSummary}
            </span>
            <span style={{ fontSize: 11.5 }}>
              {stock.Candles.length} ({stock.Candles[0].Date} → {stock.Candles[stock.Candles.length - 1].Date})
            </span>
          </div>
        )}
        {stock.StrategyHits && stock.StrategyHits.length > 0 && (
          <div style={{ display: "flex", gap: 10 }}>
            <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", width: 104, flexShrink: 0 }}>
              {dict.llmStrategyHits}
            </span>
            <span style={{ fontSize: 11.5 }}>{stock.StrategyHits.map((h) => `${h.Name} (${h.DaysAgo}d)`).join(", ")}</span>
          </div>
        )}
        {stock.PastLessons && stock.PastLessons.length > 0 && (
          <div style={{ display: "flex", gap: 10 }}>
            <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", width: 104, flexShrink: 0 }}>
              {dict.llmPerTickerLessons}
            </span>
            <span style={{ fontSize: 11.5 }}>{stock.PastLessons.map((l) => `${l.Date}: ${l.Lesson}`).join(" / ")}</span>
          </div>
        )}
      </div>
    </div>
  );
}

function RunSidebar({
  dict,
  runs,
  selectedId,
  onOpenRun,
}: {
  dict: Dictionary;
  runs: LLMRunSummary[];
  selectedId: number | null;
  onOpenRun: (id: number) => void;
}) {
  return (
    <div className="card llm-sidebar">
      {runs.map((r) => (
        <button
          key={r.id}
          className={`llm-sidebar-btn${r.id === selectedId ? " active" : ""}`}
          onClick={() => onOpenRun(r.id)}
        >
          <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <span className={`tag${r.kind === "recommend" ? " tint" : ""}`}>
              {r.kind === "recommend" ? dict.llmRunRecommend : dict.llmRunDailyReport}
            </span>
          </span>
          <span className="mono">{r.createdAt}</span>
          <span style={{ fontSize: 11, color: "var(--ink-3)" }}>
            {r.market.toUpperCase()} · {r.model} · {r.latencyMs}ms
          </span>
        </button>
      ))}
    </div>
  );
}

function RunDetail({
  dict,
  names,
  writable,
  onUnauthorized,
  runId,
}: {
  dict: Dictionary;
  names: Record<string, string>;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  runId: number;
}) {
  const [detail, setDetail] = useState<LLMRunDetail | null>(null);
  const [error, setError] = useState(false);
  const [blockedSources, setBlockedSources] = useState<BlockedNewsSource[] | null>(null);

  const loadBlocked = () => fetchBlockedNewsSources().then((r) => setBlockedSources(r.sources));

  useEffect(() => {
    setDetail(null);
    setError(false);
    fetchLLMRunDetail(runId)
      .then(setDetail)
      .catch(() => setError(true));
    loadBlocked().catch(() => setBlockedSources([]));
  }, [runId]);

  const doBlock = (source: string) => {
    blockNewsSource(source)
      .then(() => loadBlocked())
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) onUnauthorized(() => doBlock(source));
      });
  };

  const doUnblock = (source: string) => {
    unblockNewsSource(source)
      .then(() => loadBlocked())
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) onUnauthorized(() => doUnblock(source));
      });
  };

  if (error) return <div className="error-message">{dict.error}</div>;
  if (!detail || !blockedSources) return <div className="loading">{dict.loading}</div>;

  const blocked = new Set(blockedSources.map((b) => normTitle(b.source)));
  const news = allNews(detail);
  const stocks = [...detail.input.watchlist, ...detail.input.candidates];
  const dupes = duplicateTitleSet(news);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="card">
        <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
          <span className="mono" style={{ fontSize: 15 }}>
            #{detail.id}
          </span>
          <span className="mono" style={{ fontSize: 12, color: "var(--ink-2)" }}>
            {detail.createdAt}
          </span>
          <span className={`tag${detail.kind === "recommend" ? " tint" : ""}`}>
            {detail.kind === "recommend" ? dict.llmRunRecommend : dict.llmRunDailyReport}
          </span>
          <span className="tag">{detail.market.toUpperCase()}</span>
        </div>
        <div className="dash-kpi-strip" style={{ marginTop: 12, marginBottom: 0 }}>
          <StatBlock label={dict.llmModel} value={detail.model} />
          <StatBlock label={dict.llmLatency} value={detail.latencyMs} />
          <StatBlock label={dict.llmWatchlist} value={detail.watchlistCount} />
          <StatBlock label={dict.llmCandidates} value={detail.candidateCount} />
        </div>
      </div>

      <div className="card">
        <div className="eyebrow" style={{ marginBottom: 10 }}>
          {dict.llmDataQuality}
        </div>
        <div className="llm-quality-grid">
          <QualityBlock
            label={dict.llmLowQualitySource}
            value={countLowQuality(news, blocked)}
            warn={countLowQuality(news, blocked) > 0}
          />
          <QualityBlock
            label={dict.llmStaleNews}
            value={countStale(news, detail.createdAt)}
            warn={countStale(news, detail.createdAt) > 2}
          />
          <QualityBlock
            label={dict.llmDuplicateTitles}
            value={countDuplicateTitles(news)}
            warn={countDuplicateTitles(news) > 0}
          />
          <QualityBlock label={dict.llmCandleGaps} value={detail.candleGapCount} warn={detail.candleGapCount > 0} />
        </div>
      </div>

      <div className="card">
        <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
          <span className="eyebrow">{dict.llmBlockedSources}</span>
          {blockedSources.map((b) => (
            <span key={b.source} className="llm-chip">
              {b.source}
              {writable && (
                <button className="llm-chip-x" aria-label="unblock" onClick={() => doUnblock(b.source)}>
                  ×
                </button>
              )}
            </span>
          ))}
        </div>
        {blockedSources.length > 0 ? (
          <div style={{ fontSize: 11.5, color: "var(--ink-3)", marginTop: 8 }}>{dict.llmBlockedHint}</div>
        ) : (
          <div className="empty-message">{dict.llmNoBlockedSources}</div>
        )}
      </div>

      <div className="card" style={{ overflowX: "auto" }}>
        <div className="eyebrow">{dict.llmNewsMarket}</div>
        <table className="mono" style={{ width: "100%" }}>
          <thead>
            <tr>
              <th>{dict.llmSource}</th>
              <th>{dict.llmPublishedAt}</th>
              <th>{dict.llmHeadline}</th>
              {writable && <th />}
            </tr>
          </thead>
          <tbody>
            {detail.input.marketNews.map((n, i) => (
              <NewsRow
                key={i}
                dict={dict}
                news={n}
                createdAt={detail.createdAt}
                isDuplicate={dupes.has(normTitle(n.Headline))}
                blocked={blocked}
                writable={writable}
                onBlock={doBlock}
              />
            ))}
          </tbody>
        </table>

        <div className="eyebrow" style={{ marginTop: 14 }}>
          {dict.llmNewsPerTicker}
        </div>
        <table className="mono" style={{ width: "100%" }}>
          <thead>
            <tr>
              <th>{dict.llmSource}</th>
              <th>{dict.llmScope}</th>
              <th>{dict.llmPublishedAt}</th>
              <th>{dict.llmHeadline}</th>
              {writable && <th />}
            </tr>
          </thead>
          <tbody>
            {stocks.flatMap((s, si) => {
              const ticker = (s.Quote?.Ticker as string) || "?";
              return (s.News ?? []).map((n, ni) => (
                <NewsRow
                  key={`${si}-${ni}`}
                  dict={dict}
                  news={n}
                  scope={tickerLabel(ticker, names)}
                  createdAt={detail.createdAt}
                  isDuplicate={dupes.has(normTitle(n.Headline))}
                  blocked={blocked}
                  writable={writable}
                  onBlock={doBlock}
                />
              ));
            })}
          </tbody>
        </table>
      </div>

      <div className="card">
        <div className="eyebrow" style={{ marginBottom: 10 }}>
          {dict.llmStocksTitle}
        </div>
        <div className="llm-cards-grid">
          {stocks.map((s, i) => (
            <StockCard key={i} dict={dict} names={names} stock={s} />
          ))}
        </div>
      </div>

      <div className="llm-pair-grid">
        {detail.input.marketContext && (
          <div className="card">
            <div className="eyebrow" style={{ marginBottom: 8 }}>
              {dict.llmMarketContext}
            </div>
            <div className="mono" style={{ fontSize: 12, color: "var(--ink-2)", lineHeight: 1.7 }}>
              {formatMarketContext(detail.input.marketContext)}
            </div>
          </div>
        )}
        {detail.input.recentLessons && detail.input.recentLessons.length > 0 && (
          <div className="card">
            <div className="eyebrow" style={{ marginBottom: 8 }}>
              {dict.llmCrossTickerLessons}
            </div>
            {detail.input.recentLessons.map((l, i) => (
              <div className="stat-note" key={i}>
                {tickerLabel(l.Ticker, names)} · {l.Date}: {l.Lesson}
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.llmRawReply}</div>
        <pre className="mono llm-raw" style={{ whiteSpace: "pre-wrap" }}>
          {detail.outputRaw}
        </pre>
      </div>
    </div>
  );
}

export function LlmRunsView({ dict, names = {}, writable, onUnauthorized, runId, onOpenRun }: Props) {
  const [runs, setRuns] = useState<LLMRunSummary[] | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    fetchLLMRuns()
      .then((r) => setRuns(r.runs))
      .catch(() => setError(true));
  }, []);

  useEffect(() => {
    if (runId === null && runs && runs.length > 0) onOpenRun(runs[0].id);
  }, [runId, runs, onOpenRun]);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span className="eyebrow">{dict.navLlm}</span>
          <span className="tag tint">{dict.llmDevTag}</span>
        </div>
        <span style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{dict.llmSubtitle}</span>
      </div>
      {error ? (
        <div className="error-message">{dict.error}</div>
      ) : !runs ? (
        <div className="loading">{dict.loading}</div>
      ) : runs.length === 0 ? (
        <div className="empty-message">{dict.llmNoRuns}</div>
      ) : (
        <div className="llm-layout">
          <RunSidebar dict={dict} runs={runs} selectedId={runId} onOpenRun={onOpenRun} />
          {runId !== null && (
            <RunDetail dict={dict} names={names} writable={writable} onUnauthorized={onUnauthorized} runId={runId} />
          )}
        </div>
      )}
    </div>
  );
}
