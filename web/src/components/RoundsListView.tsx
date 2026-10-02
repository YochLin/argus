import { Fragment, useEffect, useState } from "react";
import { currencySymbol, fetchRounds, tickerLabel, type Market, type RoundSummary } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  market: Market;
  onOpenRound: (ticker: string, start: string) => void;
  // names is /api/company-names' TW ticker → Chinese-name map (see App.tsx).
  names?: Record<string, string>;
}

type Status = "all" | "open" | "closed";

export interface Tally {
  n: number;
  won: number;
  net: number;
}

export interface MonthGroup {
  month: string; // "YYYY-MM" of the close date
  rounds: RoundSummary[];
  tally: Tally;
}

export interface YearGroup {
  year: string;
  months: MonthGroup[];
  tally: Tally;
}

export function tally(list: RoundSummary[]): Tally {
  return {
    n: list.length,
    won: list.filter((r) => r.realizedPnL > 0).length,
    net: list.reduce((a, r) => a + r.realizedPnL, 0),
  };
}

// Calendar days, at least 1 (a same-day round reads "1 天", per the design
// template); an open round runs up to `today`. Not the reports page's
// floor() (same-day = 0): that one feeds bucket statistics, this is a label.
export function holdDays(start: string, end: string, today: string): number {
  const ms = Date.parse(`${end || today}T00:00:00Z`) - Date.parse(`${start}T00:00:00Z`);
  return Math.max(1, Math.round(ms / 86400000));
}

// groupRounds applies the page's filters, then lays the survivors out the way
// the template does: open rounds on their own (newest start first), closed
// rounds under close-year → close-month (newest close first) so a round lands
// in the same month as its realized P&L does in the calendar and reports.
// summary and years describe every closed round, filters ignored — they feed
// the header and the year chips, which must not shrink as you filter.
export function groupRounds(
  rounds: RoundSummary[],
  f: { year: string; status: Status; q: string },
  label: (ticker: string) => string,
) {
  const needle = f.q.trim().toLowerCase();
  const hit = rounds.filter(
    (r) =>
      (!needle || label(r.ticker).toLowerCase().includes(needle)) &&
      (f.status === "all" || (f.status === "open") === r.open),
  );
  const byTicker = (a: RoundSummary, b: RoundSummary) => (a.ticker < b.ticker ? -1 : 1);
  const newestFirst = (key: (r: RoundSummary) => string) => (a: RoundSummary, b: RoundSummary) =>
    key(a) === key(b) ? byTicker(a, b) : key(a) < key(b) ? 1 : -1;

  const closedAll = rounds.filter((r) => !r.open);
  const years = Array.from(new Set(closedAll.map((r) => r.end.slice(0, 4))))
    .sort()
    .reverse()
    .map((year) => ({ year, n: closedAll.filter((r) => r.end.startsWith(year)).length }));

  const closedHit = hit.filter((r) => !r.open && (f.year === "all" || r.end.startsWith(f.year)));
  const groups: YearGroup[] = years
    .filter((y) => f.year === "all" || y.year === f.year)
    .map(({ year }) => {
      const inYear = closedHit.filter((r) => r.end.startsWith(year));
      const months = Array.from(new Set(inYear.map((r) => r.end.slice(0, 7))))
        .sort()
        .reverse()
        .map((month) => {
          const list = inYear.filter((r) => r.end.startsWith(month)).sort(newestFirst((r) => r.end));
          return { month, rounds: list, tally: tally(list) };
        });
      return { year, months, tally: tally(inYear) };
    })
    .filter((g) => g.months.length > 0);

  return {
    summary: tally(closedAll),
    years,
    open: hit.filter((r) => r.open).sort(newestFirst((r) => r.start)),
    groups,
  };
}

function fmtSigned(v: number, currency: string): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${currency}${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

function pnlClass(v: number): string {
  return v > 0 ? "profit" : v < 0 ? "loss" : "";
}

function localToday(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

interface RowProps {
  r: RoundSummary;
  label: string;
  today: string;
  currency: string;
  dict: Dictionary;
  onOpen: () => void;
}

function RoundRow({ r, label, today, currency, dict, onOpen }: RowProps) {
  return (
    <div
      className="rl-grid rl-row"
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
    >
      <span>{label}</span>
      <span>{r.start}</span>
      <span className={r.open ? "rl-open-tag" : undefined}>{r.open ? dict.open : r.end}</span>
      <span className="rl-days">
        {holdDays(r.start, r.end, today)}
        {dict.roundsDaySuffix}
      </span>
      <span className="rl-r">{r.shares.toLocaleString()}</span>
      {/* An open round's partial realized P&L is left out on purpose: the
          totals above only count closed rounds, so showing it here would
          not add up against them. */}
      <span className={`rl-r ${r.open ? "rl-dim" : pnlClass(r.realizedPnL)}`}>
        {r.open ? "—" : fmtSigned(r.realizedPnL, currency)}
      </span>
    </div>
  );
}

// The picker for Phase 5 PR3's round detail page: every position round trip
// (design doc's "首次買進 → 清倉歸零算一個回合"). Phase 6: restricted to
// market's own rounds (see internal/web/rounds.go's buildRounds), refetched
// on toggle change. Layout follows the design template's rounds list: a
// filter bar, open rounds up top, closed rounds grouped by close year/month.
export function RoundsListView({ dict, market, onOpenRound, names = {} }: Props) {
  const [rounds, setRounds] = useState<RoundSummary[] | null>(null);
  const [error, setError] = useState(false);
  const [year, setYear] = useState("all");
  const [status, setStatus] = useState<Status>("all");
  const [q, setQ] = useState("");
  // Only years the user toggled by hand; the rest fall back to defaultOpen.
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const currency = currencySymbol(market);

  useEffect(() => {
    setRounds(null);
    setError(false);
    setYear("all"); // the other market may have no such year
    setExpanded({});
    fetchRounds(market)
      .then((r) => setRounds(r.rounds))
      .catch(() => setError(true));
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!rounds) {
    return <div className="loading">{dict.loading}</div>;
  }
  if (rounds.length === 0) {
    return <div className="empty-message">{dict.noRounds}</div>;
  }

  const label = (ticker: string) => tickerLabel(ticker, names);
  const { summary, years, open, groups } = groupRounds(rounds, { year, status, q }, label);
  const today = localToday();
  const defaultOpen = (y: string) => year !== "all" || y === years[0]?.year || q.trim() !== "";
  const isOpen = (y: string) => expanded[y] ?? defaultOpen(y);
  const setAll = (v: boolean) => setExpanded(Object.fromEntries(years.map((y) => [y.year, v])));
  const row = (r: RoundSummary) => (
    <RoundRow
      key={`${r.ticker}-${r.start}`}
      r={r}
      label={label(r.ticker)}
      today={today}
      currency={currency}
      dict={dict}
      onOpen={() => onOpenRound(r.ticker, r.start)}
    />
  );

  const yearChips = [{ key: "all", label: dict.roundsAll, n: summary.n }].concat(
    years.map((y) => ({ key: y.year, label: y.year, n: y.n })),
  );
  const statusChips: { key: Status; label: string }[] = [
    { key: "all", label: dict.roundsAll },
    { key: "open", label: dict.roundsStatusOpen },
    { key: "closed", label: dict.roundsStatusClosed },
  ];

  return (
    <div className="card rl-card">
      <div className="rl-head">
        <span className="eyebrow">{dict.navRounds}</span>
        <span className="rl-summary">
          <span>
            {summary.n} {dict.roundsClosedSuffix}
          </span>
          <span>·</span>
          <span>
            {dict.roundsWon} {summary.won}/{summary.n}
          </span>
          <span>·</span>
          <span className={pnlClass(summary.net)}>
            {dict.roundsNet} {fmtSigned(summary.net, currency)}
          </span>
        </span>
      </div>

      <div className="rl-filters">
        <div className="rl-chips">
          {yearChips.map((c) => (
            <button
              key={c.key}
              type="button"
              className={`rl-chip${year === c.key ? " active" : ""}`}
              aria-pressed={year === c.key}
              onClick={() => {
                setYear(c.key);
                setExpanded({});
              }}
            >
              <span>{c.label}</span>
              <span className="rl-chip-n">{c.n}</span>
            </button>
          ))}
        </div>
        <div className="rl-chips">
          {statusChips.map((c) => (
            <button
              key={c.key}
              type="button"
              className={`rl-chip${status === c.key ? " active" : ""}`}
              aria-pressed={status === c.key}
              onClick={() => setStatus(c.key)}
            >
              {c.label}
            </button>
          ))}
        </div>
        <input
          className="rl-search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={dict.searchPlaceholder}
          aria-label={dict.searchPlaceholder}
        />
        <div className="rl-toggles">
          <button type="button" onClick={() => setAll(true)}>
            {dict.roundsExpandAll}
          </button>
          <button type="button" onClick={() => setAll(false)}>
            {dict.roundsCollapseAll}
          </button>
        </div>
      </div>

      <div className="rl-grid rl-colhead">
        <span>{dict.ticker}</span>
        <span>{dict.startDate}</span>
        <span>{dict.endDate}</span>
        <span>{dict.roundsHeld}</span>
        <span className="rl-r">{dict.shares}</span>
        <span className="rl-r">{dict.realizedPnL}</span>
      </div>

      {open.length > 0 && (
        <>
          <div className="rl-open-head">
            <span className="rl-dot" />
            <span>{dict.roundsStatusOpen}</span>
            <span className="rl-open-n">{open.length}</span>
          </div>
          {open.map(row)}
        </>
      )}

      {groups.map((g) => {
        const expandedNow = isOpen(g.year);
        return (
          <div className="rl-year-wrap" key={g.year}>
            <button
              type="button"
              className="rl-year"
              aria-expanded={expandedNow}
              onClick={() => setExpanded({ ...expanded, [g.year]: !expandedNow })}
            >
              <span className="rl-caret">{expandedNow ? "▾" : "▸"}</span>
              <span className="rl-year-label">{g.year}</span>
              <span className="rl-dim-3">
                {g.tally.n} {dict.roundsCountSuffix}
              </span>
              <span className="rl-year-win">
                {dict.roundsWon} {g.tally.won}/{g.tally.n}
              </span>
              <span className={`rl-year-net ${pnlClass(g.tally.net)}`}>{fmtSigned(g.tally.net, currency)}</span>
            </button>
            {expandedNow &&
              g.months.map((m) => (
                <Fragment key={m.month}>
                  <div className="rl-month">
                    <span className="rl-month-label">{dict.months[Number(m.month.slice(5)) - 1]}</span>
                    <span>
                      {m.tally.n} {dict.roundsCountSuffix} · {dict.roundsWon} {m.tally.won}/{m.tally.n}
                    </span>
                    <span className={`rl-month-net ${pnlClass(m.tally.net)}`}>{fmtSigned(m.tally.net, currency)}</span>
                  </div>
                  {m.rounds.map(row)}
                </Fragment>
              ))}
          </div>
        );
      })}

      {open.length === 0 && groups.length === 0 && <div className="rl-empty">{dict.roundsNoMatch}</div>}
      <div className="rl-note">{dict.roundsFootnote}</div>
    </div>
  );
}
