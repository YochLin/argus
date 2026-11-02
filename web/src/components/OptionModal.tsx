import { useState, type ReactNode } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  ApiError,
  deleteOptionClosed,
  deleteOptionPosition,
  editOptionClosed,
  editOptionPosition,
  executeOptionClose,
  executeOptionOpen,
  undoOptionClose,
  type OptionClosedTrade,
  type OptionPosition,
} from "../api";
import type { Dictionary } from "../i18n";

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

export function contractLabel(p: { underlying: string; right: string; strike: number }): string {
  return `${p.underlying} ${p.right === "C" ? "CALL" : "PUT"} ${p.strike}`;
}

export type OptionKind = "csp" | "cc" | "long";

export function kindOf(p: { contracts: number; right: string }): OptionKind {
  if (p.contracts > 0) return "long";
  return p.right === "P" ? "csp" : "cc";
}

export function kindLabel(dict: Dictionary, kind: OptionKind): string {
  return kind === "csp" ? dict.optionKindCsp : kind === "cc" ? dict.optionKindCc : dict.optionKindLong;
}

export function daysUntil(date: string): number {
  const ms = new Date(`${date}T00:00:00`).getTime() - new Date().setHours(0, 0, 0, 0);
  return Math.round(ms / 86400000);
}

export function fmtWhole(v: number): string {
  return `$${Math.round(v).toLocaleString()}`;
}

export function fmtSignedWhole(v: number): string {
  return `${v > 0 ? "+" : v < 0 ? "-" : ""}$${Math.abs(Math.round(v)).toLocaleString()}`;
}

function pnlClass(v: number): string {
  return v > 0 ? "profit" : v < 0 ? "loss" : "";
}

interface BaseProps {
  dict: Dictionary;
  onClose: () => void;
  // onDone reloads the page behind the modal and shows message as a toast.
  onDone: (message: string) => void;
  onUnauthorized: (retry: () => void) => void;
}

// useSubmit is the one failure path of every write here: a 401 re-opens the
// login and retries, anything else lands in the modal's error line.
function useSubmit({ dict, onDone, onUnauthorized }: BaseProps) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(write: () => Promise<unknown>, message: string) {
    setSubmitting(true);
    setError(null);
    try {
      await write();
      onDone(message);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => void submit(write, message));
      } else {
        setError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setSubmitting(false);
    }
  }
  return { submit, submitting, error };
}

function Shell({
  title,
  width,
  onClose,
  children,
}: {
  title: string;
  width: number;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-backdrop opt-backdrop">
          <Dialog.Content
            className="modal opt-modal"
            style={{ width }}
            aria-describedby={undefined}
            onOpenAutoFocus={(e) => e.preventDefault()}
          >
            <div className="opt-modal-header">
              <Dialog.Title className="eyebrow">{title}</Dialog.Title>
              <Dialog.Close className="modal-close" aria-label="close">
                ×
              </Dialog.Close>
            </div>
            {children}
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" className={on ? "opt-chip active" : "opt-chip"} onClick={onClick}>
      {children}
    </button>
  );
}

function Field({
  label,
  full,
  invalid,
  type = "number",
  ...input
}: {
  label: string;
  full?: boolean;
  invalid?: boolean;
  type?: string;
  value: string;
  placeholder?: string;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}) {
  return (
    <label className={full ? "opt-field opt-field-full" : "opt-field"}>
      {label}
      <input className={invalid ? "invalid" : ""} type={type} {...input} />
    </label>
  );
}

// DeleteFoot is the design's in-place confirmation: the footer turns into the
// question, so a delete needs no second dialog.
function DeleteFoot({
  dict,
  message,
  disabled,
  onKeep,
  onDelete,
}: {
  dict: Dictionary;
  message: string;
  disabled: boolean;
  onKeep: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="opt-del-foot">
      <span>{message}</span>
      <div>
        <button type="button" className="opt-btn-keep" onClick={onKeep}>
          {dict.optKeep}
        </button>
        <button type="button" className="opt-btn-delete" disabled={disabled} onClick={onDelete}>
          {dict.optConfirmDelete}
        </button>
      </div>
    </div>
  );
}

function Foot({
  dict,
  deleteLabel,
  onAskDelete,
  onClose,
  saveLabel,
  canSave,
  submitting,
  onSave,
}: {
  dict: Dictionary;
  deleteLabel?: string;
  onAskDelete?: () => void;
  onClose: () => void;
  saveLabel: string;
  canSave: boolean;
  submitting: boolean;
  onSave: () => void;
}) {
  return (
    <div className="opt-modal-foot">
      {onAskDelete && (
        <button type="button" className="opt-btn-del-link" onClick={onAskDelete}>
          {deleteLabel}
        </button>
      )}
      <span style={{ flex: 1 }} />
      <button type="button" className="opt-btn-cancel" onClick={onClose}>
        {dict.cancel}
      </button>
      <button
        type="button"
        className="opt-btn-save"
        style={canSave ? undefined : { opacity: 0.5 }}
        disabled={submitting}
        onClick={onSave}
      >
        {saveLabel}
      </button>
    </div>
  );
}

// OptionFormModal is both 開倉 (a strategy pick, then the form) and 編輯部位
// (straight to the form, with a delete in the footer). The strategy stands in
// for side + right: the backend only ever needs those two.
export function OptionFormModal({
  position,
  held,
  ...base
}: BaseProps & { position?: OptionPosition; held: Record<string, number> }) {
  const { dict, onClose } = base;
  const { submit, submitting, error } = useSubmit(base);
  const [step, setStep] = useState<"pick" | "form">(position ? "form" : "pick");
  const [kind, setKind] = useState<OptionKind | null>(position ? kindOf(position) : null);
  const [longRight, setLongRight] = useState<"C" | "P">(position?.right === "C" ? "C" : "P");
  const [underlying, setUnderlying] = useState(position?.underlying ?? "");
  const [strike, setStrike] = useState(position ? String(position.strike) : "");
  const [expiry, setExpiry] = useState(position?.expiry ?? "");
  const [qty, setQty] = useState(position ? String(Math.abs(position.contracts)) : "");
  const [premium, setPremium] = useState(position ? String(position.avgPremium) : "");
  const [fee, setFee] = useState("");
  const [date, setDate] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [tried, setTried] = useState(false);
  const [askDelete, setAskDelete] = useState(false);

  const u = underlying.trim().toUpperCase();
  const right = kind === "long" ? longRight : kind === "cc" ? "C" : "P";
  const strikeN = Number(strike);
  const qtyN = Number(qty);
  const premiumN = Number(premium);
  const valid = u !== "" && strikeN > 0 && expiry !== "" && qtyN > 0 && premiumN >= 0 && premium !== "";
  const label = u ? contractLabel({ underlying: u, right, strike: strikeN }) : "—";

  const heldShares = held[u] ?? 0;
  const lockCash = kind === "csp" ? strikeN * qtyN * 100 : 0;
  const lockShares = kind === "cc" ? qtyN * 100 : 0;
  const naked = kind === "cc" && qtyN > 0 && lockShares > heldShares;
  const isShort = kind !== "long";

  function save() {
    if (!valid) {
      setTried(true);
      return;
    }
    const side = isShort ? "SELL" : "BUY";
    const symbol = occSymbol(u, right, expiry, strikeN);
    if (position) {
      submit(
        () =>
          editOptionPosition({
            symbol: position.contractSymbol,
            newSymbol: symbol,
            side,
            contracts: qtyN,
            premium: premiumN,
          }),
        `${label} ${dict.optUpdatedToast}`,
      );
    } else {
      submit(
        () =>
          executeOptionOpen({
            symbol,
            side,
            contracts: qtyN,
            premium: premiumN,
            fee: fee ? Number(fee) : undefined,
            date: date || undefined,
          }),
        `${label} ${dict.optAddedToast}`,
      );
    }
  }

  const kinds: { id: OptionKind; note: string }[] = [
    { id: "csp", note: dict.optKindCspNote },
    { id: "cc", note: dict.optKindCcNote },
    { id: "long", note: dict.optKindLongNote },
  ];

  return (
    <Shell title={position ? dict.optEditTitle : dict.optAddTitle} width={440} onClose={onClose}>
      <div className="opt-modal-body">
        {step === "pick" ? (
          <>
            <span className="opt-hint">{dict.optPickHint}</span>
            {kinds.map((k) => (
              <button
                key={k.id}
                type="button"
                className="opt-kind-btn"
                onClick={() => {
                  setKind(k.id);
                  setStep("form");
                }}
              >
                <span>
                  <strong>{kindLabel(dict, k.id)}</strong>
                  <small>{k.note}</small>
                </span>
                <span className="opt-kind-arrow">›</span>
              </button>
            ))}
          </>
        ) : (
          <>
            {position && <span className="opt-hint">{dict.optEditNote}</span>}
            <div className="opt-kind-row">
              <span className="opt-kind-tag">{kind && kindLabel(dict, kind)}</span>
              <button
                type="button"
                onClick={() => {
                  setStep("pick");
                  setAskDelete(false);
                }}
              >
                ‹ {dict.optChange}
              </button>
            </div>
            <div className="opt-grid">
              <Field
                full
                type="text"
                label={dict.optFieldUnderlying}
                value={underlying}
                invalid={tried && !u}
                placeholder="AAPL"
                onChange={(e) => setUnderlying(e.target.value)}
              />
              {kind === "long" && (
                <span className="opt-field opt-field-full">
                  {dict.optFieldRight}
                  <span className="opt-chips">
                    <Chip on={longRight === "P"} onClick={() => setLongRight("P")}>
                      PUT
                    </Chip>
                    <Chip on={longRight === "C"} onClick={() => setLongRight("C")}>
                      CALL
                    </Chip>
                  </span>
                </span>
              )}
              <Field
                label={dict.optionStrike}
                value={strike}
                invalid={tried && !(strikeN > 0)}
                placeholder="200"
                onChange={(e) => setStrike(e.target.value)}
              />
              <Field
                type="date"
                label={dict.optionExpiry}
                value={expiry}
                invalid={tried && !expiry}
                onChange={(e) => setExpiry(e.target.value)}
              />
              <Field
                label={dict.optionContracts}
                value={qty}
                invalid={tried && !(qtyN > 0)}
                placeholder="1"
                onChange={(e) => setQty(e.target.value)}
              />
              <Field
                label={dict.optFieldPremium}
                value={premium}
                invalid={tried && (premium === "" || premiumN < 0)}
                placeholder="4.20"
                onChange={(e) => setPremium(e.target.value)}
              />
            </div>
            <div className="opt-preview">
              <div>
                <span className="opt-preview-contract">{label}</span>
                <span className="opt-preview-dte">{expiry ? `${daysUntil(expiry)}d` : "—"}</span>
              </div>
              <div>
                <span className="opt-preview-label">{isShort ? dict.optCreditLabel : dict.optCostLabel}</span>
                <span className={isShort ? "opt-preview-value profit" : "opt-preview-value loss"}>
                  {isShort ? "+" : "−"}
                  {fmtWhole(premiumN * qtyN * 100)}
                </span>
              </div>
              {(lockCash > 0 || lockShares > 0) && (
                <div>
                  <span className="opt-preview-label">
                    {lockCash > 0 ? dict.optPreviewCash : dict.optPreviewShares}
                  </span>
                  <span className="opt-preview-value">
                    {lockCash > 0
                      ? fmtWhole(lockCash)
                      : `${lockShares.toLocaleString()} / ${heldShares.toLocaleString()}`}
                  </span>
                </div>
              )}
              {naked && (
                <div className="opt-naked">
                  <span>NAKED</span>
                  <span>{dict.optNakedWarn}</span>
                </div>
              )}
            </div>
            {!position && (
              <>
                <button type="button" className="advanced-toggle" onClick={() => setShowAdvanced((v) => !v)}>
                  {showAdvanced ? "▾" : "▸"} {dict.advancedOptions}
                </button>
                {showAdvanced && (
                  <div className="opt-grid">
                    <Field label={dict.fee} value={fee} onChange={(e) => setFee(e.target.value)} />
                    <Field type="date" label={dict.tradeDate} value={date} onChange={(e) => setDate(e.target.value)} />
                  </div>
                )}
              </>
            )}
            {error && <div className="error-message">{error}</div>}
          </>
        )}
      </div>
      {step === "form" &&
        (askDelete && position ? (
          <DeleteFoot
            dict={dict}
            message={dict.optDeleteAsk.replace("{label}", contractLabel(position))}
            disabled={submitting}
            onKeep={() => setAskDelete(false)}
            onDelete={() =>
              submit(
                () => deleteOptionPosition(position.contractSymbol),
                `${contractLabel(position)} ${dict.optDeletedToast}`,
              )
            }
          />
        ) : (
          <Foot
            dict={dict}
            deleteLabel={dict.optDeletePosition}
            onAskDelete={position ? () => setAskDelete(true) : undefined}
            onClose={onClose}
            saveLabel={position ? dict.optSaveChanges : dict.optSave}
            canSave={valid}
            submitting={submitting}
            onSave={save}
          />
        ))}
    </Shell>
  );
}

type Outcome = "EXPIRED" | "ASSIGNED" | "EXERCISED" | "CLOSED";

// OptionCloseModal is 了結合約: POST /api/options/close, through the same
// bot.recordOption/resolveOption /osell /oassign /oexercise use, so assign and
// exercise still book their stock trade. Partial size, fee and date sit behind
// 進階選項, which the design doesn't have and this page already supported.
export function OptionCloseModal({ position, ...base }: BaseProps & { position: OptionPosition }) {
  const { dict, onClose } = base;
  const { submit, submitting, error } = useSubmit(base);
  const [outcome, setOutcome] = useState<Outcome>("EXPIRED");
  const [price, setPrice] = useState("");
  const [contracts, setContracts] = useState(String(Math.abs(position.contracts)));
  const [fee, setFee] = useState("");
  const [date, setDate] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [tried, setTried] = useState(false);

  const isShort = position.contracts < 0;
  const needsPrice = outcome === "CLOSED";
  const held = Math.abs(position.contracts);
  const qty = needsPrice ? Number(contracts) : held;
  const signed = isShort ? -qty : qty;
  const exit = needsPrice ? Number(price) : 0;
  const realized =
    (exit - position.avgPremium) * signed * position.multiplier - (needsPrice && fee ? Number(fee) : 0);
  const valid = !needsPrice || (price !== "" && exit >= 0 && qty > 0);
  const label = contractLabel(position);

  function save() {
    if (!valid) {
      setTried(true);
      return;
    }
    const action = outcome === "CLOSED" ? (isShort ? "BTC" : "STC") : outcome;
    submit(
      () =>
        executeOptionClose({
          symbol: position.contractSymbol,
          action,
          contracts: needsPrice ? qty : undefined,
          premium: needsPrice ? exit : undefined,
          fee: needsPrice && fee ? Number(fee) : undefined,
          date: date || undefined,
        }),
      `${label} ${dict.optClosedToast}`,
    );
  }

  return (
    <Shell title={dict.optCloseTitle} width={400} onClose={onClose}>
      <div className="opt-modal-body">
        <div className="opt-contract-row">
          <span className="opt-contract">{label}</span>
          <span className="opt-preview-dte">
            {held} {dict.optionContracts}
          </span>
        </div>
        <span className="opt-hint">{dict.optOutcomeHint}</span>
        <div className="opt-chips">
          <Chip on={outcome === "EXPIRED"} onClick={() => setOutcome("EXPIRED")}>
            {dict.optExpiredBtn}
          </Chip>
          {isShort ? (
            <Chip on={outcome === "ASSIGNED"} onClick={() => setOutcome("ASSIGNED")}>
              {dict.optAssignedBtn}
            </Chip>
          ) : (
            <Chip on={outcome === "EXERCISED"} onClick={() => setOutcome("EXERCISED")}>
              {dict.optExercisedBtn}
            </Chip>
          )}
          <Chip on={outcome === "CLOSED"} onClick={() => setOutcome("CLOSED")}>
            {isShort ? dict.optBuyToClose : dict.optSellToClose}
          </Chip>
        </div>
        {needsPrice && (
          <Field
            label={isShort ? dict.optBuybackLabel : dict.optSellLabel}
            value={price}
            invalid={tried && (price === "" || exit < 0)}
            placeholder="1.20"
            onChange={(e) => setPrice(e.target.value)}
          />
        )}
        <div className="opt-realized">
          <span className="opt-preview-label">{dict.optRealized}</span>
          <span className={pnlClass(realized)}>{fmtSignedWhole(realized)}</span>
        </div>
        {(outcome === "ASSIGNED" || outcome === "EXERCISED") && <span className="opt-hint">{dict.optAssignNote}</span>}
        <button type="button" className="advanced-toggle" onClick={() => setShowAdvanced((v) => !v)}>
          {showAdvanced ? "▾" : "▸"} {dict.advancedOptions}
        </button>
        {showAdvanced && (
          <div className="opt-grid">
            {needsPrice && (
              <Field label={dict.optionContracts} value={contracts} onChange={(e) => setContracts(e.target.value)} />
            )}
            {needsPrice && <Field label={dict.fee} value={fee} onChange={(e) => setFee(e.target.value)} />}
            <Field type="date" label={dict.tradeDate} value={date} onChange={(e) => setDate(e.target.value)} />
          </div>
        )}
        {error && <div className="error-message">{error}</div>}
      </div>
      <Foot
        dict={dict}
        onClose={onClose}
        saveLabel={dict.optSave}
        canSave={valid}
        submitting={submitting}
        onSave={save}
      />
    </Shell>
  );
}

// OptionClosedModal is 編輯已了結紀錄. Realized P&L is typed rather than derived,
// so it can hold what the broker booked. An assignment or exercise generated a
// stock trade, so its outcome is fixed and it can't be undone here.
export function OptionClosedModal({ record, ...base }: BaseProps & { record: OptionClosedTrade }) {
  const { dict, onClose } = base;
  const { submit, submitting, error } = useSubmit(base);
  const stockLinked = record.action === "ASSIGNED" || record.action === "EXERCISED";
  const initial: Outcome = stockLinked ? (record.action as Outcome) : record.action === "EXPIRED" ? "EXPIRED" : "CLOSED";
  const [outcome, setOutcome] = useState<Outcome>(initial);
  const [contracts, setContracts] = useState(String(record.contracts));
  const [date, setDate] = useState(record.date);
  const [pnl, setPnl] = useState(String(record.realizedPnL));
  const [tried, setTried] = useState(false);
  const [askDelete, setAskDelete] = useState(false);

  const qty = Number(contracts);
  const pnlN = Number(pnl.replace(/[,$+\s]/g, ""));
  const pnlOk = pnl.trim() !== "" && Number.isFinite(pnlN);
  const valid = qty > 0 && date !== "" && pnlOk;
  const label = contractLabel(record);
  const ids = { id: record.id, symbol: record.contractSymbol };

  function save() {
    if (!valid) {
      setTried(true);
      return;
    }
    submit(
      () => editOptionClosed({ ...ids, action: outcome, contracts: qty, date, realizedPnL: pnlN }),
      `${label} ${dict.optUpdatedToast}`,
    );
  }

  return (
    <Shell title={dict.optClosedEditTitle} width={420} onClose={onClose}>
      <div className="opt-modal-body">
        <span className="opt-contract">{label}</span>
        <div className="opt-field">
          {dict.optionAction}
          <div className="opt-chips">
            {stockLinked ? (
              <Chip on onClick={() => {}}>
                {record.action === "ASSIGNED" ? dict.optAssignedBtn : dict.optExercisedBtn}
              </Chip>
            ) : (
              <>
                <Chip on={outcome === "EXPIRED"} onClick={() => setOutcome("EXPIRED")}>
                  {dict.optExpiredBtn}
                </Chip>
                <Chip on={outcome === "CLOSED"} onClick={() => setOutcome("CLOSED")}>
                  {dict.optActClosed}
                </Chip>
              </>
            )}
          </div>
        </div>
        <div className="opt-grid">
          <Field
            label={dict.optionContracts}
            value={contracts}
            invalid={tried && !(qty > 0)}
            placeholder="1"
            onChange={(e) => setContracts(e.target.value)}
          />
          <Field
            type="date"
            label={dict.exitDate}
            value={date}
            invalid={tried && !date}
            onChange={(e) => setDate(e.target.value)}
          />
          <label className="opt-field opt-field-full">
            {dict.optRealized}
            <input
              className={`${tried && !pnlOk ? "invalid" : ""} ${pnlOk ? pnlClass(pnlN) : ""}`}
              value={pnl}
              placeholder="-320"
              onChange={(e) => setPnl(e.target.value)}
            />
            <small>{dict.optPnlHint}</small>
          </label>
        </div>
        {!stockLinked && (
          <div className="opt-undo">
            <span>{dict.optUndoNote}</span>
            <button
              type="button"
              disabled={submitting}
              onClick={() => submit(() => undoOptionClose(ids), `${label} ${dict.optUndoneToast}`)}
            >
              {dict.optUndoClose}
            </button>
          </div>
        )}
        {error && <div className="error-message">{error}</div>}
      </div>
      {askDelete ? (
        <DeleteFoot
          dict={dict}
          message={dict.optClosedDeleteAsk}
          disabled={submitting}
          onKeep={() => setAskDelete(false)}
          onDelete={() => submit(() => deleteOptionClosed(ids), `${label} ${dict.optDeletedToast}`)}
        />
      ) : (
        <Foot
          dict={dict}
          deleteLabel={dict.optDeleteRecord}
          onAskDelete={() => setAskDelete(true)}
          onClose={onClose}
          saveLabel={dict.optSaveChanges}
          canSave={valid}
          submitting={submitting}
          onSave={save}
        />
      )}
    </Shell>
  );
}
