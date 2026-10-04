import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  howOpen: boolean;
  onToggleHow: () => void;
}

// Shown at the top of every page while the server has no WEB_PASSWORD, i.e.
// every write endpoint 404s. Without it the Add/Edit buttons just vanish and
// the user can't tell "nothing to do here" from "editing is switched off".
export function ReadOnlyBanner({ dict, howOpen, onToggleHow }: Props) {
  return (
    <div className="ro">
      <div className="ro-banner">
        <span className="ro-tag">
          <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
            <rect x="3" y="7" width="10" height="7" rx="1.5" />
            <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
          </svg>
          {dict.roTag}
        </span>
        <span className="ro-msg">{dict.roMsg}</span>
        <button type="button" className="ro-how" onClick={onToggleHow}>
          {howOpen ? dict.roHide : dict.roHow}
        </button>
      </div>
      {howOpen && (
        <ol className="ro-steps">
          {[dict.roStep1, dict.roStep2, dict.roStep3].map((text, i) => (
            <li key={i}>
              <span className="ro-step-n">{i + 1}</span>
              <span>
                {text}
                {i === 0 && <span className="ro-step-code">WEB_PASSWORD=…</span>}
              </span>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}

// ReadOnlyImportPanel stands in for the import form on /import and /w/import
// while read-only — those pages used to render nothing at all. The wealth
// variant sits inside the page's existing card under the column guide.
export function ReadOnlyImportPanel({
  dict,
  body,
  howOpen,
  onToggleHow,
}: Props & { body: string }) {
  return (
    <div className="ro-import">
      <span className="ro-import-title">{dict.roImportTitle}</span>
      <span className="ro-import-body">{body}</span>
      <button type="button" className="ro-import-how" onClick={onToggleHow}>
        {howOpen ? dict.roHide : dict.roHow}
      </button>
    </div>
  );
}
