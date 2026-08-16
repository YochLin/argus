import { useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { addBuyAlert, ApiError, executeBuy, executeSell, setStopPrice, setThesis } from "../api";
import type { Dictionary } from "../i18n";

export type TradeMode = "buy" | "sell" | "stop" | "buyalert";

interface Props {
  dict: Dictionary;
  mode: TradeMode;
  ticker: string;
  // editableTicker is true only for TopBar's global "+ Trade" entry point —
  // a row-opened modal (PositionsTable) always has a fixed ticker, per
  // docs/phase-10-web-trade-input.md §4.3.
  editableTicker?: boolean;
  prefillPrice?: number;
  onClose: () => void;
  // onSuccess fires once, right after a successful write — callers use it
  // to refresh whatever data view is currently on screen. The modal stays
  // open afterward showing the server's confirmation text until the user
  // closes it themselves.
  onSuccess: () => void;
  // onUnauthorized hands the caller a zero-arg retry closure for the exact
  // request that just 401'd — App.tsx opens LoginModal and calls it back
  // on a successful login, so the trade the user was already filling out
  // resends automatically ("登入成功後重送", §4.3) instead of forcing them
  // to reopen the form.
  onUnauthorized: (retry: () => void) => void;
}

const titleKey: Record<TradeMode, keyof Dictionary> = {
  buy: "tradeBuyTitle",
  sell: "tradeSellTitle",
  stop: "tradeStopTitle",
  buyalert: "tradeBuyAlertTitle",
};

// singlePriceMode covers the two modes that only ever collect a ticker and a
// price — "stop" (positions.stop_price) and "buyalert" (a new buy_alerts
// row) — as opposed to "buy"/"sell" which also need shares/fee/date.
function isSinglePriceMode(mode: TradeMode): boolean {
  return mode === "stop" || mode === "buyalert";
}

export function TradeModal({
  dict,
  mode,
  ticker: initialTicker,
  editableTicker = false,
  prefillPrice,
  onClose,
  onSuccess,
  onUnauthorized,
}: Props) {
  const [ticker, setTicker] = useState(initialTicker);
  const [shares, setShares] = useState("");
  const [price, setPrice] = useState(prefillPrice && prefillPrice > 0 ? String(prefillPrice) : "");
  const [fee, setFee] = useState("");
  const [date, setDate] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [showThesis, setShowThesis] = useState(false);
  const [thesisText, setThesisText] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);
  const [thesisWarning, setThesisWarning] = useState<string | null>(null);

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      const cleanTicker = ticker.trim().toUpperCase();
      const res =
        mode === "stop"
          ? await setStopPrice({ ticker: cleanTicker, price: Number(price) })
          : mode === "buyalert"
            ? await addBuyAlert({ ticker: cleanTicker, price: Number(price) })
            : await (mode === "buy" ? executeBuy : executeSell)({
                ticker: cleanTicker,
                shares: Number(shares),
                price: Number(price),
                fee: fee ? Number(fee) : undefined,
                date: date || undefined,
              });
      setSuccessMsg(res.message);
      // The thesis note is a second, independent write — a failure here
      // must not make the (already-succeeded) buy look like it failed too,
      // see docs Phase 21 §PR1's "two requests" decision.
      if (mode === "buy" && thesisText.trim()) {
        try {
          await setThesis(cleanTicker, thesisText.trim());
        } catch (e) {
          setThesisWarning(e instanceof ApiError ? e.message : dict.error);
        }
      }
      onSuccess();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(submit);
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSubmitting(false);
    }
  }

  const canSubmit = ticker.trim() !== "" && Number(price) > 0 && (isSinglePriceMode(mode) || Number(shares) > 0);

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-backdrop">
          {/* Content nested inside Overlay so .modal-backdrop's flex centering
              keeps working unchanged; onOpenAutoFocus is prevented so the
              autoFocus props below still decide the landing field (Radix would
              otherwise pull focus to the first tabbable element, the × button). */}
          <Dialog.Content
            className="modal"
            aria-describedby={undefined}
            onOpenAutoFocus={(e) => e.preventDefault()}
          >
            <div className="modal-header">
              <Dialog.Title className="eyebrow">{dict[titleKey[mode]]}</Dialog.Title>
              <Dialog.Close className="modal-close" aria-label="close">
                ×
              </Dialog.Close>
            </div>
            {successMsg ? (
              <div className="modal-body">
                <div className="success-message">{successMsg}</div>
                {thesisWarning && (
                  <div className="error-message">
                    {dict.thesisSaveFailedNote}: {thesisWarning}
                  </div>
                )}
                <div className="modal-actions">
                  <button className="btn-primary" onClick={onClose}>
                    {dict.close}
                  </button>
                </div>
              </div>
            ) : (
              <div className="modal-body">
                <label className="form-field">
                  <span>{dict.ticker}</span>
                  <input
                    className="mono"
                    value={ticker}
                    readOnly={!editableTicker}
                    onChange={(e) => setTicker(e.target.value)}
                    autoFocus={editableTicker}
                  />
                </label>
                {!isSinglePriceMode(mode) && (
                  <label className="form-field">
                    <span>{dict.shares}</span>
                    <input
                      className="mono"
                      type="number"
                      value={shares}
                      onChange={(e) => setShares(e.target.value)}
                    />
                  </label>
                )}
                <label className="form-field">
                  <span>{mode === "stop" ? dict.stopPriceCol : mode === "buyalert" ? dict.buyAlertPriceCol : dict.price}</span>
                  <input
                    className="mono"
                    type="number"
                    value={price}
                    onChange={(e) => setPrice(e.target.value)}
                    autoFocus={!editableTicker}
                  />
                </label>
                {!isSinglePriceMode(mode) && (
                  <>
                    <button
                      type="button"
                      className="advanced-toggle"
                      onClick={() => setShowAdvanced((v) => !v)}
                    >
                      {showAdvanced ? "▾" : "▸"} {dict.advancedOptions}
                    </button>
                    {showAdvanced && (
                      <>
                        <label className="form-field">
                          <span>{dict.fee}</span>
                          <input
                            className="mono"
                            type="number"
                            value={fee}
                            onChange={(e) => setFee(e.target.value)}
                          />
                        </label>
                        <label className="form-field">
                          <span>{dict.tradeDate}</span>
                          <input
                            className="mono"
                            type="date"
                            value={date}
                            onChange={(e) => setDate(e.target.value)}
                          />
                        </label>
                      </>
                    )}
                  </>
                )}
                {mode === "buy" && (
                  <>
                    <button
                      type="button"
                      className="advanced-toggle"
                      onClick={() => setShowThesis((v) => !v)}
                    >
                      {showThesis ? "▾" : "▸"} {dict.thesisAddToggle}
                    </button>
                    {showThesis && (
                      <label className="form-field">
                        <textarea
                          className="mono"
                          rows={3}
                          value={thesisText}
                          placeholder={dict.thesisFieldPlaceholder}
                          onChange={(e) => setThesisText(e.target.value)}
                        />
                      </label>
                    )}
                  </>
                )}
                {error && <div className="error-message">{error}</div>}
                <div className="modal-actions">
                  <button type="button" onClick={onClose}>
                    {dict.cancel}
                  </button>
                  <button
                    type="button"
                    className="btn-primary"
                    disabled={!canSubmit || submitting}
                    onClick={submit}
                  >
                    {dict.submit}
                  </button>
                </div>
              </div>
            )}
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
