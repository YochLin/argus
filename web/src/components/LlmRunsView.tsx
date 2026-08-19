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
  onBack: () => void;
}

// SQLite's CURRENT_TIMESTAMP has no "T"/timezone suffix and is always UTC.
function parseSqliteUTC(s: string): number {
  return new Date(s.replace(" ", "T") + "Z").getTime();
}

function normTitle(s: string): string {
  return s.trim().toLowerCase();
}

function allNews(detail: LLMRunDetail): LLMNewsItem[] {
  const stocks = [...detail.input.watchlist, ...detail.input.candidates];
  return [...detail.input.marketNews, ...stocks.flatMap((s) => s.News ?? [])];
}

function countLowQuality(news: LLMNewsItem[], blocked: Set<string>): number {
  return news.filter((n) => blocked.has(normTitle(n.Source))).length;
}

function countStale(news: LLMNewsItem[], createdAt: string): number {
  const created = parseSqliteUTC(createdAt);
  const cutoffMs = 72 * 3600 * 1000;
  return news.filter((n) => created - new Date(n.PublishedAt).getTime() > cutoffMs).length;
}

function countDuplicateTitles(news: LLMNewsItem[]): number {
  const counts = new Map<string, number>();
  for (const n of news) counts.set(normTitle(n.Headline), (counts.get(normTitle(n.Headline)) ?? 0) + 1);
  let dupes = 0;
  for (const c of counts.values()) if (c > 1) dupes += c;
  return dupes;
}

// ponytail: naive weekday-count gap heuristic (skips Sat/Sun only, not a
// real trading-calendar) — upgrade to an internal/market-equivalent TS check
// if holiday false positives turn out to matter in practice.
function countCandleGaps(stocks: LLMStockData[]): number {
  let gaps = 0;
  for (const s of stocks) {
    const candles = s.Candles;
    if (!candles || candles.length < 2) continue;
    for (let i = 1; i < candles.length; i++) {
      const d = new Date(candles[i - 1].Date);
      const cur = new Date(candles[i].Date).getTime();
      let weekdays = 0;
      while (d.getTime() < cur) {
        d.setDate(d.getDate() + 1);
        const day = d.getDay();
        if (day !== 0 && day !== 6) weekdays++;
      }
      if (weekdays > 1) gaps++;
    }
  }
  return gaps;
}

function formatKV(value: unknown): string {
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value)) return value.length === 0 ? "—" : JSON.stringify(value);
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

// Generic renderer for the object-shaped StockData fields (Quote,
// Fundamentals, AnalystRating, Technicals, Position, PrevRec, ...) — avoids
// hand-typing/hand-rendering every nested Go struct's field list, and stays
// correct as those structs grow new fields (design doc's "never trim a
// field" rule).
function KVTable({ obj }: { obj: Record<string, unknown> }) {
  const entries = Object.entries(obj).filter(([, v]) => v !== null && v !== undefined && v !== "");
  if (entries.length === 0) return null;
  return (
    <table className="mono">
      <tbody>
        {entries.map(([k, v]) => (
          <tr key={k}>
            <td>{k}</td>
            <td>{formatKV(v)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function NewsTable({
  dict,
  news,
  blocked,
  writable,
  onBlock,
}: {
  dict: Dictionary;
  news: LLMNewsItem[];
  blocked: Set<string>;
  writable: boolean;
  onBlock: (source: string) => void;
}) {
  if (news.length === 0) return null;
  return (
    <table className="mono">
      <thead>
        <tr>
          <th>{dict.llmSource}</th>
          <th>{dict.llmPublishedAt}</th>
          <th>{dict.llmHeadline}</th>
          {writable && <th />}
        </tr>
      </thead>
      <tbody>
        {news.map((n, i) => {
          const isBlocked = blocked.has(normTitle(n.Source));
          return (
            <tr key={i} className={isBlocked ? "llm-row-dim" : ""}>
              <td>
                {n.Source} {isBlocked && <span className="tag warn">{dict.llmBlocked}</span>}
              </td>
              <td>{n.PublishedAt}</td>
              <td>
                {n.URL ? (
                  <a href={n.URL} target="_blank" rel="noreferrer">
                    {n.Headline}
                  </a>
                ) : (
                  n.Headline
                )}
              </td>
              {writable && (
                <td>
                  {!isBlocked && (
                    <button className="btn-sm" onClick={() => onBlock(n.Source)}>
                      {dict.llmBlockSource}
                    </button>
                  )}
                </td>
              )}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

function StockCard({ dict, names, stock }: { dict: Dictionary; names: Record<string, string>; stock: LLMStockData }) {
  const ticker = (stock.Quote?.Ticker as string) || "?";
  return (
    <div className="card">
      <div className="eyebrow">{tickerLabel(ticker, names)}</div>
      {stock.ScanReason && (
        <div className="stat-note">
          {dict.llmScanReason}: {stock.ScanReason}
        </div>
      )}
      {stock.Quote && <KVTable obj={stock.Quote} />}
      {stock.Fundamentals && <KVTable obj={stock.Fundamentals} />}
      {stock.AnalystRating && <KVTable obj={stock.AnalystRating} />}
      {stock.InstitutionalFlow && <KVTable obj={stock.InstitutionalFlow} />}
      {stock.Technicals && <KVTable obj={stock.Technicals} />}
      {stock.Position && <KVTable obj={stock.Position} />}
      {stock.PrevRec && <KVTable obj={stock.PrevRec} />}
      {stock.Candles && stock.Candles.length > 0 && (
        <div className="stat-note">
          {dict.llmCandlesSummary}: {stock.Candles.length} ({stock.Candles[0].Date} →{" "}
          {stock.Candles[stock.Candles.length - 1].Date})
        </div>
      )}
      {stock.StrategyHits && stock.StrategyHits.length > 0 && (
        <div className="stat-note">
          {dict.llmStrategyHits}: {stock.StrategyHits.map((h) => `${h.Name} (${h.DaysAgo}d)`).join(", ")}
        </div>
      )}
      {stock.PastLessons && stock.PastLessons.length > 0 && (
        <div className="stat-note">
          {dict.llmPerTickerLessons}: {stock.PastLessons.map((l) => `${l.Date}: ${l.Lesson}`).join(" / ")}
        </div>
      )}
    </div>
  );
}

function RunList({ dict, onOpenRun }: { dict: Dictionary; onOpenRun: (id: number) => void }) {
  const [runs, setRuns] = useState<LLMRunSummary[] | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    fetchLLMRuns()
      .then((r) => setRuns(r.runs))
      .catch(() => setError(true));
  }, []);

  if (error) return <div className="error-message">{dict.error}</div>;
  if (!runs) return <div className="loading">{dict.loading}</div>;
  if (runs.length === 0) return <div className="empty-message">{dict.llmNoRuns}</div>;

  return (
    <div className="card">
      <div className="eyebrow">{dict.navLlm}</div>
      <table className="mono">
        <thead>
          <tr>
            <th>{dict.llmCreated}</th>
            <th>{dict.llmKind}</th>
            <th>Market</th>
            <th>{dict.llmModel}</th>
            <th>{dict.llmLatency}</th>
            <th>{dict.llmWatchlist}</th>
            <th>{dict.llmCandidates}</th>
            <th>{dict.llmNews}</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((r) => (
            <tr
              key={r.id}
              className="round-row"
              tabIndex={0}
              onClick={() => onOpenRun(r.id)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") onOpenRun(r.id);
              }}
            >
              <td>{r.createdAt}</td>
              <td>{r.kind === "recommend" ? dict.llmRunRecommend : r.kind === "daily_report" ? dict.llmRunDailyReport : r.kind}</td>
              <td>{r.market}</td>
              <td>{r.model}</td>
              <td>{r.latencyMs}ms</td>
              <td>{r.watchlistCount}</td>
              <td>{r.candidateCount}</td>
              <td>{r.newsCount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RunDetail({
  dict,
  names,
  writable,
  onUnauthorized,
  runId,
  onBack,
}: {
  dict: Dictionary;
  names: Record<string, string>;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  runId: number;
  onBack: () => void;
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

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <button className="btn-sm" onClick={onBack}>
        ← {dict.navLlm}
      </button>

      <div className="card">
        <div className="eyebrow">
          {detail.kind === "recommend" ? dict.llmRunRecommend : dict.llmRunDailyReport} · {detail.market}
        </div>
        <table className="mono">
          <tbody>
            <tr>
              <td>{dict.llmModel}</td>
              <td>{detail.model}</td>
            </tr>
            <tr>
              <td>{dict.llmLatency}</td>
              <td>{detail.latencyMs}ms</td>
            </tr>
            <tr>
              <td>{dict.llmCreated}</td>
              <td>{detail.createdAt}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div className="card">
        <div className="eyebrow">{dict.llmDataQuality}</div>
        <table className="mono">
          <tbody>
            <tr>
              <td>{dict.llmLowQualitySource}</td>
              <td>{countLowQuality(news, blocked)}</td>
            </tr>
            <tr>
              <td>{dict.llmStaleNews}</td>
              <td>{countStale(news, detail.createdAt)}</td>
            </tr>
            <tr>
              <td>{dict.llmDuplicateTitles}</td>
              <td>{countDuplicateTitles(news)}</td>
            </tr>
            <tr>
              <td>{dict.llmCandleGaps}</td>
              <td>{countCandleGaps(stocks)}</td>
            </tr>
          </tbody>
        </table>
      </div>

      {detail.input.marketContext && (
        <div className="card">
          <div className="eyebrow">{dict.llmMarketContext}</div>
          <KVTable obj={detail.input.marketContext} />
        </div>
      )}

      {detail.input.recentLessons && detail.input.recentLessons.length > 0 && (
        <div className="card">
          <div className="eyebrow">{dict.llmCrossTickerLessons}</div>
          {detail.input.recentLessons.map((l, i) => (
            <div className="stat-note" key={i}>
              {tickerLabel(l.Ticker, names)} · {l.Date}: {l.Lesson}
            </div>
          ))}
        </div>
      )}

      <div className="card">
        <div className="eyebrow">{dict.llmNewsMarket}</div>
        <NewsTable dict={dict} news={detail.input.marketNews} blocked={blocked} writable={writable} onBlock={doBlock} />
      </div>

      <div className="card">
        <div className="eyebrow">{dict.llmNewsPerTicker}</div>
        {stocks.map((s, i) => {
          const items = s.News ?? [];
          if (items.length === 0) return null;
          const ticker = (s.Quote?.Ticker as string) || "?";
          return (
            <div key={i}>
              <div className="stat-note">{tickerLabel(ticker, names)}</div>
              <NewsTable dict={dict} news={items} blocked={blocked} writable={writable} onBlock={doBlock} />
            </div>
          );
        })}
      </div>

      <div className="llm-cards-grid">
        {stocks.map((s, i) => (
          <StockCard key={i} dict={dict} names={names} stock={s} />
        ))}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.llmBlockedSources}</div>
        {blockedSources.length === 0 ? (
          <div className="empty-message">{dict.llmNoBlockedSources}</div>
        ) : (
          <table className="mono">
            <tbody>
              {blockedSources.map((b) => (
                <tr key={b.source}>
                  <td>{b.source}</td>
                  <td>{b.createdAt}</td>
                  {writable && (
                    <td>
                      <button className="btn-sm" onClick={() => doUnblock(b.source)}>
                        {dict.llmUnblock}
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <div className="eyebrow">{dict.llmRawReply}</div>
        <pre className="mono" style={{ whiteSpace: "pre-wrap" }}>
          {detail.outputRaw}
        </pre>
      </div>
    </div>
  );
}

export function LlmRunsView({ dict, names = {}, writable, onUnauthorized, runId, onOpenRun, onBack }: Props) {
  return runId ? (
    <RunDetail dict={dict} names={names} writable={writable} onUnauthorized={onUnauthorized} runId={runId} onBack={onBack} />
  ) : (
    <RunList dict={dict} onOpenRun={onOpenRun} />
  );
}
