import { useState } from "react";
import type { DropEvent } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  drops: DropEvent[]; // oldest first
  dates: string[]; // the chart's candle dates; their count is the summary line's window
  benchmark: string;
  currency: string;
  selKey: string | null;
  onSelect: (key: string | null) => void;
  showOnChart: boolean;
  onToggleShow: () => void;
}

export const DROP_COLOR = "#f59e0b";

// A drop's identity on this chart: its first falling bar.
export function dropKey(d: DropEvent): string {
  return d.start;
}

type KindFilter = "all" | "streak" | "gap";

function fill(template: string, ...args: (string | number)[]): string {
  let i = 0;
  return template.replace(/%s/g, () => String(args[i++]));
}

function spct(v: number): string {
  return `${v >= 0 ? "+" : ""}${v.toFixed(1)}%`;
}

function tone(v: number): string {
  return v >= 0 ? "profit" : "loss";
}

// How many days the drop spans is what the badge and the kind label show, and
// the API sends dates only — it's the bars between them, same as the design.
function barsOf(d: DropEvent, dates: string[]): number {
  return dates.filter((x) => x >= d.start && x <= d.end).length;
}

// An index that fell at least half as far means the stock mostly went with it.
function isSystemic(d: DropEvent): boolean {
  return d.indexPct !== null && d.indexPct <= d.pct * 0.5;
}

function Badge({ d, days, big }: { d: DropEvent; days: number; big?: boolean }) {
  return (
    <span className={`drop-badge${big ? " big" : ""}${d.kind === "gap" ? " gap" : ""}`}>
      {d.kind === "gap" ? "GAP" : `▼${days}`}
    </span>
  );
}

// The rail's 下跌事件 tab: the picked drop on top, the filterable list below.
// The numbers are computed from the candles for any ticker. The cause section
// carries the Telegram event's writeup when there is one, and says so when
// there isn't: a past day's news can't be fetched, so most drops have none.
export function DropPanel({
  dict,
  drops,
  dates,
  benchmark,
  currency,
  selKey,
  onSelect,
  showOnChart,
  onToggleShow,
}: Props) {
  const [kind, setKind] = useState<KindFilter>("all");
  const sel = selKey ? drops.find((d) => dropKey(d) === selKey) ?? null : null;
  const recovered = drops.filter((d) => d.recoverBars !== null);
  const avgRec = recovered.length
    ? Math.round(recovered.reduce((s, d) => s + (d.recoverBars ?? 0), 0) / recovered.length)
    : null;
  const vis = drops.filter((d) => kind === "all" || d.kind === kind).reverse();
  const kinds: [KindFilter, string, number][] = [
    ["all", dict.dropAll, drops.length],
    ["streak", dict.dropStreaks, drops.filter((d) => d.kind === "streak").length],
    ["gap", dict.dropGaps, drops.filter((d) => d.kind === "gap").length],
  ];
  const kindLabel = (d: DropEvent) => (d.kind === "gap" ? dict.dropKindGap : fill(dict.dropKindStreak, barsOf(d, dates)));

  let detail = <div className="pat-pick">{dict.dropPick}</div>;
  if (sel) {
    const stats: { label: string; value: string; cls: string }[] = [
      { label: dict.dropStatPct, value: spct(sel.pct), cls: tone(sel.pct) },
      {
        label: fill(dict.dropStatIndex, benchmark),
        value: sel.indexPct === null ? "—" : spct(sel.indexPct),
        cls: sel.indexPct === null ? "drop-dim" : tone(sel.indexPct),
      },
      {
        label: dict.dropStatRecover,
        value: sel.recoverBars === null ? dict.dropNotYet : fill(dict.dropDays, sel.recoverBars),
        cls: sel.recoverBars === null ? "drop-dim" : "",
      },
      {
        label: dict.dropStatFwd,
        value: sel.fwd10Pct === null ? "—" : spct(sel.fwd10Pct),
        cls: sel.fwd10Pct === null ? "drop-dim" : tone(sel.fwd10Pct),
      },
    ];
    detail = (
      <div className="pat-detail">
        <div className="pat-head">
          <Badge d={sel} days={barsOf(sel, dates)} big />
          <span className="drop-kind">{kindLabel(sel)}</span>
          <span className="drop-range">
            {sel.start === sel.end ? sel.end : `${sel.start} → ${sel.end}`}
            {sel.gapPct !== null && ` · ${fill(dict.dropGapped, spct(sel.gapPct))}`}
          </span>
          <button type="button" className="pat-close" onClick={() => onSelect(null)}>
            ✕
          </button>
        </div>
        <div className="drop-cause">
          <span className="pat-sub">{dict.dropCause}</span>
          {sel.summary ? (
            <span className="drop-cause-text">{sel.summary}</span>
          ) : (
            <span className="drop-cause-none">{dict.dropNoCause}</span>
          )}
        </div>
        {sel.indexPct !== null && (
          <div className="drop-scope">
            <span className="drop-scope-tag">{isSystemic(sel) ? dict.dropSystemic : dict.dropIdiosyncratic}</span>
            <span className="drop-scope-note">{isSystemic(sel) ? dict.dropSystemicNote : dict.dropIdiosyncraticNote}</span>
          </div>
        )}
        <div className="pat-hist">
          {stats.map((s) => (
            <div key={s.label}>
              <span>{s.label}</span>
              <span className={s.cls}>{s.value}</span>
            </div>
          ))}
        </div>
        <span className="drop-range">
          {fill(dict.dropPriceNote, `${currency}${sel.preClose.toFixed(2)}`, `${currency}${sel.low.toFixed(2)}`)}
        </span>
        {sel.summary && <div className="pat-disclaimer">{dict.dropDisclaimer}</div>}
      </div>
    );
  }

  return (
    <div className="pat-panel">
      {detail}
      <div className="drop-list-wrap">
        <div className="drop-filters">
          {kinds.map(([k, label, n]) => (
            <button
              key={k}
              type="button"
              className={`drop-filter${kind === k ? " active" : ""}`}
              onClick={() => setKind(k)}
            >
              {label}
              <span className="drop-filter-count">{n}</span>
            </button>
          ))}
          <button type="button" className={`drop-show${showOnChart ? " on" : ""}`} onClick={onToggleShow}>
            {dict.dropShow}
          </button>
        </div>
        <span className="drop-range">
          {fill(dict.dropSummary, dates.length, recovered.length, drops.length)}
          {avgRec !== null && ` · ${fill(dict.dropSummaryAvg, avgRec)}`}
        </span>
        <div className="drop-list">
          {vis.map((d) => {
            const key = dropKey(d);
            return (
              <div
                key={key}
                className={`drop-row${key === selKey ? " selected" : ""}`}
                onClick={() => onSelect(key === selKey ? null : key)}
              >
                <Badge d={d} days={barsOf(d, dates)} />
                <div className="drop-row-body">
                  <span className={d.summary ? "drop-row-summary" : "drop-row-summary none"}>
                    {d.summary || dict.dropNoCauseShort}
                  </span>
                  <div className="drop-row-meta">
                    <span className="drop-range">
                      {d.start === d.end ? d.end.slice(2) : `${d.start.slice(2)} → ${d.end.slice(5)}`}
                    </span>
                    <span className={d.recoverBars !== null ? "drop-rec profit" : "drop-rec"}>
                      {d.recoverBars !== null ? fill(dict.dropRecovered, d.recoverBars) : dict.dropNotRecovered}
                    </span>
                  </div>
                </div>
                <span className={`drop-row-pct ${tone(d.pct)}`}>{spct(d.pct)}</span>
              </div>
            );
          })}
          {vis.length === 0 && <div className="pat-none">{dict.dropNone}</div>}
        </div>
      </div>
    </div>
  );
}
