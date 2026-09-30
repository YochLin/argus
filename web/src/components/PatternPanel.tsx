import type { Candle, PatternHit } from "../api";
import type { Dictionary } from "../i18n";
import { PAT_COLORS, patKey, patText, patternConds, patternHistory } from "../patterns";

interface Props {
  dict: Dictionary;
  hits: PatternHit[]; // every pattern, for the same-type history
  visible: PatternHit[]; // after the chips / 只看高信心
  candles: Candle[];
  selKey: string | null;
  onSelect: (key: string | null) => void;
}

// Newest first. A row is badge · name · (gap tag) · end date · confidence: the
// design's row also carried the direction word and a start-date range, which
// wrapped in the ~340px rail. Direction is the badge colour; the range is in
// the detail.
const MAX_EVENTS = 60;

function fill(template: string, arg: string): string {
  return template.replace("%s", () => arg);
}

function spct(v: number, digits: number): string {
  return `${v >= 0 ? "+" : ""}${v.toFixed(digits)}%`;
}

function Badge({ hit, code, big }: { hit: PatternHit; code: string; big?: boolean }) {
  const color = PAT_COLORS[hit.dir];
  return (
    <span
      className={`pat-badge${big ? " big" : ""}${hit.cat === "gap" ? " gap" : ""}`}
      style={{
        color,
        borderColor: color,
        background: hit.cat === "gap" ? (hit.dir === "bull" ? "rgba(16,185,129,.14)" : "rgba(239,68,68,.14)") : undefined,
      }}
    >
      {code}
    </span>
  );
}

// The rail's 型態 tab: the selected pattern's detail on top (why it fired, how
// the same pattern has played out on this ticker), the event list below.
export function PatternPanel({ dict, hits, visible, candles, selKey, onSelect }: Props) {
  const sel = selKey ? hits.find((p) => patKey(p) === selKey) ?? null : null;
  const events = [...visible].reverse().slice(0, MAX_EVENTS);

  const range = (p: PatternHit) => (p.start === p.end ? p.end : `${p.start} → ${p.end}`);

  let detail = null;
  if (sel) {
    const txt = patText(dict, sel.type);
    const hist = patternHistory(hits, sel);
    const bars = candles.filter((c) => c.date >= sel.start && c.date <= sel.end).length;
    const color = PAT_COLORS[sel.dir];
    detail = (
      <div className="pat-detail">
        <div className="pat-head">
          <Badge hit={sel} code={txt.code} big />
          <span className="pat-name">{txt.name}</span>
          <span className="pat-dir" style={{ color, borderColor: color }}>
            {dict[`patDir_${sel.dir}`]}
          </span>
          <span className="pat-cat">{dict[`patCat_${sel.cat}`]}</span>
          <button type="button" className="pat-close" onClick={() => onSelect(null)}>
            ✕
          </button>
        </div>
        <div className="pat-meta">
          <span>
            {range(sel)} · {fill(dict.patBars, String(bars))}
          </span>
          {sel.conf && (
            <span>
              {dict.patConf} {dict[`patConfLabel_${sel.conf}`]}
            </span>
          )}
        </div>
        <div className="pat-def">{txt.def}</div>
        <div>
          <div className="pat-sub">{sel.cat === "gap" ? dict.patWhyGap : dict.patWhy}</div>
          <div className="pat-conds">
            {patternConds(sel, candles, dict).map((c) => (
              <div key={c.label} className="pat-cond">
                <span>{c.label}</span>
                <span className="pat-cond-val">{c.value}</span>
              </div>
            ))}
          </div>
        </div>
        <div>
          <div className="pat-hist-head">
            <span className="pat-sub">{dict.patHist}</span>
            {hist.lowSample && <span className="pat-low">{dict.patLowSample}</span>}
          </div>
          <div className="pat-hist">
            <div>
              <span>{dict.patHistN}</span>
              <span>{hist.n}</span>
            </div>
            <div>
              <span>{dict.patHistAvg}</span>
              <span className={hist.avg5 === null ? "" : hist.avg5 >= 0 ? "profit" : "loss"}>
                {hist.avg5 === null ? "—" : spct(hist.avg5, 1)}
              </span>
            </div>
            <div>
              <span>{sel.dir === "neu" ? dict.patHistUp : dict.patHistHit}</span>
              <span>{hist.hitPct === null ? "—" : `${hist.hitPct}%`}</span>
            </div>
            <div>
              <span>{dict.patThisFwd}</span>
              {sel.fwd5 === null ? (
                <span className="pat-fwd-pending">{dict.patFwdPending}</span>
              ) : (
                <span className={sel.fwd5 >= 0 ? "profit" : "loss"}>{spct(sel.fwd5, 1)}</span>
              )}
            </div>
          </div>
        </div>
        <div className="pat-disclaimer">{dict.patDisclaimer}</div>
      </div>
    );
  }

  return (
    <div className="pat-panel">
      {detail ?? <div className="pat-pick">{dict.patPick}</div>}
      <div className="pat-events">
        <div className="pat-events-head">
          <span className="pat-sub">{dict.patEvents}</span>
          <span className="pat-events-count">{visible.length}</span>
        </div>
        <div className="pat-event-list">
          {events.map((p) => {
            const key = patKey(p);
            const txt = patText(dict, p.type);
            const gapTag =
              p.cat === "gap" ? (p.gap && p.gap.fillDate === "" ? dict.patGapOpen : dict.patGapFilled) : "";
            return (
              <div
                key={key}
                className={`pat-event${key === selKey ? " selected" : ""}`}
                onClick={() => onSelect(key === selKey ? null : key)}
              >
                <Badge hit={p} code={txt.code} />
                <span className="pat-event-name">{txt.name}</span>
                {gapTag && (
                  <span
                    className="pat-gap-tag"
                    style={p.gap && p.gap.fillDate === "" ? { color: PAT_COLORS[p.dir], borderColor: PAT_COLORS[p.dir] } : undefined}
                  >
                    {gapTag}
                  </span>
                )}
                <span className="pat-event-date">{p.end}</span>
                <span className={`pat-event-conf ${p.conf}`}>{p.conf ? dict[`patConfLabel_${p.conf}`] : ""}</span>
              </div>
            );
          })}
          {events.length === 0 && <div className="pat-none">{dict.patNone}</div>}
        </div>
      </div>
    </div>
  );
}
