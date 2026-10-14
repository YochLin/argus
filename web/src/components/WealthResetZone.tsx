import * as Dialog from "@radix-ui/react-dialog";
import { useEffect, useRef, useState } from "react";
import { ApiError, fetchWealthReset, resetWealth, type WealthResetInfo } from "../api";
import { useAssistant } from "../assistant";
import { useFlash } from "../flash";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  onUnauthorized: (retry: () => void) => void;
}

// Phase 28 D: the settings page's danger zone, shown only in the wealth
// account (Argus Trading WebUI.dc.html's resetModel). The dialog unlocks
// 永久重設 only once its checkbox is ticked; Cancel takes the initial focus,
// so Enter on a freshly opened dialog cancels. The design's 匯出備份 button
// isn't here: the server backs the whole database up before it deletes
// anything, and the dialog says where.
export function WealthResetZone({ dict, onUnauthorized }: Props) {
  const flash = useFlash();
  const assistant = useAssistant();
  const [info, setInfo] = useState<WealthResetInfo | null>(null);
  const [open, setOpen] = useState(false);
  const [ack, setAck] = useState(false);
  const [busy, setBusy] = useState(false);
  const cancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    fetchWealthReset()
      .then(setInfo)
      .catch(() => {});
  }, []);

  function close() {
    if (busy) return;
    setOpen(false);
    setAck(false);
  }

  function confirm() {
    if (!ack || busy) return;
    setBusy(true);
    resetWealth()
      .then((r) => {
        setInfo((i) => ({ ...i, lastResetAt: r.lastResetAt }));
        setOpen(false);
        setAck(false);
        flash(dict.rstDone);
        // The server emptied the conversation and drafts too.
        assistant?.forget();
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) {
          onUnauthorized(confirm);
        } else {
          flash(e instanceof ApiError ? e.message : dict.error, "error");
        }
      })
      .finally(() => setBusy(false));
  }

  const wipe = [dict.rstWipe1, dict.rstWipe2, dict.rstWipe3, dict.rstWipe4, dict.rstWipe5];
  const keep = [dict.rstKeep1, dict.rstKeep2, dict.rstKeep3];

  return (
    // asst-vars: the design's loss tint lives with the assistant's tokens.
    <div className="rst-zone asst-vars">
      <div className="rst-zone-head">
        <span className="rst-zone-label">{dict.rstZone}</span>
        <span className="rst-zone-note">{dict.rstZoneNote}</span>
      </div>
      <div className="rst-zone-row">
        <div className="rst-zone-copy">
          <span className="rst-zone-key">{dict.rstKey}</span>
          <span className="rst-zone-desc">{dict.rstDesc}</span>
        </div>
        <div className="rst-zone-action">
          <button type="button" className="rst-open" onClick={() => setOpen(true)}>
            {dict.rstBtn}
          </button>
          <span className="rst-last">
            {info?.lastResetAt ? dict.rstLast.replace("%s", info.lastResetAt) : dict.rstNever}
          </span>
        </div>
      </div>

      <Dialog.Root open={open} onOpenChange={(o) => !o && close()}>
        <Dialog.Portal>
          <Dialog.Overlay className="confirm-backdrop rst-backdrop">
            <Dialog.Content
              className="rst-dialog asst-vars"
              role="alertdialog"
              onOpenAutoFocus={(e) => {
                e.preventDefault();
                cancelRef.current?.focus();
              }}
            >
              <div className="rst-dialog-head">
                <span className="rst-dialog-icon" aria-hidden="true">
                  !
                </span>
                <div className="rst-dialog-titles">
                  <Dialog.Title className="rst-dialog-title">{dict.rstTitle}</Dialog.Title>
                  <span className="rst-dialog-irrev">{dict.rstIrrev}</span>
                </div>
              </div>
              <Dialog.Description className="rst-dialog-body">{dict.rstBody}</Dialog.Description>
              <div className="rst-lists">
                <div className="rst-list rst-list--wipe">
                  <span className="rst-list-label">{dict.rstWipeLabel}</span>
                  {wipe.map((x) => (
                    <span className="rst-list-item" key={x}>
                      <span className="rst-list-mark">✕</span>
                      {x}
                    </span>
                  ))}
                </div>
                <div className="rst-list rst-list--keep">
                  <span className="rst-list-label">{dict.rstKeepLabel}</span>
                  {keep.map((x) => (
                    <span className="rst-list-item" key={x}>
                      <span className="rst-list-mark">✓</span>
                      {x}
                    </span>
                  ))}
                </div>
              </div>
              <div className="rst-backup">
                <span className="rst-backup-hint">{dict.rstBackupHint}</span>
                {info?.backupDir && <span className="rst-backup-path">{info.backupDir}</span>}
              </div>
              <label className="rst-ack">
                <input type="checkbox" checked={ack} onChange={() => setAck((a) => !a)} />
                <span>{dict.rstAck}</span>
              </label>
              <div className="rst-actions">
                <button type="button" ref={cancelRef} className="rst-cancel" onClick={close}>
                  {dict.cancel}
                </button>
                <button type="button" className="rst-ok" disabled={!ack || busy} onClick={confirm}>
                  {dict.rstOk}
                </button>
              </div>
            </Dialog.Content>
          </Dialog.Overlay>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  );
}
