import { useEffect, useState } from "react";
import { fetchRounds, type RoundSummary } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  onOpenRound: (ticker: string, start: string) => void;
}

function fmtSigned(v: number): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}$${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

// The picker for Phase 5 PR3's round detail page: every position round trip
// (design doc's "首次買進 → 清倉歸零算一個回合"), most-recently-started
// first — an open round (still held) has no End date, shown as dict.open
// instead.
export function RoundsListView({ dict, onOpenRound }: Props) {
  const [rounds, setRounds] = useState<RoundSummary[] | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    fetchRounds()
      .then((r) => setRounds(r.rounds))
      .catch(() => setError(true));
  }, []);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!rounds) {
    return <div className="loading">{dict.loading}</div>;
  }
  if (rounds.length === 0) {
    return <div className="empty-message">{dict.noRounds}</div>;
  }

  return (
    <div className="card">
      <div className="eyebrow">{dict.navRounds}</div>
      <table className="mono">
        <thead>
          <tr>
            <th>{dict.ticker}</th>
            <th>{dict.startDate}</th>
            <th>{dict.endDate}</th>
            <th>{dict.shares}</th>
            <th>{dict.realizedPnL}</th>
          </tr>
        </thead>
        <tbody>
          {rounds.map((r) => (
            <tr
              key={`${r.ticker}-${r.start}`}
              className="round-row"
              tabIndex={0}
              onClick={() => onOpenRound(r.ticker, r.start)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") onOpenRound(r.ticker, r.start);
              }}
            >
              <td>{r.ticker}</td>
              <td>{r.start}</td>
              <td>{r.open ? <span className="eyebrow">{dict.open}</span> : r.end}</td>
              <td>{r.shares}</td>
              <td className={r.realizedPnL > 0 ? "profit" : r.realizedPnL < 0 ? "loss" : ""}>
                {fmtSigned(r.realizedPnL)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
