import { useEffect, useRef, useState } from "react";
import {
  addWatchlistTicker,
  ApiError,
  currencySymbol,
  fetchWatchlistSummary,
  removeWatchlistTicker,
  type Market,
  type WatchlistSummaryItem,
} from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  market: Market;
  onOpenTicker: (ticker: string) => void;
  names?: Record<string, string>;
  writable?: boolean;
  onUnauthorized?: (retry: () => void) => void;
  onSuccess?: () => void;
}

// Edge-to-edge area+line sparkline (mockup's watchCards.area/line paths) —
// sits flush against the card's cropped bottom edge via the wrapper's
// negative margins in theme.css, not this component's own sizing.
function Sparkline({ data }: { data: number[] }) {
  const w = 200;
  const h = 56;
  if (!data || data.length < 2) {
    return <svg viewBox={`0 0 ${w} ${h}`} style={{ display: "block", width: "100%", height: h }} />;
  }
  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;
  const pad = 4;

  const points = data.map((val, idx) => {
    const x = (idx / (data.length - 1)) * w;
    const y = h - pad - ((val - min) / range) * (h - 2 * pad);
    return [x, y] as const;
  });

  const linePath = points.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const areaPath = `${linePath} L${w},${h} L0,${h} Z`;

  const isUp = data[data.length - 1] >= data[0];
  const color = isUp ? "var(--profit)" : "var(--loss)";

  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      style={{ display: "block", width: "100%", height: h }}
    >
      <path d={areaPath} fill={color} fillOpacity={0.1} stroke="none" />
      <path d={linePath} fill="none" stroke={color} strokeWidth="1.6" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

export function ChartListView({
  dict,
  market,
  onOpenTicker,
  names = {},
  writable = false,
  onUnauthorized,
  onSuccess,
}: Props) {
  const [items, setItems] = useState<WatchlistSummaryItem[] | null>(null);
  const [error, setError] = useState(false);
  const [search, setSearch] = useState("");
  const [heldOnly, setHeldOnly] = useState(false);
  const [newTicker, setNewTicker] = useState("");
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  // Guards against out-of-order responses: a mistyped/invalid ticker makes
  // its own fetch the slowest (every provider in the Multi chain fails
  // before giving up), so a reload started before removing it can resolve
  // *after* the reload that confirms the removal and stomp the correct
  // state with stale data. Only the most recently issued reload may apply.
  const reloadSeq = useRef(0);

  function reload() {
    const seq = ++reloadSeq.current;
    setItems(null);
    setError(false);
    fetchWatchlistSummary(market)
      .then((r) => {
        if (seq === reloadSeq.current) setItems(r.tickers);
      })
      .catch(() => {
        if (seq === reloadSeq.current) setError(true);
      });
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

  async function handleRemove(ticker: string, e: React.MouseEvent) {
    e.stopPropagation();
    setBusy(true);
    setActionError(null);
    try {
      await removeWatchlistTicker(ticker);
      reload();
      onSuccess?.();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401 && onUnauthorized) {
        onUnauthorized(() => handleRemove(ticker, e));
      } else {
        setActionError(err instanceof ApiError ? err.message : dict.error);
      }
    } finally {
      setBusy(false);
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const currency = currencySymbol(market);
  const heldTotal = items ? items.filter((item) => item.heldShares > 0).length : 0;
  const filteredItems = items
    ? items.filter((item) => {
        if (heldOnly && item.heldShares <= 0) return false;
        const q = search.trim().toLowerCase();
        if (!q) return true;
        const name = names[item.ticker] || "";
        return item.ticker.toLowerCase().includes(q) || name.toLowerCase().includes(q);
      })
    : [];

  return (
    <>
      <div className="watchlist-header-row">
        <div>
          <div className="eyebrow">
            {dict.watchlistCount} {items ? `(${items.length})` : ""}
          </div>
        </div>
        <div className="watchlist-controls">
          <button
            className={`watchlist-held-filter${heldOnly ? " active" : ""}`}
            aria-pressed={heldOnly}
            onClick={() => setHeldOnly((v) => !v)}
          >
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
              <path d="M2.5 8.5 L6 12 L13.5 4" />
            </svg>
            <span>{dict.heldOnly}</span>
            <span className="watchlist-held-filter-count">{heldTotal}</span>
          </button>
          <input
            className="watchlist-search-input"
            value={search}
            placeholder={dict.searchPlaceholder}
            onChange={(e) => setSearch(e.target.value)}
          />
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
        </div>
      </div>

      {actionError && <div className="error-message">{actionError}</div>}

      {!items ? (
        <div className="loading">{dict.loading}</div>
      ) : filteredItems.length === 0 ? (
        <div className="card empty-message">{search.trim() || heldOnly ? dict.noMatch : dict.pickTicker}</div>
      ) : (
        <div className="watchlist-grid">
          {filteredItems.map((item) => {
            const isProfit = item.changePct >= 0;
            const supPct =
              item.support !== null && item.price > 0 ? (-(item.price - item.support) / item.price) * 100 : null;
            const resPct =
              item.resistance !== null && item.price > 0
                ? ((item.resistance - item.price) / item.price) * 100
                : null;
            return (
              <div
                key={item.ticker}
                className="watchlist-card"
                tabIndex={0}
                onClick={() => onOpenTicker(item.ticker)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") onOpenTicker(item.ticker);
                }}
              >
                <div className="watchlist-card-top">
                  <div className="watchlist-card-id">
                    <div className="watchlist-ticker-code mono">{item.ticker}</div>
                    <div className="watchlist-ticker-name">{names[item.ticker] || ""}</div>
                  </div>
                  <div className="watchlist-card-price">
                    <div className="watchlist-price">
                      {currency}
                      {item.price > 0 ? item.price.toFixed(2) : "—"}
                    </div>
                    <span className={`watchlist-change-badge ${isProfit ? "profit" : "loss"}`}>
                      {item.changePct >= 0 ? "+" : ""}
                      {item.changePct.toFixed(2)}%
                    </span>
                  </div>
                </div>

                <div className="watchlist-card-hold-row">
                  {item.heldShares > 0 && (
                    <span className="watchlist-hold-tag">
                      {dict.positions} {item.heldShares}
                    </span>
                  )}
                </div>

                <div className="watchlist-card-levels">
                  <div>
                    <div className="watchlist-level-label">{dict.nearestSup}</div>
                    <div className="watchlist-level-val profit">
                      {item.support !== null
                        ? `${currency}${item.support.toFixed(2)}  ${supPct! >= 0 ? "+" : ""}${supPct!.toFixed(1)}%`
                        : "—"}
                    </div>
                  </div>
                  <div>
                    <div className="watchlist-level-label">{dict.nearestRes}</div>
                    <div className="watchlist-level-val loss">
                      {item.resistance !== null
                        ? `${currency}${item.resistance.toFixed(2)}  ${resPct! >= 0 ? "+" : ""}${resPct!.toFixed(1)}%`
                        : "—"}
                    </div>
                  </div>
                </div>

                <div className="watchlist-sparkline-wrap">
                  <Sparkline data={item.sparkline} />
                </div>

                {writable && (
                  <button
                    className="watchlist-remove-btn"
                    disabled={busy}
                    onClick={(e) => handleRemove(item.ticker, e)}
                    aria-label={dict.remove}
                  >
                    ×
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </>
  );
}
