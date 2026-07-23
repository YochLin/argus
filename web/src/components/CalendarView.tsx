import { Fragment, useEffect, useMemo, useState } from "react";
import { currencySymbol, fetchCalendar, type Calendar, type Market, type Transaction } from "../api";
import type { Dictionary } from "../i18n";
import { TradesTable } from "./TradesTable";

interface Props {
  dict: Dictionary;
  market: Market;
}

interface Cell {
  date: string | null;
  value: number | null;
}

function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

function currentMonth(): string {
  const now = new Date();
  return `${now.getFullYear()}-${pad2(now.getMonth() + 1)}`;
}

function shiftMonth(month: string, delta: number): string {
  const [y, m] = month.split("-").map(Number);
  const d = new Date(y, m - 1 + delta, 1);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}`;
}

function daysInMonth(month: string): number {
  const [y, m] = month.split("-").map(Number);
  return new Date(y, m, 0).getDate();
}

function firstWeekday(month: string): number {
  const [y, m] = month.split("-").map(Number);
  return new Date(y, m - 1, 1).getDay();
}

// buildWeeks lays the month's days into fixed 7-wide rows, padded with
// null cells before day 1 and after the last day so every row aligns under
// the Sun–Sat header regardless of which weekday the month starts/ends on.
function buildWeeks(month: string, valuesByDate: Map<string, number>): Cell[][] {
  const total = daysInMonth(month);
  const lead = firstWeekday(month);

  const cells: Cell[] = [];
  for (let i = 0; i < lead; i++) cells.push({ date: null, value: null });
  for (let day = 1; day <= total; day++) {
    const date = `${month}-${pad2(day)}`;
    cells.push({ date, value: valuesByDate.has(date) ? valuesByDate.get(date)! : null });
  }
  while (cells.length % 7 !== 0) cells.push({ date: null, value: null });

  const weeks: Cell[][] = [];
  for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7));
  return weeks;
}

function fmtSigned(v: number, currency: string): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${currency}${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

function weekTotal(week: Cell[]): number {
  return week.reduce((sum, c) => sum + (c.value ?? 0), 0);
}

export function CalendarView({ dict, market }: Props) {
  const [month, setMonth] = useState(currentMonth());
  const [calendar, setCalendar] = useState<Calendar | null>(null);
  const [error, setError] = useState(false);
  const [selectedDate, setSelectedDate] = useState<string | null>(null);
  const currency = currencySymbol(market);

  useEffect(() => {
    setCalendar(null);
    setError(false);
    setSelectedDate(null);
    fetchCalendar(month, market)
      .then(setCalendar)
      .catch(() => setError(true));
  }, [month, market]);

  const valuesByDate = useMemo(() => {
    const m = new Map<string, number>();
    for (const d of calendar?.days ?? []) m.set(d.date, d.value);
    return m;
  }, [calendar]);

  const transactionsByDate = useMemo(() => {
    const m = new Map<string, Transaction[]>();
    for (const t of calendar?.transactions ?? []) {
      const list = m.get(t.date) ?? [];
      list.push(t);
      m.set(t.date, list);
    }
    return m;
  }, [calendar]);

  const weeks = useMemo(() => buildWeeks(month, valuesByDate), [month, valuesByDate]);
  const monthTotal = useMemo(
    () => (calendar?.days ?? []).reduce((sum, d) => sum + d.value, 0),
    [calendar],
  );

  return (
    <>
      <div className="calendar-nav">
        <button onClick={() => setMonth((m) => shiftMonth(m, -1))} aria-label="prev month">
          ‹
        </button>
        <span className="calendar-month-label mono">{month}</span>
        <button onClick={() => setMonth((m) => shiftMonth(m, 1))} aria-label="next month">
          ›
        </button>
        <button onClick={() => setMonth(currentMonth())}>{dict.today}</button>
        <span className={`calendar-total mono ${monthTotal >= 0 ? "profit" : "loss"}`}>
          {dict.monthTotal}: {fmtSigned(monthTotal, currency)}
        </span>
      </div>

      {error && <div className="error-message">{dict.error}</div>}
      {!error && !calendar && <div className="loading">{dict.loading}</div>}

      {calendar && (
        <>
          <div className="calendar-grid">
            {dict.weekdays.map((wd) => (
              <div className="calendar-weekday" key={wd}>
                {wd}
              </div>
            ))}
            <div className="calendar-weekday">{dict.weekTotal}</div>

            {weeks.map((week, wi) => (
              // Fragments don't add DOM nodes, so the 7 day cells + 1 total
              // cell per week still land as consecutive grid items under
              // the 8-column template — CSS grid doesn't care that they're
              // grouped for React's key purposes.
              <Fragment key={wi}>
                {week.map((cell, ci) =>
                  cell.date === null ? (
                    <div className="cal-cell empty" key={`${wi}-${ci}`} />
                  ) : (
                    <button
                      key={cell.date}
                      className={`cal-cell clickable${cell.value !== null ? (cell.value >= 0 ? " profit-bg" : " loss-bg") : ""}${selectedDate === cell.date ? " selected" : ""}`}
                      onClick={() => setSelectedDate(cell.date)}
                    >
                      <span className="cal-day-num">{Number(cell.date.slice(-2))}</span>
                      <span className="cal-day-value mono">
                        {cell.value !== null ? fmtSigned(cell.value, currency) : ""}
                      </span>
                    </button>
                  ),
                )}
                <div className="calendar-week-total mono">{fmtSigned(weekTotal(week), currency)}</div>
              </Fragment>
            ))}
          </div>

          {selectedDate && (
            <div className="card day-detail-panel">
              <div className="eyebrow">{selectedDate}</div>
              <TradesTable
                dict={dict}
                transactions={transactionsByDate.get(selectedDate) ?? []}
                currency={currency}
              />
            </div>
          )}
        </>
      )}
    </>
  );
}
