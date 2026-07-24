import { useEffect, useState } from "react";
import { fetchTickers, type Market } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  market: Market;
  onOpenTicker: (ticker: string) => void;
}

// Phase 7's /chart list page: the ticker picker for the support/resistance
// chart view (docs/phase-7-support-resistance.md §5.1) — watchlist ∪
// held tickers in market, same reasoning as RoundsListView's picker but
// sourced from /api/tickers instead of /api/rounds since a round only
// exists for a ticker that's actually been traded, while a chart is useful
// for anything being watched.
export function ChartListView({ dict, market, onOpenTicker }: Props) {
  const [tickers, setTickers] = useState<string[] | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    setTickers(null);
    setError(false);
    fetchTickers(market)
      .then((r) => setTickers(r.tickers))
      .catch(() => setError(true));
  }, [market]);

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }
  if (!tickers) {
    return <div className="loading">{dict.loading}</div>;
  }
  if (tickers.length === 0) {
    return <div className="empty-message">{dict.pickTicker}</div>;
  }

  return (
    <div className="card">
      <div className="eyebrow">{dict.navChart}</div>
      <table className="mono">
        <thead>
          <tr>
            <th>{dict.ticker}</th>
          </tr>
        </thead>
        <tbody>
          {tickers.map((t) => (
            <tr
              key={t}
              className="round-row"
              tabIndex={0}
              onClick={() => onOpenTicker(t)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") onOpenTicker(t);
              }}
            >
              <td>{t}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
