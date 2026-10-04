import { useCallback, useEffect, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import {
  ApiError,
  archiveWealthAsset,
  deleteWealthAssetSnapshot,
  fetchWealthAssetHistory,
  fetchWealthFX,
  saveWealthAssetSnapshot,
  unarchiveWealthAsset,
  updateWealthAsset,
  type AssetGroup,
  type WealthAsset,
  type WealthSnapshotRow,
} from "../api";
import type { Dictionary } from "../i18n";
import { useFlash } from "../flash";
import { ConfirmDialog } from "./ConfirmDialog";
import { groupLabel, typeLabel } from "./WealthHomeView";

const GROUPS: AssetGroup[] = ["liquid", "growth", "income", "hard"];

interface Props {
  dict: Dictionary;
  asset: WealthAsset;
  writable: boolean;
  onClose: () => void;
  // Called after anything that changed server data, so the page behind
  // refetches; the drawer itself closes only on save/archive/restore.
  onChanged: () => void;
  onUnauthorized: (retry: () => void) => void;
}

// The server judges "today's record" by its own date, which only arrives with
// the history; until then the browser's date stands in (never sent anywhere).
function localToday(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// RowEditDrawer is the design's row editor (rowEdModel): an asset's identity
// fields, a form to log its value, its value history, and archive/restore.
// Side, type, currency and source are shown but never editable — see
// db.AssetEdit for why.
export function RowEditDrawer({ dict, asset, writable, onClose, onChanged, onUnauthorized }: Props) {
  const flash = useFlash();
  const archived = !!asset.archivedAt;
  const isAsset = asset.side === "asset";
  const canEdit = writable && !archived;

  const [hist, setHist] = useState<{ today: string; snapshots: WealthSnapshotRow[] } | null>(null);
  const [rate, setRate] = useState<number | null>(asset.currency === "TWD" ? 1 : null); // TWD per 1 unit
  const [name, setName] = useState(asset.name);
  const [venue, setVenue] = useState(asset.venue);
  const [group, setGroup] = useState<AssetGroup>(asset.assetGroup);
  const [nameErr, setNameErr] = useState(false);
  const [amt, setAmt] = useState("");
  const [date, setDate] = useState("");
  const [fixing, setFixing] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmArchive, setConfirmArchive] = useState(false);

  const loadHist = useCallback(() => {
    fetchWealthAssetHistory(asset.id)
      .then(setHist)
      .catch(() => flash(dict.error, "error"));
  }, [asset.id, dict.error, flash]);

  useEffect(loadHist, [loadHist]);

  useEffect(() => {
    if (asset.currency === "TWD") return;
    fetchWealthFX()
      .then((r) => setRate(r.rates[asset.currency] ?? null))
      .catch(() => setRate(null)); // the ≈NT$ preview is a convenience; it just disappears
  }, [asset.currency]);

  const today = hist?.today ?? localToday();
  const snapshots = hist?.snapshots ?? [];
  const latest = snapshots[0];
  const cur = asset.currency;
  const fmtCur = (v: number) =>
    `${cur === "TWD" ? "NT$ " : `${cur} `}${v.toLocaleString("en-US", { maximumFractionDigits: cur === "TWD" || cur === "JPY" ? 0 : 2 })}`;

  async function run(op: () => Promise<unknown>, after: () => void) {
    setBusy(true);
    try {
      await op();
      after();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onUnauthorized(() => void run(op, after));
      } else if (e instanceof ApiError && e.status === 409) {
        setErr(dict.wealthEdErrLocked);
      } else {
        flash(e instanceof ApiError ? e.message : dict.error, "error");
      }
    } finally {
      setBusy(false);
    }
  }

  function resetLog() {
    setAmt("");
    setDate("");
    setFixing(false);
    setErr(null);
  }

  function record() {
    const n = Number(amt);
    if (amt.trim() === "" || !Number.isFinite(n)) {
      setErr(dict.wealthEdErrAmount);
      return;
    }
    const d = date || today;
    if (d > today) {
      setErr(dict.wealthEdErrFuture);
      return;
    }
    if (d !== today && snapshots.some((s) => s.date === d)) {
      setErr(dict.wealthEdErrLocked);
      return;
    }
    const wasFixing = fixing;
    void run(
      () => saveWealthAssetSnapshot(asset.id, n, d),
      () => {
        flash(wasFixing ? dict.wealthEdFlashFixed : dict.wealthEdFlashLogged);
        resetLog();
        loadHist();
        onChanged();
      },
    );
  }

  function startFix(row: WealthSnapshotRow) {
    setFixing(true);
    setAmt(String(row.value));
    setDate(row.date);
    setErr(null);
  }

  function removeRecord(row: WealthSnapshotRow) {
    void run(
      () => deleteWealthAssetSnapshot(asset.id, row.date),
      () => {
        flash(dict.wealthEdFlashDeleted);
        if (fixing && date === row.date) resetLog();
        loadHist();
        onChanged();
      },
    );
  }

  function save() {
    if (name.trim() === "") {
      setNameErr(true);
      return;
    }
    void run(
      () => updateWealthAsset({ assetId: asset.id, name: name.trim(), assetGroup: group, venue: venue.trim() }),
      () => {
        flash(dict.wealthEdFlashSaved);
        onChanged();
        onClose();
      },
    );
  }

  const preview = rate != null && cur !== "TWD" && amt.trim() !== "" && Number.isFinite(Number(amt));

  return (
    <>
      <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
        <Dialog.Portal>
          <Dialog.Overlay className="wealth-drawer-overlay row-ed-overlay">
            <Dialog.Content className="wealth-drawer-panel row-ed" aria-describedby={undefined} onOpenAutoFocus={(e) => e.preventDefault()}>
              <div className="wealth-drawer-header">
                <Dialog.Title className="row-ed-title">{archived ? dict.wealthEdTitleArchived : dict.wealthEdTitle}</Dialog.Title>
                <Dialog.Close className="modal-close" style={{ marginLeft: "auto" }} aria-label="close">
                  ×
                </Dialog.Close>
              </div>

              <div className="wealth-drawer-body">
                <div className="row-ed-summary">
                  <span className="row-ed-name">{asset.name}</span>
                  <span className="row-ed-current">
                    <span className="row-ed-current-val">{latest ? fmtCur(latest.value) : "—"}</span>
                    <span className="row-ed-current-date">
                      {latest ? dict.wealthEdLastRecord.replace("%s", latest.date) : hist ? dict.wealthEdNoRecord : ""}
                    </span>
                  </span>
                </div>

                {!writable && (
                  <div className="row-ed-note">
                    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" style={{ flexShrink: 0 }}>
                      <rect x="3" y="7" width="10" height="7" rx="1.5" />
                      <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
                    </svg>
                    {dict.wealthEdRoNote}
                  </div>
                )}
                {archived && (
                  <div className="row-ed-note row-ed-note--archived">
                    {dict.wealthEdArchNote.replace("%s", (asset.archivedAt ?? "").slice(0, 10))}
                  </div>
                )}

                <div className="row-ed-locked-wrap">
                  <div className="row-ed-locked">
                    {[
                      [dict.wealthEdLockSide, `${isAsset ? dict.wealthSideAsset : dict.wealthSideLiability} · ${typeLabel(dict, asset.type)}`],
                      [dict.wealthEdLockCurrency, cur],
                      [dict.wealthEdLockSource, srcName(dict, asset.source)],
                    ].map(([k, v]) => (
                      <div key={k} className="row-ed-locked-cell">
                        <span className="row-ed-locked-k">{k}</span>
                        <span className="row-ed-locked-v">{v}</span>
                      </div>
                    ))}
                  </div>
                  <span className="row-ed-hint">{dict.wealthEdLockNote}</span>
                </div>

                {canEdit && (
                  <div className="row-ed-form">
                    <label className="row-ed-field">
                      {dict.wealthName}
                      <input
                        value={name}
                        onChange={(e) => {
                          setName(e.target.value);
                          setNameErr(false);
                        }}
                      />
                      {nameErr && <span className="row-ed-err">{dict.wealthEdErrName}</span>}
                    </label>
                    <label className="row-ed-field">
                      {dict.wealthVenue}
                      <input
                        value={venue}
                        onChange={(e) => setVenue(e.target.value)}
                        placeholder={isAsset ? dict.wealthEdInstPhAsset : dict.wealthEdInstPhLiab}
                      />
                    </label>
                    {isAsset && (
                      <div className="row-ed-field">
                        {dict.wealthGroupLabel}
                        <div className="wealth-chip-row">
                          {GROUPS.map((g) => (
                            <button key={g} type="button" className={`wealth-chip${group === g ? " active" : ""}`} onClick={() => setGroup(g)}>
                              {groupLabel(dict, g)}
                            </button>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}

                {canEdit && (
                  <div className="row-ed-log">
                    <div className="row-ed-log-head">
                      <span className="row-ed-section">{fixing ? dict.wealthEdFixTitle : dict.wealthEdLogTitle}</span>
                      {fixing && (
                        <button type="button" className="row-ed-linkbtn" onClick={resetLog}>
                          {dict.wealthEdCancelFix}
                        </button>
                      )}
                    </div>
                    <div className="row-ed-log-inputs">
                      <div className="row-ed-amount">
                        <span className="row-ed-amount-cur">{cur}</span>
                        <input
                          aria-label={dict.wealthValue}
                          value={amt}
                          inputMode="decimal"
                          placeholder={latest ? String(latest.value) : ""}
                          onChange={(e) => {
                            setAmt(e.target.value.replace(/[^0-9.]/g, ""));
                            setErr(null);
                          }}
                        />
                      </div>
                      <input
                        className="row-ed-date"
                        type="date"
                        aria-label="date"
                        value={date || today}
                        max={today}
                        onChange={(e) => {
                          setDate(e.target.value);
                          setErr(null);
                        }}
                      />
                    </div>
                    <div className="row-ed-log-hint">
                      <span className="row-ed-hint">{cur === "TWD" ? dict.wealthEdHintTwd : dict.wealthEdHintFx.replace("%s", cur)}</span>
                      {preview && <span className="row-ed-conv">{`≈ NT$ ${Math.round(Number(amt) * (rate as number)).toLocaleString("en-US")}`}</span>}
                    </div>
                    {err && <span className="row-ed-err">{err}</span>}
                    <button type="button" className="row-ed-record" disabled={busy} onClick={record}>
                      {fixing ? dict.wealthEdUpdate : dict.wealthEdAdd}
                    </button>
                  </div>
                )}

                <div className="row-ed-hist">
                  <span className="row-ed-section">{dict.wealthEdHistTitle}</span>
                  <div className="row-ed-hist-list">
                    {hist && snapshots.length === 0 && <span className="row-ed-hint">{dict.wealthEdHistEmpty}</span>}
                    {snapshots.map((s, i) => {
                      const mine = s.date === today;
                      return (
                        <div key={s.date} className={`row-ed-hist-row${i === 0 ? " current" : ""}${fixing && date === s.date ? " fixing" : ""}`}>
                          <span className="row-ed-hist-date">{s.date}</span>
                          <span className="row-ed-hist-val">{fmtCur(s.value)}</span>
                          <span className={`row-ed-tag${mine ? " mine" : ""}`}>{mine ? dict.wealthEdTagToday : dict.wealthEdTagHistory}</span>
                          <span className="row-ed-hist-actions">
                            {mine && canEdit && (
                              <>
                                <button type="button" className="row-ed-fix" onClick={() => startFix(s)}>
                                  {dict.wealthEdFix}
                                </button>
                                <button type="button" className="row-ed-del" disabled={busy} onClick={() => removeRecord(s)}>
                                  {dict.wealthEdDel}
                                </button>
                              </>
                            )}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                  <span className="row-ed-hint row-ed-hint--loose">{dict.wealthEdHistNote}</span>
                </div>
              </div>

              <div className="wealth-drawer-footer row-ed-footer">
                {canEdit && (
                  <button type="button" className="row-ed-archive" onClick={() => setConfirmArchive(true)}>
                    {dict.wealthEdArchive}
                  </button>
                )}
                {archived && writable && (
                  <button
                    type="button"
                    className="row-ed-restore"
                    disabled={busy}
                    onClick={() =>
                      void run(
                        () => unarchiveWealthAsset(asset.id),
                        () => {
                          flash(dict.wealthFlashRestored.replace("%s", asset.name));
                          onChanged();
                          onClose();
                        },
                      )
                    }
                  >
                    {dict.wealthRestore}
                  </button>
                )}
                <button type="button" className="wealth-drawer-cancel" style={{ marginLeft: "auto" }} onClick={onClose}>
                  {dict.wealthEdClose}
                </button>
                {canEdit && (
                  <button type="button" className="btn-primary" disabled={busy} onClick={save}>
                    {dict.wealthEdSave}
                  </button>
                )}
              </div>
            </Dialog.Content>
          </Dialog.Overlay>
        </Dialog.Portal>
      </Dialog.Root>

      {confirmArchive && (
        <ConfirmDialog
          title={dict.wealthArchiveTitle.replace("%s", asset.name)}
          body={dict.wealthArchiveBody}
          okLabel={dict.wealthArchive}
          cancelLabel={dict.cancel}
          onCancel={() => setConfirmArchive(false)}
          onConfirm={() => {
            setConfirmArchive(false);
            void run(
              () => archiveWealthAsset(asset.id),
              () => {
                flash(dict.wealthFlashArchived.replace("%s", asset.name));
                onChanged();
                onClose();
              },
            );
          }}
        />
      )}
    </>
  );
}

function srcName(dict: Dictionary, source: string): string {
  if (source === "import") return dict.wealthSrcImport;
  if (source === "sync") return dict.wealthSrcSync;
  return dict.wealthSrcManual;
}
