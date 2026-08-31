import { useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { ApiError, executeOptionClose, executeOptionOpen, type OptionPosition } from "../api";
import type { Dictionary } from "../i18n";

export type OptionModalMode = "open" | "close";

interface Props {
  dict: Dictionary;
  mode: OptionModalMode;
  // position is required for mode "close" — the contract being closed.
  position?: OptionPosition;
  onClose: () => void;
  onSuccess: () => void;
  onUnauthorized: (retry: () => void) => void;
}

// occSymbol builds the OCC/OSI symbol option.Format (internal/option/
// contract.go) round-trips: underlying + expiry(YYMMDD) + right +
// strike*1000 zero-padded to 8 digits. Kept in lockstep with that Go
// function rather than sending the raw fields, since RecordOption's single
// write path (both here and /obuy /osell) only ever takes a symbol.
function occSymbol(underlying: string, right: "C" | "P", expiry: string, strike: number): string {
  const [y, m, d] = expiry.split("-");
  const strikeMilli = Math.round(strike * 1000);
  return `${underlying.trim().toUpperCase()}${y.slice(2)}${m}${d}${right}${String(strikeMilli).padStart(8, "0")}`;
}

type CloseAction = "BTC" | "STC" | "EXPIRED" | "ASSIGNED" | "EXERCISED";

// OptionModal is the /options page's Add/Close write UI (design-parity
// pass over Phase 12 PR4's read-only page) — POST /api/options/open|close
// (internal/web/options.go), which route through the same bot.recordOption/
// resolveOption /obuy /osell /oassign /oexercise use, so a web-submitted
// order gets the identical naked-call warning and stock-side assign/
// exercise trade. Unlike the design reference's 3-way "kind" picker
// (long/CSP/covered-call) this collects right+side directly — the backend
// only ever needs those two, and the named-strategy layer is pure UI sugar
// this page doesn't have a use for yet.
export function OptionModal({ dict, mode, position, onClose, onSuccess, onUnauthorized }: Props) {
  const [underlying, setUnderlying] = useState("");
  const [right, setRight] = useState<"C" | "P">("C");
  const [side, setSide] = useState<"BUY" | "SELL">("BUY");
  const [strike, setStrike] = useState("");
  const [expiry, setExpiry] = useState("");
  const [contracts, setContracts] = useState("1");
  const [premium, setPremium] = useState("");
  const [fee, setFee] = useState("");
  const [date, setDate] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);

  const tradeCloseAction: CloseAction = position && position.contracts < 0 ? "BTC" : "STC";
  const [closeAction, setCloseAction] = useState<CloseAction>(tradeCloseAction);
  const [closeContracts, setCloseContracts] = useState(position ? String(Math.abs(position.contracts)) : "");
  const [closePremium, setClosePremium] = useState("");
  const [closeFee, setCloseFee] = useState("");
  const [closeDate, setCloseDate] = useState("");

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  async function submitOpen() {
    setSubmitting(true);
    setError(null);
    try {
      const res = await executeOptionOpen({
        symbol: occSymbol(underlying, right, expiry, Number(strike)),
        side,
        contracts: Number(contracts),
        premium: Number(premium),
        fee: fee ? Number(fee) : undefined,
        date: date || undefined,
      });
      setSuccessMsg(res.message);
      onSuccess();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(submitOpen);
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSubmitting(false);
    }
  }

  const needsPrice = closeAction === "BTC" || closeAction === "STC";

  async function submitClose() {
    if (!position) return;
    setSubmitting(true);
    setError(null);
    try {
      const res = await executeOptionClose({
        symbol: position.contractSymbol,
        action: closeAction,
        contracts: needsPrice ? Number(closeContracts) : undefined,
        premium: needsPrice ? Number(closePremium) : undefined,
        fee: needsPrice && closeFee ? Number(closeFee) : undefined,
        date: closeDate || undefined,
      });
      setSuccessMsg(res.message);
      onSuccess();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(submitClose);
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSubmitting(false);
    }
  }

  const canSubmitOpen =
    underlying.trim() !== "" && Number(strike) > 0 && expiry !== "" && Number(contracts) > 0 && Number(premium) >= 0;
  const canSubmitClose = !needsPrice || (Number(closeContracts) > 0 && Number(closePremium) >= 0);

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-backdrop">
          <Dialog.Content
            className="modal modal-wide"
            aria-describedby={undefined}
            onOpenAutoFocus={(e) => e.preventDefault()}
          >
            <div className="modal-header">
              <Dialog.Title className="eyebrow">
                {mode === "open" ? dict.optAddTitle : dict.optCloseTitle}
              </Dialog.Title>
              <Dialog.Close className="modal-close" aria-label="close">
                ×
              </Dialog.Close>
            </div>

            {successMsg ? (
              <div className="modal-body">
                <div className="success-message" style={{ whiteSpace: "pre-wrap" }}>
                  {successMsg}
                </div>
                <div className="modal-actions">
                  <button className="btn-primary" onClick={onClose}>
                    {dict.close}
                  </button>
                </div>
              </div>
            ) : mode === "open" ? (
              <div className="modal-body">
                <div className="seg-toggle">
                  <button type="button" className={right === "C" ? "active" : ""} onClick={() => setRight("C")}>
                    {dict.optCall}
                  </button>
                  <button type="button" className={right === "P" ? "active" : ""} onClick={() => setRight("P")}>
                    {dict.optPut}
                  </button>
                </div>
                <div className="seg-toggle">
                  <button type="button" className={side === "BUY" ? "active" : ""} onClick={() => setSide("BUY")}>
                    {dict.buy}
                  </button>
                  <button type="button" className={side === "SELL" ? "active" : ""} onClick={() => setSide("SELL")}>
                    {dict.sell}
                  </button>
                </div>
                <label className="form-field">
                  <span>{dict.ticker}</span>
                  <input
                    className="mono"
                    value={underlying}
                    onChange={(e) => setUnderlying(e.target.value)}
                    placeholder="AAPL"
                    autoFocus
                  />
                </label>
                <label className="form-field">
                  <span>{dict.optionStrike}</span>
                  <input className="mono" type="number" value={strike} onChange={(e) => setStrike(e.target.value)} placeholder="200" />
                </label>
                <label className="form-field">
                  <span>{dict.optionExpiry}</span>
                  <input className="mono" type="date" value={expiry} onChange={(e) => setExpiry(e.target.value)} />
                </label>
                <label className="form-field">
                  <span>{dict.optionContracts}</span>
                  <input
                    className="mono"
                    type="number"
                    value={contracts}
                    onChange={(e) => setContracts(e.target.value)}
                    placeholder="1"
                  />
                </label>
                <label className="form-field">
                  <span>{dict.optFieldPremium}</span>
                  <input
                    className="mono"
                    type="number"
                    value={premium}
                    onChange={(e) => setPremium(e.target.value)}
                    placeholder="4.20"
                  />
                </label>
                <button type="button" className="advanced-toggle" onClick={() => setShowAdvanced((v) => !v)}>
                  {showAdvanced ? "▾" : "▸"} {dict.advancedOptions}
                </button>
                {showAdvanced && (
                  <>
                    <label className="form-field">
                      <span>{dict.fee}</span>
                      <input className="mono" type="number" value={fee} onChange={(e) => setFee(e.target.value)} />
                    </label>
                    <label className="form-field">
                      <span>{dict.tradeDate}</span>
                      <input className="mono" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
                    </label>
                  </>
                )}
                {error && <div className="error-message">{error}</div>}
                <div className="modal-actions">
                  <button type="button" onClick={onClose}>
                    {dict.cancel}
                  </button>
                  <button type="button" className="btn-primary" disabled={!canSubmitOpen || submitting} onClick={submitOpen}>
                    {dict.submit}
                  </button>
                </div>
              </div>
            ) : (
              <div className="modal-body">
                <div className="mono" style={{ fontSize: 14 }}>
                  {position!.contractSymbol}
                </div>
                <span>{dict.optOutcomeHint}</span>
                <div className="seg-toggle">
                  <button
                    type="button"
                    className={closeAction === tradeCloseAction ? "active" : ""}
                    onClick={() => setCloseAction(tradeCloseAction)}
                  >
                    {tradeCloseAction === "BTC" ? dict.optBuyToClose : dict.optSellToClose}
                  </button>
                  <button
                    type="button"
                    className={closeAction === "EXPIRED" ? "active" : ""}
                    onClick={() => setCloseAction("EXPIRED")}
                  >
                    {dict.optExpiredBtn}
                  </button>
                  {position!.contracts < 0 && (
                    <button
                      type="button"
                      className={closeAction === "ASSIGNED" ? "active" : ""}
                      onClick={() => setCloseAction("ASSIGNED")}
                    >
                      {dict.optAssignedBtn}
                    </button>
                  )}
                  {position!.contracts > 0 && (
                    <button
                      type="button"
                      className={closeAction === "EXERCISED" ? "active" : ""}
                      onClick={() => setCloseAction("EXERCISED")}
                    >
                      {dict.optExercisedBtn}
                    </button>
                  )}
                </div>
                {needsPrice && (
                  <>
                    <label className="form-field">
                      <span>{dict.optionContracts}</span>
                      <input
                        className="mono"
                        type="number"
                        value={closeContracts}
                        onChange={(e) => setCloseContracts(e.target.value)}
                      />
                    </label>
                    <label className="form-field">
                      <span>{dict.optFieldPremium}</span>
                      <input
                        className="mono"
                        type="number"
                        value={closePremium}
                        onChange={(e) => setClosePremium(e.target.value)}
                        placeholder="1.20"
                      />
                    </label>
                    <label className="form-field">
                      <span>{dict.fee}</span>
                      <input className="mono" type="number" value={closeFee} onChange={(e) => setCloseFee(e.target.value)} />
                    </label>
                  </>
                )}
                <label className="form-field">
                  <span>{dict.tradeDate}</span>
                  <input className="mono" type="date" value={closeDate} onChange={(e) => setCloseDate(e.target.value)} />
                </label>
                {error && <div className="error-message">{error}</div>}
                <div className="modal-actions">
                  <button type="button" onClick={onClose}>
                    {dict.cancel}
                  </button>
                  <button type="button" className="btn-primary" disabled={!canSubmitClose || submitting} onClick={submitClose}>
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
