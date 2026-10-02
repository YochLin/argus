import { useEffect, useState } from "react";
import { fetchFillSnapshot, type FillNews, type FillSnapshot, type FillSnapshotBody } from "../api";
import type { Dictionary } from "../i18n";
import {
  alignment,
  hindsightRows,
  newsTime,
  readings,
  safeHref,
  snapshotJson,
  type FillRow,
  type Reading,
  type Tone,
} from "../fillSnapshot";
import { PAT_COLORS, patKeyOf, patText } from "../patterns";

interface Props {
  dict: Dictionary;
  ticker: string;
  rows: FillRow[]; // newest first
  current: FillRow | null; // the fill being shown
  onPick: (id: number) => void;
  currency: string;
  // Called with a pattern key (see patKey) when a candle-event chip is clicked
  // and the chart has that pattern; null disables the chip.
  patternKnown: (key: string) => boolean;
  onPattern: (key: string) => void;
}

function fill(template: string, ...args: string[]): string {
  return args.reduce((t, a) => t.replace("%s", () => a), template);
}

const spct = (v: number, digits: number) => `${v >= 0 ? "+" : ""}${v.toFixed(digits)}%`;

// signals.fillCrossLookback in Go: the day count stops there.
const CROSS_CAP = 60;

const TAG_COLORS: Record<string, string> = {
  earn: "#f59e0b",
  guide: "#f59e0b",
  analyst: "#818cf8",
  sector: "#38bdf8",
  macro: "#94a3b8",
  flow: "#a78bfa",
  other: "#94a3b8",
};

const MARK: Record<Tone, string> = { ok: "✓", bad: "✕", neu: "·" };

function SideBadge({ dict, side }: { dict: Dictionary; side: "BUY" | "SELL" }) {
  return <span className={`snap-side ${side === "BUY" ? "buy" : "sell"}`}>{side === "BUY" ? dict.buy : dict.sell}</span>;
}

interface Card {
  id: Reading["id"];
  label: string;
  value: string;
  tag: string;
  sub: string;
  gauge: number | null; // 0–100, RSI only
}

// The four indicator cards' text. The tones come from readings().
function cards(dict: Dictionary, s: FillSnapshotBody): Card[] {
  const rsiTag = { hot: dict.rsiHot, cold: dict.rsiCold, mid: dict.rsiMid }[s.rsiZone];
  const cross = s.macdCrossDays >= CROSS_CAP ? `${CROSS_CAP}+` : String(s.macdCrossDays);
  const trend = {
    bull: [dict.trendBullVal, dict.trendBullTag],
    bear: [dict.trendBearVal, dict.trendBearTag],
    range: [dict.trendRangeVal, dict.trendRangeTag],
  }[s.trend];
  const vol = { up: dict.volUpTag, down: dict.volDownTag, flat: dict.volFlatTag }[s.volState];
  return [
    {
      id: "rsi",
      label: dict.indRsi,
      value: s.rsi14.toFixed(1),
      tag: rsiTag,
      sub: fill(dict.rsiSub, s.rsi14Prev5.toFixed(1), s.rsi14 >= s.rsi14Prev5 ? "↑" : "↓"),
      gauge: Math.max(0, Math.min(100, s.rsi14)),
    },
    {
      id: "macd",
      label: dict.indMacd,
      value: `${s.macdHist >= 0 ? "+" : ""}${s.macdHist.toFixed(2)}`,
      tag: fill(s.macdHist > 0 ? dict.macdGolden : dict.macdDeath, cross),
      sub: fill(dict.macdSub, s.macdDif.toFixed(2), s.macdDea.toFixed(2), s.macdWidening ? dict.macdWiden : dict.macdNarrow),
      gauge: null,
    },
    {
      id: "trend",
      label: dict.indTrend,
      value: trend[0],
      tag: trend[1],
      sub: fill(dict.trendSub, spct(s.closeVsMa20Pct, 1), spct(s.ma20Slope5dPct, 2)),
      gauge: null,
    },
    {
      id: "vol",
      label: dict.indVol,
      // A session still running has traded only part of its volume, so the ratio
      // would read as a dried-up market: say so instead of showing it.
      value: s.provisional ? "—" : `${s.volRatio20.toFixed(2)}×`,
      tag: s.provisional ? dict.snapIntradayTag : vol,
      sub: s.provisional ? dict.snapVolPending : fill(dict.volSub, `${s.volRatio5v20.toFixed(2)}×`),
      gauge: null,
    },
  ];
}

function NewsRow({ dict, n }: { dict: Dictionary; n: FillNews }) {
  const href = safeHref(n.url);
  const color = TAG_COLORS[n.tag] ?? TAG_COLORS.other;
  const sentCls = n.sentiment === "bull" ? "profit" : n.sentiment === "bear" ? "loss" : "";
  return (
    <div className="snap-news">
      <div className="snap-news-side">
        <span className="snap-news-time">{newsTime(n.publishedAt)}</span>
        {n.tag && (
          <span
            className={`snap-news-tag${n.major ? " major" : ""}`}
            style={n.major ? { background: color } : { color, borderColor: color }}
            title={n.major ? dict.newsMajor : undefined}
          >
            {(dict as unknown as Record<string, string>)[`newsTag_${n.tag}`] ?? dict.newsTag_other}
          </span>
        )}
      </div>
      <div className="snap-news-body">
        {href ? (
          <a className={`snap-news-head${n.major ? " major" : ""}`} href={href} target="_blank" rel="noopener noreferrer">
            {n.headline}
          </a>
        ) : (
          <span className={`snap-news-head${n.major ? " major" : ""}`}>{n.headline}</span>
        )}
        <span className="snap-news-meta">
          <span>{n.source}</span>
          <span className={sentCls}>{dict[`newsSent_${n.sentiment || "none"}`]}</span>
        </span>
      </div>
    </div>
  );
}

// The rail's 成交快照 tab: pick one of this ticker's fills and see how the stock
// looked that day. Every number comes from /api/fill-snapshot; the tones for a
// buy are fillSnapshot.readings, a sell gets none.
export function FillSnapshotPanel({ dict, ticker, rows, current, onPick, currency, patternKnown, onPattern }: Props) {
  // Keyed by the fill it was fetched for, so a render between picking another
  // fill and the new response never shows the previous fill's numbers.
  const [res, setRes] = useState<{ id: number; data: FillSnapshot } | null>(null);
  const [failedId, setFailedId] = useState<number | null>(null);
  const [listOpen, setListOpen] = useState(false);
  const [jsonOpen, setJsonOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const fillId = current?.fill.id ?? null;
  useEffect(() => {
    setJsonOpen(false);
    setCopied(false);
    if (!current) return;
    const id = current.fill.id;
    let live = true;
    fetchFillSnapshot(ticker, current.fill.date, current.fill.price)
      .then((d) => {
        if (!live) return;
        setRes({ id, data: d });
        setFailedId(null);
      })
      .catch(() => live && setFailedId(id));
    return () => {
      live = false;
    };
    // current is identified by its id; the object itself is rebuilt on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ticker, fillId]);

  if (!current || rows.length === 0) return <div className="snap-note">{dict.snapEmpty}</div>;

  const { fill: f, side } = current;
  const pos = rows.indexOf(current);
  const newer = rows[pos - 1];
  const older = rows[pos + 1];
  const px = (v: number) => `${currency}${v.toFixed(2)}`;
  const data = res && res.id === f.id ? res.data : null;
  const failed = failedId === f.id;
  const snap = data?.snapshot ?? null;

  const step = (row: FillRow | undefined, tip: string, glyph: string) => (
    <button type="button" className="snap-step" title={tip} disabled={!row} onClick={() => row && onPick(row.fill.id)}>
      {glyph}
    </button>
  );

  const picker = (
    <div className="snap-pick">
      {step(older, dict.snapPrevTip, "‹")}
      <button type="button" className="snap-pick-cur" title={dict.snapPickTip} onClick={() => setListOpen((o) => !o)}>
        <SideBadge dict={dict} side={side} />
        <span className="snap-pick-label">
          {f.date}
          {current.roundNo > 0 && ` · #${current.roundNo}`}
        </span>
        <span className="snap-pick-pos">
          {pos + 1} / {rows.length}
        </span>
        <span className="snap-pick-caret">▾</span>
      </button>
      {step(newer, dict.snapNextTip, "›")}
      {listOpen && (
        <>
          <div className="snap-pick-scrim" onClick={() => setListOpen(false)} />
          <div className="snap-pick-list">
            {rows.map((r) => (
              <div
                key={r.fill.id}
                className={`snap-pick-row${r.fill.id === f.id ? " selected" : ""}`}
                onClick={() => {
                  onPick(r.fill.id);
                  setListOpen(false);
                }}
              >
                <SideBadge dict={dict} side={r.side} />
                <span>{r.fill.date}</span>
                <span className="snap-pick-px">{px(r.fill.price)}</span>
                <span className="snap-pick-no">{r.roundNo > 0 ? `#${r.roundNo}` : ""}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );

  const head = (
    <div className="snap-head">
      <div className="snap-head-top">
        <span className="snap-date">{f.date}</span>
        <span className="snap-fill">{fill(dict.snapFillLine, f.shares.toLocaleString(), px(f.price))}</span>
        {current.roundNo > 0 && <span className="snap-round">{fill(dict.snapRound, String(current.roundNo))}</span>}
      </div>
      {snap && (
        <span className="snap-dayline">
          {fill(
            snap.provisional ? dict.snapDayLineLive : dict.snapDayLine,
            spct(snap.dayChangePct, 2),
            snap.close.toFixed(2),
            snap.high.toFixed(2),
            snap.low.toFixed(2),
          )}
        </span>
      )}
      <span className="snap-prov">
        <span className="snap-prov-dot" />
        {snap?.provisional ? dict.snapProvenanceLive : dict.snapProvenance}
        {snap && snap.date !== f.date && ` · ${fill(dict.snapBarNote, snap.date)}`}
      </span>
    </div>
  );

  if (failed) return <div className="snap-col">{picker}{head}<div className="snap-note">{dict.snapLoadFail}</div></div>;
  if (!data) return <div className="snap-col">{picker}{head}<div className="snap-note">{dict.loading}</div></div>;

  const rs = snap ? readings(snap, side) : [];
  const align = alignment(rs);
  const cs = snap ? cards(dict, snap) : [];

  return (
    <div className="snap-col">
      {picker}
      {head}

      {!snap && <div className="snap-note">{dict.snapNoData}</div>}

      {snap && side === "BUY" && (
        <div className="snap-verdict">
          <span className="snap-checks">
            {rs.map((r) => (
              <span key={r.id} className={`snap-check ${r.tone}`} />
            ))}
          </span>
          <span className={`snap-verdict-text${align.ok >= 3 ? " profit" : align.bad >= 2 ? " loss" : ""}`}>
            {fill(dict.snapAligned, String(align.ok)) + (align.bad ? fill(dict.snapAgainst, String(align.bad)) : "")}
          </span>
          <span className="snap-verdict-note">{dict.snapReadAsBuy}</span>
        </div>
      )}
      {snap && side === "SELL" && <div className="snap-note small">{dict.snapSellNote}</div>}

      {snap && (
        <div className="snap-inds">
          {cs.map((c, i) => {
            const tone = rs[i].tone;
            return (
              <div key={c.id} className="snap-ind">
                <div className="snap-ind-top">
                  <span className="snap-ind-label">{c.label}</span>
                  {side === "BUY" && <span className={`snap-ind-mark ${tone}`}>{MARK[tone]}</span>}
                </div>
                <div className="snap-ind-val">
                  <span className="snap-ind-num">{c.value}</span>
                  <span className={`snap-ind-tag ${tone}`}>{c.tag}</span>
                </div>
                {c.gauge !== null && (
                  <div className="snap-gauge">
                    <span style={{ left: `${c.gauge.toFixed(0)}%` }} />
                  </div>
                )}
                <span className="snap-ind-sub">{c.sub}</span>
              </div>
            );
          })}
        </div>
      )}

      {snap && (
        <div>
          <div className="snap-title-row">
            <span className="pat-sub">{dict.snapPatTitle}</span>
            <span className="snap-count">{snap.patterns.length}</span>
          </div>
          <div className="snap-chips">
            {snap.patterns.map((p) => {
              const key = patKeyOf(p.type, snap.date);
              const t = patText(dict, p.type);
              const known = patternKnown(key);
              return (
                <button
                  key={p.type}
                  type="button"
                  className="snap-chip"
                  disabled={!known}
                  onClick={() => onPattern(key)}
                >
                  <span className="snap-chip-dot" style={{ background: PAT_COLORS[p.dir] }} />
                  {t.code} · {t.name}
                </button>
              );
            })}
          </div>
          {snap.patterns.length === 0 && <div className="snap-note small">{dict.snapPatNone}</div>}
        </div>
      )}

      <div>
        <div className="snap-title-row">
          <span className="pat-sub">{dict.snapNewsTitle}</span>
        </div>
        <div className="snap-note small">{dict.snapNewsNote}</div>
        <div className="snap-newslist">
          {data.news.map((n, i) => (
            <NewsRow key={`${i}:${n.headline}`} dict={dict} n={n} />
          ))}
        </div>
        {data.news.length === 0 && <div className="snap-note small">{dict.snapNewsNone}</div>}
      </div>

      {data.hindsight && (
        <div className="snap-hind">
          <div className="snap-hind-head">
            <span className="pat-sub">{dict.hindTitle}</span>
            <span className="snap-note small">{dict.hindNote}</span>
          </div>
          <div className="snap-hind-grid">
            {hindsightRows(data.hindsight, side).map((h) => (
              <div key={h.id}>
                <span className="snap-hind-label">
                  {{ fwd5: dict.hind5, fwd20: dict.hind20, best: dict.hindBest, worst: dict.hindWorst }[h.id]}
                </span>
                <span className={`snap-hind-val ${h.tone}`}>{h.value === null ? dict.hindPending : spct(h.value, 1)}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {snap && (
        <div className="snap-data">
          <div className="snap-data-head">
            <span className="snap-data-title">{dict.snapDataTitle}</span>
            <button type="button" className="snap-btn accent" onClick={() => setJsonOpen((o) => !o)}>
              {jsonOpen ? dict.snapJsonHide : dict.snapJsonView}
            </button>
            <button
              type="button"
              className="snap-btn"
              onClick={() => {
                try {
                  void navigator.clipboard.writeText(snapshotJson(ticker, f, snap, data.news));
                  setCopied(true);
                } catch {
                  /* clipboard unavailable (insecure context); the pane stays viewable */
                }
              }}
            >
              {copied ? dict.snapCopied : dict.snapCopy}
            </button>
          </div>
          <span className="snap-note small">{dict.snapDataNote}</span>
          {jsonOpen && <pre className="snap-json">{snapshotJson(ticker, f, snap, data.news)}</pre>}
        </div>
      )}
    </div>
  );
}
