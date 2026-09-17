import { useEffect, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  ApiError,
  archiveWealthAsset,
  createWealthAsset,
  fetchWealthAssets,
  fetchWealthHome,
  saveWealthAssetSnapshot,
  type AllocationModel,
  type AssetGroup,
  type AssetSide,
  type WealthAsset,
  type WealthHome,
} from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
}

const MODELS: AllocationModel[] = ["conserv", "balanced", "growth"];
const GROUPS: AssetGroup[] = ["liquid", "growth", "income", "hard"];
// CURRENCIES mirrors the design spec's display-currency menu (§8.1) — the
// backend only converts USD/TWD so far (wealth_home.go), the rest are
// accepted as an asset's native currency and simply won't convert into the
// net-worth total yet (rendered "—" if that ever matters — see
// wealth_home.go's "degrade the whole metric" rule).
const CURRENCIES = ["TWD", "USD", "JPY", "EUR", "CNY"];

// Kind mirrors assets.CategoryOf's recognized type strings (§8.5) plus
// "loan" (liability) and "other" (legacy catch-all) — every kind but
// deposit/loan submits straight through as `type` with no extra fields,
// same as "other" always did, since none of these have a detail table yet
// (insurance_details/fund_details/bond_details are PR8/10/11, still ahead).
type Kind = "deposit" | "loan" | "insurance" | "fund" | "bond" | "estate" | "gold" | "crypto" | "pension" | "other";

const KINDS: Kind[] = ["deposit", "loan", "insurance", "fund", "bond", "estate", "gold", "crypto", "pension", "other"];

// KIND_DEFAULT_GROUP is changeKind's asset_group suggestion per kind — the
// group dropdown stays editable afterward, this just saves a click for the
// common case (§8.1: asset_group is still required, independent of the
// finer /w/alloc-only category split).
const KIND_DEFAULT_GROUP: Partial<Record<Kind, AssetGroup>> = {
  deposit: "liquid",
  loan: "hard",
  insurance: "income",
  fund: "growth",
  bond: "income",
  estate: "hard",
  gold: "hard",
  crypto: "growth",
  pension: "hard",
};

function kindLabel(dict: Dictionary, k: Kind): string {
  switch (k) {
    case "deposit":
      return dict.wealthKindDeposit;
    case "loan":
      return dict.wealthKindLoan;
    case "insurance":
      return dict.wealthKindInsurance;
    case "fund":
      return dict.wealthKindFund;
    case "bond":
      return dict.wealthKindBond;
    case "estate":
      return dict.wealthKindEstate;
    case "gold":
      return dict.wealthKindGold;
    case "crypto":
      return dict.wealthKindCrypto;
    case "pension":
      return dict.wealthKindPension;
    case "other":
      return dict.wealthKindOther;
  }
}

function modelLabel(dict: Dictionary, m: AllocationModel): string {
  return m === "conserv" ? dict.wealthModelConserv : m === "growth" ? dict.wealthModelGrowth : dict.wealthModelBalanced;
}

export function groupLabel(dict: Dictionary, g: AssetGroup): string {
  switch (g) {
    case "liquid":
      return dict.wealthGroupLiquid;
    case "growth":
      return dict.wealthGroupGrowth;
    case "income":
      return dict.wealthGroupIncome;
    case "hard":
      return dict.wealthGroupHard;
  }
}

export function fmtMoney(v: number, currency: string): string {
  return `${currency}${v.toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

// groupColorClass maps the four asset_group buckets onto the app's generic
// four-way series palette (--s1..--s4, theme-invariant) for the allocation
// bar/legend/dots — matches the design mock's per-group dot/segment color,
// which isn't itself specified beyond "give each category its own color."
const GROUP_COLOR_CLASS: Record<AssetGroup, string> = {
  liquid: "s1",
  growth: "s2",
  income: "s3",
  hard: "s4",
};

export function groupColorClass(g: AssetGroup): string {
  return GROUP_COLOR_CLASS[g];
}

export function WealthHomeView({ dict, writable, onUnauthorized }: Props) {
  const [model, setModel] = useState<AllocationModel>("balanced");
  const [home, setHome] = useState<WealthHome | null>(null);
  const [assets, setAssets] = useState<WealthAsset[] | null>(null);
  const [error, setError] = useState(false);
  const [refreshSignal, setRefreshSignal] = useState(0);
  const [showAdd, setShowAdd] = useState(false);
  const [editing, setEditing] = useState<WealthAsset | null>(null);

  useEffect(() => {
    setError(false);
    fetchWealthHome(model)
      .then(setHome)
      .catch(() => setError(true));
  }, [model, refreshSignal]);

  useEffect(() => {
    fetchWealthAssets()
      .then((r) => setAssets(r.assets))
      .catch(() => setError(true));
  }, [refreshSignal]);

  function refresh() {
    setRefreshSignal((n) => n + 1);
  }

  async function handleArchive(a: WealthAsset) {
    if (!window.confirm(dict.wealthArchiveConfirm)) return;
    try {
      await archiveWealthAsset(a.id);
      refresh();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => handleArchive(a));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  if (error) {
    return <div className="error-message">{dict.error}</div>;
  }

  const currency = "NT$"; // display currency fixed to TWD for now (settings.wealth.display_currency is a later refinement)

  // Top 4 rows by |deviation|, worst-first — the design mock's "off-target"
  // card. Pure frontend derivation off the allocation rows the backend
  // already sends; no separate endpoint needed.
  const offTarget = home
    ? [...home.allocation].sort((a, b) => Math.abs(b.deviationPt) - Math.abs(a.deviationPt)).slice(0, 4)
    : [];

  return (
    <>
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 12, flexWrap: "wrap" }}>
        <div className="eyebrow">{dict.navWealth}</div>
        <div className="topbar-tabs" role="group" aria-label="allocation model" style={{ marginLeft: "auto" }}>
          {MODELS.map((m) => (
            <button key={m} className={`topbar-tab${model === m ? " active" : ""}`} onClick={() => setModel(m)}>
              {modelLabel(dict, m)}
            </button>
          ))}
        </div>
      </div>

      <div className="card card--glow">
        <div className="eyebrow">{dict.wealthNetWorth}</div>
        <div className="wealth-hero-value">{home?.netWorth != null ? fmtMoney(home.netWorth, currency) : "—"}</div>
        <div className="wealth-hero-stats">
          <span>
            {dict.wealthYTD}{" "}
            <span className={home?.ytdPct != null ? (home.ytdPct >= 0 ? "profit" : "loss") : ""}>
              {home?.ytdPct != null ? `${home.ytdPct >= 0 ? "+" : ""}${home.ytdPct.toFixed(1)}%` : "—"}
            </span>
          </span>
          <span>
            {dict.wealthMoM}{" "}
            <span className={home?.momPct != null ? (home.momPct >= 0 ? "profit" : "loss") : ""}>
              {home?.momPct != null ? `${home.momPct >= 0 ? "+" : ""}${home.momPct.toFixed(1)}%` : "—"}
            </span>
          </span>
          <span>{dict.wealthTotalAssets} {home?.totalAssets != null ? fmtMoney(home.totalAssets, currency) : "—"}</span>
          <span className="loss">
            {dict.wealthTotalLiabilities}{" "}
            {home?.totalLiabilities != null ? fmtMoney(home.totalLiabilities, currency) : "—"}
          </span>
        </div>
        {home && home.allocation.length > 0 && (
          <>
            <div className="wealth-alloc-bar">
              {home.allocation.map((row) => (
                <div
                  key={row.group}
                  className={`wealth-bar-seg ${groupColorClass(row.group)}`}
                  style={{ width: `${Math.max(0, row.currentPct)}%` }}
                />
              ))}
            </div>
            <div className="wealth-alloc-legend">
              {home.allocation.map((row) => (
                <span key={row.group} className="wealth-alloc-legend-item">
                  <span className={`wealth-dot ${groupColorClass(row.group)}`} />
                  {groupLabel(dict, row.group)} <span className="mono">{fmtMoney(row.marketValue, currency)}</span>
                </span>
              ))}
            </div>
          </>
        )}
      </div>

      {offTarget.length > 0 && (
        <div className="card">
          <div className="eyebrow">{dict.wealthOffTargetTitle}</div>
          <div className="wealth-col-stack" style={{ marginTop: 12 }}>
            {offTarget.map((row) => (
              <div key={row.group} className="wealth-item-row">
                <span className={`wealth-dot ${groupColorClass(row.group)}`} />
                {groupLabel(dict, row.group)}
                <span
                  className={`wealth-item-row-value ${row.deviationPt > 0 ? "profit" : row.deviationPt < 0 ? "loss" : ""}`}
                >
                  {row.deviationPt > 0 ? "+" : ""}
                  {row.deviationPt.toFixed(1)}pt
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="card">
        <div className="eyebrow">{dict.wealthAllocation}</div>
        {home && home.allocation.length > 0 ? (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthGroupLabel}</th>
                <th>{dict.wealthCurrentPct}</th>
                <th>{dict.wealthTargetPct}</th>
                <th>{dict.wealthDeviation}</th>
                <th>{dict.wealthMarketValue}</th>
              </tr>
            </thead>
            <tbody>
              {home.allocation.map((row) => (
                <tr key={row.group}>
                  <td>
                    <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span className={`wealth-dot ${groupColorClass(row.group)}`} />
                      {groupLabel(dict, row.group)}
                    </span>
                  </td>
                  <td>{row.currentPct.toFixed(1)}%</td>
                  <td>{row.targetPct.toFixed(1)}%</td>
                  <td className={row.deviationPt > 0 ? "profit" : row.deviationPt < 0 ? "loss" : ""}>
                    {row.deviationPt > 0 ? "+" : ""}
                    {row.deviationPt.toFixed(1)}pt
                  </td>
                  <td>{fmtMoney(row.marketValue, currency)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-message">{dict.noData}</div>
        )}
      </div>

      <div className="card">
        <div className="modal-header" style={{ border: "none", padding: 0, marginBottom: 12 }}>
          <div className="eyebrow">{dict.wealthAssetsLabel}</div>
          {writable && (
            <button className="btn-tint" onClick={() => setShowAdd(true)}>
              {dict.wealthAddAsset}
            </button>
          )}
        </div>
        {!assets ? (
          <div className="loading">{dict.loading}</div>
        ) : assets.length === 0 ? (
          <div className="empty-message">{dict.wealthEmpty}</div>
        ) : (
          <table className="mono">
            <thead>
              <tr>
                <th>{dict.wealthName}</th>
                <th>{dict.wealthGroupLabel}</th>
                <th>{dict.wealthVenue}</th>
                <th>{dict.wealthValue}</th>
                {writable && <th />}
              </tr>
            </thead>
            <tbody>
              {assets.map((a) => (
                <tr key={a.id}>
                  <td>{a.name}</td>
                  <td>{groupLabel(dict, a.assetGroup)}</td>
                  <td>{a.venue || dict.wealthVenueUnset}</td>
                  <td className={a.side === "liability" ? "loss" : ""}>
                    {a.value != null ? fmtMoney(a.value, a.currency === "TWD" ? currency : a.currency) : "—"}
                  </td>
                  {writable && (
                    <td className="row-actions">
                      <button onClick={() => setEditing(a)}>{dict.wealthEditValueTitle}</button>
                      <button onClick={() => handleArchive(a)}>{dict.wealthArchive}</button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showAdd && (
        <AddAssetModal
          dict={dict}
          onClose={() => setShowAdd(false)}
          onSuccess={() => {
            setShowAdd(false);
            refresh();
          }}
          onUnauthorized={onUnauthorized}
        />
      )}
      {editing && (
        <EditValueModal
          dict={dict}
          asset={editing}
          onClose={() => setEditing(null)}
          onSuccess={() => {
            setEditing(null);
            refresh();
          }}
          onUnauthorized={onUnauthorized}
        />
      )}
    </>
  );
}

function AddAssetModal({
  dict,
  onClose,
  onSuccess,
  onUnauthorized,
}: {
  dict: Dictionary;
  onClose: () => void;
  onSuccess: () => void;
  onUnauthorized: (retry: () => void) => void;
}) {
  const [kind, setKind] = useState<Kind>("deposit");
  const [side, setSide] = useState<AssetSide>("asset");
  const [name, setName] = useState("");
  const [group, setGroup] = useState<AssetGroup>("liquid");
  const [venue, setVenue] = useState("");
  const [currency, setCurrency] = useState("TWD");
  const [initialValue, setInitialValue] = useState("");
  const [bank, setBank] = useState("");
  const [accountNote, setAccountNote] = useState("");
  const [lender, setLender] = useState("");
  const [ratePct, setRatePct] = useState("");
  const [originalPrincipal, setOriginalPrincipal] = useState("");
  const [remainingMonths, setRemainingMonths] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function changeKind(next: Kind) {
    setKind(next);
    setSide(next === "loan" ? "liability" : "asset");
    const defaultGroup = KIND_DEFAULT_GROUP[next];
    if (defaultGroup) {
      setGroup(defaultGroup);
    }
  }

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      await createWealthAsset({
        side,
        type: kind,
        name: name.trim(),
        assetGroup: group,
        venue: venue.trim() || undefined,
        currency,
        initialValue: Number(initialValue),
        deposit: kind === "deposit" ? { bank: bank.trim() || undefined, accountNote: accountNote.trim() || undefined } : undefined,
        loan:
          kind === "loan"
            ? {
                lender: lender.trim() || undefined,
                ratePct: ratePct ? Number(ratePct) : undefined,
                originalPrincipal: originalPrincipal ? Number(originalPrincipal) : undefined,
                remainingMonths: remainingMonths ? Number(remainingMonths) : undefined,
              }
            : undefined,
      });
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

  const canSubmit = name.trim() !== "" && Number(initialValue) !== 0 && !Number.isNaN(Number(initialValue));

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-backdrop">
          <Dialog.Content className="modal" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="modal-header">
              <Dialog.Title className="eyebrow">{dict.wealthAddTitle}</Dialog.Title>
              <Dialog.Close className="modal-close" aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="modal-body">
              <div className="topbar-tabs" role="group" aria-label="asset kind" style={{ flexWrap: "wrap" }}>
                {KINDS.map((k) => (
                  <button
                    key={k}
                    className={`topbar-tab${kind === k ? " active" : ""}`}
                    onClick={() => changeKind(k)}
                  >
                    {kindLabel(dict, k)}
                  </button>
                ))}
              </div>
              <label className="form-field">
                <span>{dict.wealthName}</span>
                <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
              </label>
              <label className="form-field">
                <span>{dict.wealthSide}</span>
                <select value={side} onChange={(e) => setSide(e.target.value as AssetSide)}>
                  <option value="asset">{dict.wealthSideAsset}</option>
                  <option value="liability">{dict.wealthSideLiability}</option>
                </select>
              </label>
              <label className="form-field">
                <span>{dict.wealthGroupLabel}</span>
                <select value={group} onChange={(e) => setGroup(e.target.value as AssetGroup)}>
                  {GROUPS.map((g) => (
                    <option key={g} value={g}>
                      {groupLabel(dict, g)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="form-field">
                <span>{dict.wealthVenue}</span>
                <input value={venue} onChange={(e) => setVenue(e.target.value)} />
              </label>
              <label className="form-field">
                <span>{dict.wealthCurrency}</span>
                <select value={currency} onChange={(e) => setCurrency(e.target.value)}>
                  {CURRENCIES.map((c) => (
                    <option key={c} value={c}>
                      {c}
                    </option>
                  ))}
                </select>
              </label>
              <label className="form-field">
                <span>{dict.wealthInitialValue}</span>
                <input className="mono" type="number" value={initialValue} onChange={(e) => setInitialValue(e.target.value)} />
              </label>
              {kind === "deposit" && (
                <>
                  <label className="form-field">
                    <span>{dict.wealthBank}</span>
                    <input value={bank} onChange={(e) => setBank(e.target.value)} />
                  </label>
                  <label className="form-field">
                    <span>{dict.wealthAccountNote}</span>
                    <input value={accountNote} onChange={(e) => setAccountNote(e.target.value)} />
                  </label>
                </>
              )}
              {kind === "loan" && (
                <>
                  <label className="form-field">
                    <span>{dict.wealthLender}</span>
                    <input value={lender} onChange={(e) => setLender(e.target.value)} />
                  </label>
                  <label className="form-field">
                    <span>{dict.wealthRatePct}</span>
                    <input className="mono" type="number" value={ratePct} onChange={(e) => setRatePct(e.target.value)} />
                  </label>
                  <label className="form-field">
                    <span>{dict.wealthOriginalPrincipal}</span>
                    <input
                      className="mono"
                      type="number"
                      value={originalPrincipal}
                      onChange={(e) => setOriginalPrincipal(e.target.value)}
                    />
                  </label>
                  <label className="form-field">
                    <span>{dict.wealthRemainingMonths}</span>
                    <input
                      className="mono"
                      type="number"
                      value={remainingMonths}
                      onChange={(e) => setRemainingMonths(e.target.value)}
                    />
                  </label>
                </>
              )}
              {error && <div className="error-message">{error}</div>}
              <div className="modal-actions">
                <button className="btn-primary" disabled={!canSubmit || submitting} onClick={submit}>
                  {dict.wealthAddAsset}
                </button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function EditValueModal({
  dict,
  asset,
  onClose,
  onSuccess,
  onUnauthorized,
}: {
  dict: Dictionary;
  asset: WealthAsset;
  onClose: () => void;
  onSuccess: () => void;
  onUnauthorized: (retry: () => void) => void;
}) {
  const [value, setValue] = useState(asset.value != null ? String(asset.value) : "");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      await saveWealthAssetSnapshot(asset.id, Number(value));
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

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-backdrop">
          <Dialog.Content className="modal" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
            <div className="modal-header">
              <Dialog.Title className="eyebrow">
                {dict.wealthEditValueTitle} — {asset.name}
              </Dialog.Title>
              <Dialog.Close className="modal-close" aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <div className="modal-body">
              <label className="form-field">
                <span>{dict.wealthValue}</span>
                <input className="mono" type="number" value={value} onChange={(e) => setValue(e.target.value)} autoFocus />
              </label>
              {error && <div className="error-message">{error}</div>}
              <div className="modal-actions">
                <button
                  className="btn-primary"
                  disabled={submitting || value.trim() === "" || Number.isNaN(Number(value))}
                  onClick={submit}
                >
                  {dict.wealthEditValueTitle}
                </button>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
