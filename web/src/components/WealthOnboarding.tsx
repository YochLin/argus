import { useEffect, useState } from "react";
import { fetchWealthAssets, fetchWealthCash, fetchWealthInsure, fetchWealthProfile } from "../api";
import type { Dictionary } from "../i18n";
import { hasPickedModel } from "../wealthCategory";

// WealthEmptyCard is what a wealth page shows in place of its body while it has
// nothing to show (Argus Trading WebUI.dc.html's wes.on): one line and an add
// button. No button when onAdd is missing — a read-only server can't add.
export function WealthEmptyCard({ dict, line, onAdd }: { dict: Dictionary; line: string; onAdd?: () => void }) {
  return (
    <div className="card wealth-empty">
      <span className="wealth-empty-line">{line}</span>
      {onAdd && (
        <button className="wealth-empty-add" onClick={onAdd}>
          <span style={{ fontSize: 14, lineHeight: 1 }}>＋</span>
          {dict.wealthHomeAddNew}
        </button>
      )}
    </div>
  );
}

// useAssetCount is the number of recorded assets (liabilities don't count —
// allocation and retirement both work off assets), null until the list has
// loaded. Asked of the list itself because those pages' own totals are also
// null when a currency has no rate, which is not "nothing recorded".
export function useAssetCount(refreshSignal: number): number | null {
  const [n, setN] = useState<number | null>(null);
  useEffect(() => {
    fetchWealthAssets()
      .then((r) => setN(r.assets.filter((a) => a.side === "asset").length))
      .catch(() => {});
  }, [refreshSignal]);
  return n;
}

// useNoAssets is true once the asset list has loaded and holds no asset.
export function useNoAssets(refreshSignal: number): boolean {
  return useAssetCount(refreshSignal) === 0;
}

// wealthReadiness is the design's wReady(): why a metric reads "—" instead of
// a number. whyFewA (fewer than 3 assets) blocks anything computed from the
// mix; whyDrift adds "no target model picked yet", since drift and rebalance
// orders are measured against it; whySpend is "no monthly spending", which the
// liquidity months need. "" means that reason doesn't apply.
export function wealthReadiness(dict: Dictionary, assetCount: number, spendKnown: boolean) {
  const whyFewA = assetCount < 3 ? dict.wealthHomeNeedAssets.replace("%s", String(assetCount)) : "";
  return {
    whyFewA,
    whyDrift: hasPickedModel() ? whyFewA : dict.wealthNeedModel,
    whySpend: spendKnown ? "" : dict.wealthNeedSpend,
  };
}

// thinNote is the design's "資料不足 · reason" line under a metric.
export const thinNote = (dict: Dictionary, why: string) => (why ? `${dict.wealthHomeThinShort} · ${why}` : "");

// "Hide" is this app's own addition: the design's guide is part of an
// empty-account demo, but a real account that never takes the optional
// insurance step would see it on /w forever.
const HIDE_KEY = "argus.wealth.guideHidden";

function loadHidden(): boolean {
  try {
    return localStorage.getItem(HIDE_KEY) === "1";
  } catch {
    return false;
  }
}

// WealthGuide is /w's "suggested setup order" (the design's wes.showGuide):
// five steps, each ticked off once its data exists, gone once all five are.
// Step 1 is told by the caller (the home page already has the asset list);
// the other four are read from what their own pages save. It shows nothing
// until those are loaded, and nothing if they fail to load.
export function WealthGuide({ dict, hasAssets, onNavigate }: { dict: Dictionary; hasAssets: boolean; onNavigate: (path: string) => void }) {
  const [hidden, setHidden] = useState(loadHidden);
  const [saved, setSaved] = useState<{ cash: boolean; birth: boolean; policy: boolean } | null>(null);

  useEffect(() => {
    Promise.all([fetchWealthCash(), fetchWealthProfile(), fetchWealthInsure()])
      .then(([c, p, i]) =>
        setSaved({
          cash: c.items.some((f) => f.active && f.direction === "out"),
          birth: p.birthYear != null,
          policy: i.policies.length > 0,
        }),
      )
      .catch(() => {});
  }, []);

  if (hidden || !saved) return null;

  const steps: [string, string, string, boolean][] = [
    [dict.navWealthBalance, dict.wealthGuideWhyBalance, "/w/balance", hasAssets],
    [dict.navWealthCash, dict.wealthGuideWhyCash, "/w/cash", saved.cash],
    [dict.navWealthRetire, dict.wealthGuideWhyRetire, "/w/retire", saved.birth],
    [dict.navWealthAlloc, dict.wealthGuideWhyAlloc, "/w/alloc", hasPickedModel()],
    [dict.navWealthInsure, dict.wealthGuideWhyInsure, "/w/insure", saved.policy],
  ];
  const done = steps.filter((s) => s[3]).length;
  if (done === steps.length) return null;

  function hide() {
    setHidden(true);
    try {
      localStorage.setItem(HIDE_KEY, "1");
    } catch {
      // per-browser convenience only; it just comes back next visit
    }
  }

  return (
    <div className="card wealth-guide">
      <div className="wealth-guide-head">
        <span className="wealth-guide-title">{dict.wealthGuideTitle}</span>
        <span className="wealth-guide-count">{dict.wealthGuideDone.replace("%s", String(done)).replace("%s", String(steps.length))}</span>
        <button className="wealth-guide-hide" onClick={hide}>
          {dict.wealthGuideHide}
        </button>
      </div>
      <div className="wealth-guide-steps">
        {steps.map(([label, why, path, ok], i) => (
          <div key={path} className="wealth-guide-step">
            <span className={`wealth-guide-mark${ok ? " done" : ""}`}>{ok ? "✓" : i + 1}</span>
            <a
              href={path}
              className={`wealth-guide-label${ok ? " done" : ""}`}
              onClick={(e) => {
                e.preventDefault();
                onNavigate(path);
              }}
            >
              {label}
            </a>
            <span className="wealth-guide-why">{why}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
