import { useEffect, useState } from "react";
import { fetchWealthAIView, type WealthAIView } from "../api";
import { useAssistant } from "../assistant";
import type { Dictionary } from "../i18n";

const Sparkle = () => (
  <svg width="13" height="13" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
    <path d="M8 1.5 9.4 6.6 14.5 8 9.4 9.4 8 14.5 6.6 9.4 1.5 8 6.6 6.6Z" />
  </svg>
);

// "2026-11-01" → "11/1", the design's date on the card.
const shortDate = (d: string) => d.slice(5).split("-").map(Number).join("/");

// WealthAIViewCard is the home page's "AI 觀點" under 本月該做的事: what the
// monthly report's model noticed beyond the rules. It is set apart (dashed,
// labelled not rule-checked) so nobody reads it as one of the checks, and
// collapsed by default. Nothing shows before the first monthly report.
export function WealthAIViewCard({ dict, refresh }: { dict: Dictionary; refresh: number }) {
  const [view, setView] = useState<WealthAIView | null>(null);
  const [open, setOpen] = useState(false);
  const assistant = useAssistant();

  useEffect(() => {
    fetchWealthAIView()
      .then(setView)
      .catch(() => setView(null));
  }, [refresh]);

  if (!view) return null;
  const date = shortDate(view.generatedAt);

  return (
    <section className="aiv-card" aria-labelledby="aiv-title">
      <div className="aiv-head">
        <span className="aiv-icon" aria-hidden="true">
          <Sparkle />
        </span>
        <h2 id="aiv-title" className="aiv-title">
          {dict.aivTitle}
        </h2>
        <span className="aiv-meta">{dict.aivMeta.replace("{date}", date)}</span>
        {!open && <span className="aiv-summary">{view.summary}</span>}
        <span className="aiv-spacer" />
        <button className="aiv-toggle" aria-expanded={open} aria-controls="aiv-body" onClick={() => setOpen(!open)}>
          {open ? dict.aivCollapse : dict.aivExpand}
        </button>
        {assistant && (
          <button className="aiv-ask" onClick={() => assistant.open("home", dict.aivAskText.replace("{date}", date).replace("{summary}", view.summary))}>
            {dict.aivAsk} ›
          </button>
        )}
      </div>
      {open && (
        <ul id="aiv-body" className="aiv-points">
          {view.points.map((p, i) => (
            <li key={i}>{p}</li>
          ))}
        </ul>
      )}
    </section>
  );
}
