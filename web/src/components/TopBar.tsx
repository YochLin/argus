import { useEffect, useRef, useState } from "react";
import type { Market } from "../api";
import type { Dictionary, Lang } from "../i18n";
import { DISPLAY_CURRENCIES, type DisplayCurrency } from "../currency";

const LANGS: Array<{ id: Lang; flag: string; code: string; label: string }> = [
  { id: "zh", flag: "🇹🇼", code: "中文", label: "繁體中文" },
  { id: "en", flag: "🇺🇸", code: "EN", label: "English" },
];

interface Props {
  market: Market;
  onMarketChange: (market: Market) => void;
  // hideMarketToggle (Phase 9, docs/phase-9-asset-platform.md §8.1) hides
  // the US/TW toggle on wealth pages — assets aren't market-scoped, so
  // showing it there would imply a distinction that doesn't apply. An
  // empty placeholder still renders in its place so .topbar's
  // justify-content:space-between keeps the right-hand cluster pinned
  // right instead of collapsing to the left with only one child.
  hideMarketToggle?: boolean;
  isDark: boolean;
  onToggleTheme: () => void;
  lang: Lang;
  onLangChange: (lang: Lang) => void;
  dict: Dictionary;
  // writable/onAddTrade (Phase 10, docs/phase-10-web-trade-input.md §2
  // item 6) — the shell-level "+ Trade" entry point, absent entirely (not
  // just disabled) when WEB_PASSWORD isn't configured server-side.
  writable?: boolean;
  onAddTrade?: () => void;
  // fx (wealth pages only): the 顯示幣別 selector. rates = TWD per unit from
  // /api/wealth/fx; a currency without a rate is disabled, not guessed.
  fx?: {
    code: DisplayCurrency;
    rates: Record<string, number>;
    onChange: (code: DisplayCurrency) => void;
  };
}

// Shell-level top bar (Figma reference layout): market switch on the left,
// language + theme toggles grouped at the far right in that order — the
// language toggle used to sit in the Sidebar, but the Figma reference pairs
// it directly beside the theme toggle in the top bar's right-hand cluster.
export function TopBar({
  market,
  onMarketChange,
  hideMarketToggle = false,
  isDark,
  onToggleTheme,
  lang,
  onLangChange,
  dict,
  writable = false,
  onAddTrade,
  fx,
}: Props) {
  return (
    <div className="topbar">
      {hideMarketToggle ? (
        <div />
      ) : (
        <div className="topbar-tabs" role="group" aria-label="market">
          <button
            className={`topbar-tab${market === "us" ? " active" : ""}`}
            onClick={() => onMarketChange("us")}
          >
            US
          </button>
          <button
            className={`topbar-tab${market === "tw" ? " active" : ""}`}
            onClick={() => onMarketChange("tw")}
          >
            TW
          </button>
        </div>
      )}
      <div className="topbar-right">
        {fx && <FxDropdown dict={dict} {...fx} />}
        {writable && (
          <button className="theme-toggle" onClick={onAddTrade}>
            {dict.addTrade}
          </button>
        )}
        <LangDropdown lang={lang} onLangChange={onLangChange} />
        <button className="theme-toggle" onClick={onToggleTheme} aria-label="toggle theme">
          {isDark ? <SunIcon /> : <MoonIcon />}
          <span>{isDark ? dict.themeLight : dict.themeDark}</span>
        </button>
      </div>
    </div>
  );
}

function FxDropdown({
  dict,
  code,
  rates,
  onChange,
}: {
  dict: Dictionary;
  code: DisplayCurrency;
  rates: Record<string, number>;
  onChange: (code: DisplayCurrency) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  return (
    <div className="lang-dropdown" ref={ref}>
      <button
        className="lang-dropdown-trigger"
        style={{ fontFamily: "var(--font-mono)" }}
        onClick={() => setOpen((v) => !v)}
        aria-label="display currency"
      >
        <span style={{ color: "var(--ink-3)", fontSize: 10, letterSpacing: ".06em" }}>
          {dict.wealthDisplayCurrency}
        </span>
        <span>{code}</span>
        <ChevronDownIcon className={`lang-dropdown-chevron${open ? " open" : ""}`} />
      </button>
      {open && (
        <div className="lang-dropdown-panel" style={{ minWidth: 120 }}>
          {DISPLAY_CURRENCIES.map((c) => (
            <button
              key={c}
              className={`lang-dropdown-item${c === code ? " active" : ""}`}
              style={{ fontFamily: "var(--font-mono)" }}
              disabled={c !== "TWD" && !rates[c]}
              onClick={() => {
                onChange(c);
                setOpen(false);
              }}
            >
              {c}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function LangDropdown({ lang, onLangChange }: { lang: Lang; onLangChange: (lang: Lang) => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const current = LANGS.find((l) => l.id === lang) ?? LANGS[0];

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  return (
    <div className="lang-dropdown" ref={ref}>
      <button
        className="lang-dropdown-trigger"
        onClick={() => setOpen((v) => !v)}
        aria-label="change language"
      >
        <GlobeIcon />
        <span>{current.code}</span>
        <ChevronDownIcon className={`lang-dropdown-chevron${open ? " open" : ""}`} />
      </button>
      {open && (
        <div className="lang-dropdown-panel">
          {LANGS.map((l) => (
            <button
              key={l.id}
              className={`lang-dropdown-item${l.id === lang ? " active" : ""}`}
              onClick={() => {
                onLangChange(l.id);
                setOpen(false);
              }}
            >
              <span>{l.flag}</span>
              <span className="lang-dropdown-item-label">{l.label}</span>
              {l.id === lang && <span className="lang-dropdown-item-dot" />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

const iconProps = {
  width: 13,
  height: 13,
  viewBox: "0 0 16 16",
  stroke: "currentColor",
  strokeWidth: 1.5,
  fill: "none",
};

function SunIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <circle cx="8" cy="8" r="3.5" />
      <line x1="8" y1="1" x2="8" y2="2.5" />
      <line x1="8" y1="13.5" x2="8" y2="15" />
      <line x1="1" y1="8" x2="2.5" y2="8" />
      <line x1="13.5" y1="8" x2="15" y2="8" />
      <line x1="3.1" y1="3.1" x2="4.2" y2="4.2" />
      <line x1="11.8" y1="11.8" x2="12.9" y2="12.9" />
      <line x1="3.1" y1="12.9" x2="4.2" y2="11.8" />
      <line x1="11.8" y1="4.2" x2="12.9" y2="3.1" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <path d="M13.5 9.5A5.5 5.5 0 0 1 6.5 2.5 5.5 5.5 0 1 0 13.5 9.5Z" />
    </svg>
  );
}

function GlobeIcon() {
  return (
    <svg {...iconProps} aria-hidden="true">
      <circle cx="8" cy="8" r="6.5" />
      <line x1="1.5" y1="8" x2="14.5" y2="8" />
      <path d="M8 1.5c-2.2 1.8-2.2 11.2 0 13 2.2-1.8 2.2-11.2 0-13Z" />
    </svg>
  );
}

export function ChevronDownIcon({ className }: { className?: string }) {
  return (
    <svg {...iconProps} width={10} height={10} className={className} aria-hidden="true">
      <polyline points="3,6 8,11 13,6" />
    </svg>
  );
}
