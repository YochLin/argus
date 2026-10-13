import * as Dialog from "@radix-ui/react-dialog";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { AssetGroup, AssistantDraft, DraftPayload } from "../api";
import type { Assistant } from "../assistant";
import { useFlash } from "../flash";
import type { Dictionary } from "../i18n";
import { ConfirmDialog } from "./ConfirmDialog";
import { EXPENSE_CATEGORIES, INCOME_CATEGORIES, categoryLabel } from "./WealthCashView";
import { CURRENCIES, GROUPS, groupLabel, typeLabel } from "./WealthHomeView";

// The drafts column of the wealth assistant (Phase 28 B2, design view
// "drafts"/"applied"): what the AI proposed, each card with the server's
// verdict (ready / needs info / can't save), the edit drawer, and 確認寫入.

// DRAFT_TYPES mirrors internal/assets.DraftAssetTypes — what a draft can be.
const DRAFT_TYPES = ["deposit", "loan", "credit_card", "fund", "bond", "estate", "gold", "crypto", "pension", "other"];
const LOAN_TYPES = ["loan", "credit_card"];

// money shows an amount in its own currency — not fmtMoney, whose "NT$"
// follows the 顯示幣別 selector: a draft is what the user said, unconverted.
function money(v: number, currency?: string, negative?: boolean): string {
  const sym = currency === "TWD" ? "NT$" : currency === "USD" ? "US$" : currency ? `${currency} ` : "";
  return `${negative && v > 0 ? "−" : ""}${sym}${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 2 })}`;
}

// draftTitle is a card's (and a chip's) name.
export function draftTitle(dict: Dictionary, d: AssistantDraft): string {
  if (d.kind === "balance") return d.current?.name ?? "—";
  return d.payload.name || dict.asstPendingVal;
}

// draftChip is a chip's label: the name and the amount, as in the design.
export function draftChip(dict: Dictionary, d: AssistantDraft): string {
  const v = d.kind === "cashflow" ? d.payload.amount : d.payload.value;
  const cur = d.kind === "balance" ? d.current?.currency : d.payload.currency;
  const neg = d.kind === "asset" && LOAN_TYPES.includes(d.payload.type ?? "");
  return v == null ? draftTitle(dict, d) : `${draftTitle(dict, d)} ${money(v, cur, neg)}`;
}

function kindLabel(dict: Dictionary, d: AssistantDraft): string {
  return d.kind === "balance" ? dict.asstKindBalance : d.kind === "cashflow" ? dict.asstKindCash : dict.asstKindAsset;
}

const flowLabel = (dict: Dictionary, dir?: string) =>
  dir === "in" ? dict.wealthCashDirectionIn : dir === "out" ? dict.wealthCashDirectionOut : dir;

function statusLabel(dict: Dictionary, s: AssistantDraft["status"]): string {
  return s === "ok" ? dict.asstStOk : s === "pending" ? dict.asstStPending : dict.asstStError;
}

export function DraftsPane({
  dict,
  a,
  compact,
  onNavigate,
}: {
  dict: Dictionary;
  a: Assistant;
  compact: boolean;
  onNavigate: (path: string) => void;
}) {
  const flash = useFlash();
  const [editing, setEditing] = useState<AssistantDraft | null>(null);
  const [confirmClear, setConfirmClear] = useState(false);
  const list = useRef<HTMLDivElement>(null);

  // A chip pointed at a card: bring it into view.
  useEffect(() => {
    if (a.focusId === null) return;
    const t = setTimeout(() => {
      const el = list.current?.querySelector<HTMLElement>(`[data-draft="${a.focusId}"]`);
      if (el && list.current) list.current.scrollTo({ top: el.offsetTop - 14, behavior: "smooth" });
    }, 60);
    return () => clearTimeout(t);
  }, [a.focusId]);

  // After a write the result card sits at the top.
  useEffect(() => {
    if (a.applied && list.current) list.current.scrollTop = 0;
  }, [a.applied]);

  const ro = () => {
    flash(dict.asstRoToast, "error");
    return false;
  };
  const okN = a.drafts.filter((d) => d.status === "ok").length;
  const badN = a.drafts.length - okN;
  const commitLabel = !okN
    ? dict.asstCommitNone
    : badN
      ? dict.asstCommitNM.replace("%n", String(okN)).replace("%m", String(badN))
      : dict.asstCommitN.replace("%s", String(okN));

  return (
    <div className="asst-pane">
      <div className="asst-drafts-head">
        <div className="asst-drafts-head-row">
          <span className="asst-label">{dict.asstDrafts}</span>
          {a.drafts.length > 0 && <span className="asst-drafts-count">{a.drafts.length}</span>}
          <span className="asst-not-saved">{dict.asstNotSaved}</span>
        </div>
        <span className="asst-note">{dict.asstDraftNote}</span>
      </div>
      <div className="asst-drafts-list" ref={list}>
        {a.applied && (
          <div className="asst-applied" role="status">
            <span className="asst-applied-title">✓ {dict.asstAppliedTitle.replace("%s", String(a.applied.applied))}</span>
            <span className="asst-applied-body">
              {dict.asstAppliedBody}
              {a.drafts.length > 0 && dict.asstAppliedKept.replace("%s", String(a.drafts.length))}
            </span>
            <button className="asst-applied-link" onClick={() => onNavigate("/w/balance")}>
              {dict.asstSeeBal}
            </button>
          </div>
        )}
        {a.drafts.length === 0 && !a.applied && (
          <div className="asst-drafts-empty">
            <span>{dict.asstEmptyDraft}</span>
            <div className="asst-kinds">
              {dict.asstKinds.split("|").map((k) => (
                <span key={k}>{k}</span>
              ))}
            </div>
          </div>
        )}
        {a.drafts.map((d) => (
          <DraftCard
            key={d.id}
            dict={dict}
            d={d}
            isNew={a.newIds.includes(d.id)}
            focused={a.focusId === d.id}
            ro={!a.writable}
            onEdit={() => (a.writable ? setEditing(d) : ro())}
            onDelete={() => (a.writable ? a.deleteDraft(d.id) : ro())}
          />
        ))}
      </div>
      <div className={`asst-drafts-foot${compact ? " compact" : ""}`}>
        {badN > 0 && <span className="asst-foot-note">{dict.asstFootNote}</span>}
        <div className="asst-foot-row">
          <button
            className={`asst-clear${!a.writable || !a.drafts.length ? " off" : ""}`}
            onClick={() => (!a.writable ? ro() : a.drafts.length > 0 && setConfirmClear(true))}
          >
            {dict.asstClear}
          </button>
          <button
            className={`asst-commit${!okN ? " off" : !a.writable ? " ro" : ""}`}
            aria-disabled={!okN || !a.writable}
            onClick={() => (!a.writable ? ro() : okN > 0 && a.applyDrafts())}
          >
            {commitLabel}
          </button>
        </div>
      </div>
      {confirmClear && (
        <ConfirmDialog
          title={dict.asstClearQ}
          body={dict.asstClearBody.replace("%s", String(a.drafts.length))}
          okLabel={dict.asstClearOk}
          cancelLabel={dict.cancel}
          onCancel={() => setConfirmClear(false)}
          onConfirm={() => {
            setConfirmClear(false);
            a.clearDrafts();
          }}
        />
      )}
      {editing && (
        <DraftEditDrawer
          dict={dict}
          d={a.drafts.find((x) => x.id === editing.id) ?? editing}
          onClose={() => setEditing(null)}
          onSave={(payload) => a.saveDraft(editing.id, payload).then((ok) => ok && setEditing(null))}
        />
      )}
    </div>
  );
}

type Field = { label: string; value?: string; missing?: boolean };

function FieldGrid({ fields, dict, onEdit }: { fields: Field[]; dict: Dictionary; onEdit: () => void }) {
  return (
    <div className="asst-fields">
      {fields.map((f) => (
        <div key={f.label} className="asst-field">
          <span className="asst-field-k">{f.label}</span>
          {f.missing ? (
            <button className="asst-missing" onClick={onEdit}>
              ? {dict.asstPendingVal}
            </button>
          ) : (
            <span className="asst-field-v">{f.value ?? "—"}</span>
          )}
        </div>
      ))}
    </div>
  );
}

function DraftCard({
  dict,
  d,
  isNew,
  focused,
  ro,
  onEdit,
  onDelete,
}: {
  dict: Dictionary;
  d: AssistantDraft;
  isNew: boolean;
  focused: boolean;
  ro: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const miss = (f: string) => d.missing.includes(f);
  let body: ReactNode;
  if (d.kind === "balance") {
    const cur = d.current;
    const to = d.payload.value;
    const delta = to != null && cur?.value != null ? to - cur.value : null;
    // Owing less is the good direction for a loan, so its colours flip.
    const better = delta !== null && (LOAN_TYPES.includes(cur?.type ?? "") ? delta < 0 : delta > 0);
    body = (
      <div className="asst-upd">
        <span className="asst-upd-note">{dict.asstUpdNote}</span>
        <div className="asst-upd-row">
          <span className="asst-upd-col">
            <span className="asst-upd-k">{dict.asstFrom}</span>
            <span className="asst-upd-from">{cur?.value != null ? money(cur.value, cur.currency) : "—"}</span>
          </span>
          <span className="asst-upd-arrow">→</span>
          <span className="asst-upd-col">
            <span className="asst-upd-k">{dict.asstTo}</span>
            {to != null ? (
              <span className="asst-upd-to">{money(to, cur?.currency)}</span>
            ) : (
              <button className="asst-missing" onClick={onEdit}>
                ? {dict.asstPendingVal}
              </button>
            )}
          </span>
          {delta !== null && delta !== 0 && (
            <span className={`asst-upd-delta${better ? "" : " down"}`}>
              {delta > 0 ? "+" : "−"}
              {money(Math.abs(delta), cur?.currency)}
            </span>
          )}
        </div>
      </div>
    );
  } else if (d.kind === "cashflow") {
    // The design's three fields; currency and category only when missing.
    const p = d.payload;
    const fields: Field[] = [
      { label: dict.asstFFlow, value: flowLabel(dict, p.direction), missing: miss("direction") },
      { label: dict.asstFAmt, value: p.amount != null ? money(p.amount, p.currency) : undefined, missing: miss("amount") },
      { label: dict.wealthCashDayOfMonthLabel, value: p.day_of_month != null ? dict.asstDayN.replace("%s", String(p.day_of_month)) : "—" },
    ];
    if (miss("currency")) fields.push({ label: dict.asstFCur, missing: true });
    if (miss("category")) fields.push({ label: dict.wealthCashCategoryLabel, missing: true });
    body = <FieldGrid fields={fields} dict={dict} onEdit={onEdit} />;
  } else {
    const p = d.payload;
    const loan = LOAN_TYPES.includes(p.type ?? "");
    const fields: Field[] = [
      { label: dict.asstFType, value: p.type ? typeLabel(dict, p.type) : undefined, missing: miss("type") },
      { label: dict.asstFAmt, value: p.value != null ? money(p.value, p.currency, loan) : undefined, missing: miss("value") },
      { label: dict.asstFCur, value: p.currency, missing: miss("currency") },
    ];
    if (miss("group")) fields.push({ label: dict.wealthGroupLabel, missing: true });
    if (loan) {
      fields.push({ label: dict.asstFRate, value: p.rate_pct != null ? `${p.rate_pct}%` : "—" });
      fields.push({
        label: dict.asstFTerm,
        value: p.remaining_months != null ? dict.asstTermMonths.replace("%s", String(p.remaining_months)) : "—",
      });
    }
    if (p.date) fields.push({ label: dict.asstFDate, value: p.date });
    body = <FieldGrid fields={fields} dict={dict} onEdit={onEdit} />;
  }

  return (
    <div className={`asst-dcard ${d.status}${focused ? " focus" : ""}`} data-draft={d.id}>
      <div className="asst-dcard-head">
        <span className="asst-dcard-kind">{kindLabel(dict, d)}</span>
        <span className="asst-dcard-name">{draftTitle(dict, d)}</span>
        {isNew && <span className="asst-dcard-new">{dict.asstNewTag}</span>}
        <span className={`asst-st ${d.status}`}>{statusLabel(dict, d.status)}</span>
        <button className={`asst-dcard-btn${ro ? " ro" : ""}`} onClick={onEdit} aria-label={dict.asstEdit} title={dict.asstEdit}>
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6">
            <path d="M10.5 2.5l3 3L6 13H3v-3Z" />
          </svg>
        </button>
        <button className={`asst-dcard-btn del${ro ? " ro" : ""}`} onClick={onDelete} aria-label={dict.asstDel} title={dict.asstDel}>
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6">
            <path d="M3 4.5h10M6.5 4.5V3h3v1.5M4.5 4.5l.6 9h5.8l.6-9" />
          </svg>
        </button>
      </div>
      {body}
      {d.status === "pending" && <span className="asst-dcard-pending">{dict.asstPendingNote}</span>}
      {d.status === "error" && (
        <div className="asst-dcard-err">
          <span>
            ⚠ {d.failed ? dict.asstFailPre : ""}
            {d.error}
          </span>
          <button onClick={onEdit}>{dict.asstFix}</button>
        </div>
      )}
    </div>
  );
}

// num turns an input's text into a payload number ("" = not given).
function num(s: string): number | undefined {
  return s.trim() === "" || !Number.isFinite(Number(s)) ? undefined : Number(s);
}

const str = (v?: number) => (v == null ? "" : String(v));

const flowCategories = (dir: string): string[] => [...(dir === "in" ? INCOME_CATEGORIES : EXPENSE_CATEGORIES)];

// DraftEditDrawer is the design's 編輯草稿 drawer, with the §10.2 deviation:
// chips and number/date inputs instead of free text, so an edit can't type
// an amount the server reads as "missing". Fields it doesn't show (a fund's
// code, say) are carried over from the draft untouched.
function DraftEditDrawer({
  dict,
  d,
  onClose,
  onSave,
}: {
  dict: Dictionary;
  d: AssistantDraft;
  onClose: () => void;
  onSave: (payload: DraftPayload) => Promise<unknown>;
}) {
  const asset = d.kind === "asset" ? d.payload : null;
  const flow = d.kind === "cashflow" ? d.payload : null;
  const [f, setF] = useState({
    name: asset?.name ?? flow?.name ?? "",
    type: asset?.type ?? "",
    group: asset?.group ?? "",
    currency: asset?.currency ?? flow?.currency ?? "",
    value: str(d.kind === "cashflow" ? d.payload.amount : d.payload.value),
    date: d.kind === "cashflow" ? "" : (d.payload.date ?? ""),
    direction: flow?.direction ?? "",
    day: str(flow?.day_of_month),
    category: flow?.category ?? "",
    inst: asset ? ((LOAN_TYPES.includes(asset.type ?? "") ? asset.lender : asset.type === "deposit" ? asset.bank : asset.venue) ?? "") : "",
    accountNote: asset?.account_note ?? "",
    rate: str(asset?.rate_pct),
    principal: str(asset?.original_principal),
    months: str(asset?.remaining_months),
  });
  const [saving, setSaving] = useState(false);
  const set = (k: keyof typeof f) => (v: string) => setF((p) => ({ ...p, [k]: v }));
  const loan = LOAN_TYPES.includes(f.type);
  const valueKey = flow ? "amount" : "value";
  // A category belongs to one direction: switching drops one that doesn't fit.
  const pickDirection = (v: string) =>
    setF((p) => ({ ...p, direction: v, category: flowCategories(v).includes(p.category) ? p.category : "" }));

  function save() {
    let payload: DraftPayload;
    if (flow) {
      payload = {
        ...flow,
        direction: f.direction,
        name: f.name,
        amount: num(f.value),
        currency: f.currency,
        day_of_month: num(f.day),
        category: f.category,
      };
    } else if (!asset) {
      payload = { value: num(f.value), date: f.date || undefined };
    } else {
      const inst = f.inst.trim() || undefined;
      payload = {
        ...asset,
        name: f.name,
        type: f.type,
        group: f.group,
        currency: f.currency,
        value: num(f.value),
        date: f.date || undefined,
        lender: loan ? inst : undefined,
        bank: f.type === "deposit" ? inst : undefined,
        venue: !loan && f.type !== "deposit" ? inst : asset.venue,
        account_note: f.type === "deposit" ? f.accountNote.trim() || undefined : undefined,
        rate_pct: loan ? num(f.rate) : undefined,
        original_principal: loan ? num(f.principal) : undefined,
        remaining_months: loan ? num(f.months) : undefined,
      };
    }
    setSaving(true);
    onSave(payload).finally(() => setSaving(false));
  }

  // label marks a field the server listed as missing (the AI wasn't sure).
  const label = (text: string, key: string) => (
    <span className="wealth-drawer-field-label">
      {text}
      {d.missing.includes(key) && <span className="asst-ai-unsure">{dict.asstAiUnsure}</span>}
    </span>
  );
  const unsure = (key: string, empty: boolean) => (d.missing.includes(key) && empty ? " asst-input-unsure" : "");
  const chips = (opts: string[], cur: string, pick: (v: string) => void, text: (v: string) => string) => (
    <div className="wealth-chip-row">
      {opts.map((o) => (
        <button key={o} type="button" className={`wealth-chip${cur === o ? " active" : ""}`} onClick={() => pick(o)}>
          {text(o)}
        </button>
      ))}
    </div>
  );

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="wealth-drawer-overlay">
          <Dialog.Content className="wealth-drawer-panel asst-vars" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="wealth-drawer-header">
              <Dialog.Title className="asst-label">{dict.asstEditTitle}</Dialog.Title>
              <span className="wealth-drawer-step">{kindLabel(dict, d)}</span>
              <Dialog.Close className="modal-close" style={{ marginLeft: "auto" }} aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="wealth-drawer-body">
              <div className="asst-edit-note">{dict.asstEditNote}</div>
              {d.status === "error" && (
                <div className="asst-edit-err">
                  ⚠ {d.failed ? dict.asstFailPre : ""}
                  {d.error}
                </div>
              )}
              {flow ? (
                <>
                  <div className="wealth-drawer-field">
                    {label(dict.asstFFlow, "direction")}
                    {chips(["in", "out"], f.direction, pickDirection, (v) => flowLabel(dict, v) ?? v)}
                  </div>
                  <div className="wealth-drawer-field">
                    {label(dict.wealthCashNameLabel, "name")}
                    <input className={unsure("name", !f.name.trim())} value={f.name} onChange={(e) => set("name")(e.target.value)} />
                  </div>
                </>
              ) : asset ? (
                <>
                  <div className="wealth-drawer-field">
                    {label(dict.wealthName, "name")}
                    <input className={unsure("name", !f.name.trim())} value={f.name} onChange={(e) => set("name")(e.target.value)} />
                  </div>
                  <div className="wealth-drawer-field">
                    {label(dict.asstFType, "type")}
                    {chips(DRAFT_TYPES, f.type, set("type"), (t) => typeLabel(dict, t))}
                  </div>
                  <div className="wealth-drawer-field">
                    {label(dict.wealthGroupLabel, "group")}
                    {chips(GROUPS, f.group, set("group"), (g) => groupLabel(dict, g as AssetGroup))}
                  </div>
                </>
              ) : (
                <div className="wealth-drawer-field">
                  <span className="wealth-drawer-field-label">{dict.wealthName}</span>
                  <input value={d.current?.name ?? ""} readOnly className="asst-input-ro" />
                </div>
              )}
              <div className="wealth-drawer-field">
                {label(d.kind === "balance" ? dict.asstTo : dict.asstFAmt, valueKey)}
                <input
                  className={`mono${unsure(valueKey, f.value.trim() === "")}`}
                  type="number"
                  inputMode="decimal"
                  value={f.value}
                  onChange={(e) => set("value")(e.target.value)}
                  style={{ textAlign: "right" }}
                />
              </div>
              {(asset || flow) && (
                <div className="wealth-drawer-field">
                  {label(dict.asstFCur, "currency")}
                  {chips(CURRENCIES, f.currency, set("currency"), (c) => c)}
                </div>
              )}
              {flow ? (
                <>
                  <div className="wealth-drawer-field">
                    {label(dict.wealthCashDayOfMonthLabel, "day_of_month")}
                    <input className="mono" type="number" inputMode="numeric" min={1} max={31} value={f.day} onChange={(e) => set("day")(e.target.value)} />
                  </div>
                  <div className="wealth-drawer-field">
                    {label(dict.wealthCashCategoryLabel, "category")}
                    {chips(flowCategories(f.direction), f.category, set("category"), (c) => categoryLabel(dict, c))}
                  </div>
                </>
              ) : (
                <div className="wealth-drawer-field">
                  {label(dict.asstFDate, "date")}
                  <input className="mono" type="date" value={f.date} onChange={(e) => set("date")(e.target.value)} />
                </div>
              )}
              {asset && (
                <div className="wealth-drawer-field">
                  <span className="wealth-drawer-field-label">{loan ? dict.wealthLender : dict.wealthFInst}</span>
                  <input value={f.inst} onChange={(e) => set("inst")(e.target.value)} />
                </div>
              )}
              {asset && f.type === "deposit" && (
                <div className="wealth-drawer-field">
                  <span className="wealth-drawer-field-label">{dict.wealthAccountNote}</span>
                  <input value={f.accountNote} onChange={(e) => set("accountNote")(e.target.value)} />
                </div>
              )}
              {asset && loan && (
                <>
                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">{dict.wealthRatePct}</span>
                    <input className="mono" type="number" inputMode="decimal" placeholder="2.5" value={f.rate} onChange={(e) => set("rate")(e.target.value)} />
                  </div>
                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">{dict.wealthOriginalPrincipal}</span>
                    <input className="mono" type="number" inputMode="decimal" value={f.principal} onChange={(e) => set("principal")(e.target.value)} />
                  </div>
                  <div className="wealth-drawer-field">
                    <span className="wealth-drawer-field-label">{dict.wealthRemainingMonths}</span>
                    <input className="mono" type="number" inputMode="numeric" value={f.months} onChange={(e) => set("months")(e.target.value)} />
                  </div>
                </>
              )}
            </div>
            <div className="wealth-drawer-footer">
              <button className="wealth-drawer-cancel" style={{ marginLeft: "auto" }} onClick={onClose}>
                {dict.cancel}
              </button>
              <button className="btn-primary" disabled={saving} onClick={save}>
                {dict.asstSaveDraft}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
