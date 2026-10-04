import { useRef, useState, type ReactNode } from "react";
import type { WealthAsset } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  dict: Dictionary;
  dotColor?: string;
  name: string;
  note?: string; // a liability's rate/term line
  tag: ReactNode; // the source pill
  valueText: string;
  loss?: boolean;
  // The stored record behind the row. Undefined for the equity row, which is
  // derived from the trading account (link) and has no record to edit.
  asset?: WealthAsset;
  link: boolean;
  writable: boolean;
  open: boolean; // its drawer is showing
  onOpen: (a: WealthAsset) => void;
  onGoTrade: () => void;
  onInlineSave: (a: WealthAsset, value: number) => void;
}

// BalanceRow is one asset/liability line of /w/balance (design: wEditCell). A
// manual, TWD-denominated value is typed over in place (Enter saves, Esc
// cancels); any other value opens the row editor drawer, where the currency is
// spelled out and the history is shown, and the equity row links to the trading
// account. The ⋯ button always opens the drawer.
export function BalanceRow({ dict, dotColor, name, note, tag, valueText, loss, asset, link, writable, open, onOpen, onGoTrade, onInlineSave }: Props) {
  const [draft, setDraft] = useState<string | null>(null); // non-null while editing
  const cancelled = useRef(false);
  const inline = !!asset && writable && asset.source === "manual" && asset.currency === "TWD";
  const valueClass = `wealth-item-row-value${loss ? " loss" : ""}`;

  // Esc blurs the input too, so it flags itself first and commit() checks it.
  function commit() {
    const raw = draft ?? "";
    const n = Number(raw);
    const skip = cancelled.current;
    cancelled.current = false;
    setDraft(null);
    if (skip || !asset) return;
    if (raw.trim() !== "" && Number.isFinite(n) && n !== asset.value) onInlineSave(asset, n);
  }

  let cell: ReactNode;
  if (draft != null) {
    cell = (
      <span className="wealth-value-editing">
        <span className="wealth-value-editing-cur">NT$</span>
        <input
          autoFocus
          aria-label={dict.wealthValue}
          inputMode="decimal"
          value={draft}
          onChange={(e) => setDraft(e.target.value.replace(/[^0-9.-]/g, ""))}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") e.currentTarget.blur();
            else if (e.key === "Escape") {
              cancelled.current = true;
              e.currentTarget.blur();
            }
          }}
        />
      </span>
    );
  } else if (link) {
    cell = (
      <button type="button" className={`${valueClass} wealth-value-open`} title={dict.wealthRowLinkTitle} onClick={onGoTrade}>
        {valueText}
      </button>
    );
  } else if (asset && inline) {
    cell = (
      <button
        type="button"
        className={`${valueClass} wealth-value-editable`}
        title={dict.wealthRowEditHint}
        onClick={() => setDraft(asset.value != null ? String(asset.value) : "")}
      >
        {valueText}
      </button>
    );
  } else if (asset) {
    cell = (
      <button type="button" className={`${valueClass} wealth-value-open`} title={dict.wealthRowMore} onClick={() => onOpen(asset)}>
        {valueText}
      </button>
    );
  } else {
    cell = <span className={valueClass}>{valueText}</span>;
  }

  return (
    <div className="wealth-item-row">
      {dotColor && <span style={{ width: 8, height: 8, borderRadius: 2, flexShrink: 0, background: dotColor }} />}
      <span>{name}</span>
      {note && (
        <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)" }}>
          {note}
        </span>
      )}
      {tag}
      {cell}
      {link ? (
        <button type="button" className="wealth-row-more" title={dict.wealthRowLinkGo} aria-label={dict.wealthRowLinkGo} onClick={onGoTrade}>
          →
        </button>
      ) : (
        asset && (
          <button
            type="button"
            className={`wealth-row-more${open ? " open" : ""}`}
            title={dict.wealthRowMore}
            aria-label={dict.wealthRowMore}
            onClick={() => onOpen(asset)}
          >
            ⋯
          </button>
        )
      )}
    </div>
  );
}
