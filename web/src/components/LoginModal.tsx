import { useState, type FormEvent } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { ApiError, login } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  onClose: () => void;
  onSuccess: () => void;
}

// The single write-auth prompt (docs/phase-10-web-trade-input.md §4.1) —
// App.tsx renders this whenever any write attempt (TradeModal or
// ChartListView's watchlist add/remove) gets a 401, and calls onSuccess's
// caller-supplied retry closure once login succeeds.
export function LoginModal({ dict, onClose, onSuccess }: Props) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await login(password);
      onSuccess();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : dict.error);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog.Root open onOpenChange={(open) => !open && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="modal-backdrop">
          {/* Content nested inside Overlay so .modal-backdrop's flex centering
              keeps working unchanged; onOpenAutoFocus is prevented so React's
              own autoFocus below still wins (Radix would otherwise pull focus
              to the first tabbable element, which is the × button). */}
          <Dialog.Content
            className="modal"
            aria-describedby={undefined}
            onOpenAutoFocus={(e) => e.preventDefault()}
          >
            <div className="modal-header">
              <Dialog.Title className="eyebrow">{dict.loginTitle}</Dialog.Title>
              <Dialog.Close className="modal-close" aria-label="close">
                ×
              </Dialog.Close>
            </div>
            <form className="modal-body" onSubmit={submit}>
              <label className="form-field">
                <span>{dict.password}</span>
                <input
                  className="mono"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoFocus
                />
              </label>
              {error && <div className="error-message">{error}</div>}
              <div className="modal-actions">
                <button type="button" onClick={onClose}>
                  {dict.cancel}
                </button>
                <button type="submit" className="btn-primary" disabled={submitting || !password}>
                  {dict.login}
                </button>
              </div>
            </form>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
