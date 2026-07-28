import { useEffect, useState } from "react";
import { addWatchlistTicker, ApiError, fetchTickers, removeWatchlistTicker, tickerLabel, type Market } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  market: Market;
  onOpenTicker: (ticker: string) => void;
  // names is /api/company-names' TW ticker → Chinese-name map (see App.tsx).
  names?: Record<string, string>;
  // writable/onUnauthorized/onSuccess (Phase 10, docs/phase-10-web-trade-
  // input.md §4.3) gate the add-ticker form and per-row remove button —
  // this page is watchlist ∪ held tickers, so it doubles as the watchlist
  // management page (§2 item 6). Removing a ticker that's still a held
  // position only touches the watchlist row, matching Telegram /remove.
  writable?: boolean;
  onUnauthorized?: (retry: () => void) => void;
  onSuccess?: () => void;
}

// Phase 7's /chart list page: the ticker picker for the support/resistance
// chart view (docs/phase-7-support-resistance.md §5.1) — watchlist ∪
// held tickers in market, same reasoning as RoundsListView's picker but
// sourced from /api/tickers instead of /api/rounds since a round only
// exists for a ticker that's actually been traded, while a chart is useful
// for anything being watched.
export function ChartListView({
  dict,
  market,
  onOpenTicker,
  names = {},
  writable = false,
  onUnauthorized,
  onSuccess,
}: Props) {
  const [tickers, setTickers] = useState<string[] | null>(null);
  const [error, setError] = useState(false);
  const [newTicker, setNewTicker] = useState("");
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  function reload() {
    setTickers(null);
    setError(false);
    fetchTickers(market)
      .then((r) => setTickers(r.tickers))
      .catch(() => setError(true));
  }

  useEffect(reload, [market]);

  async function handleAdd() {
    const ticker = newTicker.trim().toUpperCase();
    if (!ticker) return;
    setBusy(true);
    setActionError(null);
    try {
      await addWatchlistTicker(ticker);
      setNewTicker("");
      reload();
      onSuccess?.();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(handleAdd);
      } else {
        setActionError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setBusy(false);
    }
  }

  async function handleRemove(ticker: string) {
    setBusy(true);
    setActionError(null);
    try {
      await removeWatchlistTicker(ticker);
      reload();
      onSuccess?.();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(() => handleRemove(ticker));
      } else {
        setActionError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setBusy(false);
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  return (
    <div className="card">
      <div className="eyebrow">{dict.navChart}</div>
      {writable && (
        <div className="add-ticker-form">
          <input
            className="mono"
            value={newTicker}
            placeholder={dict.addTickerPlaceholder}
            onChange={(e) => setNewTicker(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleAdd();
            }}
          />
          <button disabled={busy || !newTicker.trim()} onClick={handleAdd}>
            {dict.add}
          </button>
        </div>
      )}
      {actionError && <div className="error-message">{actionError}</div>}
      {!tickers ? (
        <div className="loading">{dict.loading}</div>
      ) : tickers.length === 0 ? (
        <div className="empty-message">{dict.pickTicker}</div>
      ) : (
        <table className="mono">
          <thead>
            <tr>
              <th>{dict.ticker}</th>
              {writable && <th />}
            </tr>
          </thead>
          <tbody>
            {tickers.map((t) => (
              <tr key={t} className="round-row">
                <td tabIndex={0} onClick={() => onOpenTicker(t)} onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") onOpenTicker(t);
                }}>
                  {tickerLabel(t, names)}
                </td>
                {writable && (
                  <td className="row-actions">
                    <button disabled={busy} onClick={() => handleRemove(t)}>
                      {dict.remove}
                    </button>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
