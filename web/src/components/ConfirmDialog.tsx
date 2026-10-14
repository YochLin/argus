import * as Dialog from "@radix-ui/react-dialog";

interface Props {
  title: string;
  body: string;
  okLabel: string;
  cancelLabel: string;
  onCancel: () => void;
  onConfirm: () => void;
  // variant adds a class to the dialog for a design that styles its own
  // (the wealth assistant's "asst"); tone "primary" is for a confirm that
  // destroys nothing worth a red button (the assistant's 開新對話).
  variant?: string;
  tone?: "danger" | "primary";
}

// The shared confirm step for a destructive action (archive today; delete
// later) — replaces window.confirm. Cancel is the first focusable element, so
// Radix's own initial focus lands there: Enter on a freshly opened dialog
// cancels rather than destroys. Esc and a click outside also cancel.
export function ConfirmDialog({ title, body, okLabel, cancelLabel, onCancel, onConfirm, variant, tone = "danger" }: Props) {
  return (
    <Dialog.Root open onOpenChange={(open) => !open && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className={`confirm-backdrop${variant ? ` confirm-backdrop--${variant}` : ""}`}>
          <Dialog.Content className={`confirm-dialog${variant ? ` confirm-dialog--${variant}` : ""}${tone === "primary" ? " confirm-dialog--primary" : ""}`} role="alertdialog">
            <Dialog.Title className="confirm-title">{title}</Dialog.Title>
            <Dialog.Description className="confirm-body">{body}</Dialog.Description>
            <div className="confirm-actions">
              <button type="button" onClick={onCancel}>
                {cancelLabel}
              </button>
              <button type="button" className="confirm-ok" onClick={onConfirm}>
                {okLabel}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
