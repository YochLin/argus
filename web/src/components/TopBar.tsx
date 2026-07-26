import type { Market } from "../api";
import type { Dictionary, Lang } from "../i18n";

interface Props {
  market: Market;
  onMarketChange: (market: Market) => void;
  isDark: boolean;
  onToggleTheme: () => void;
  lang: Lang;
  onLangChange: (lang: Lang) => void;
  dict: Dictionary;
}

// Shell-level top bar (Figma reference layout): market switch on the left,
// language + theme toggles grouped at the far right in that order — the
// language toggle used to sit in the Sidebar, but the Figma reference pairs
// it directly beside the theme toggle in the top bar's right-hand cluster.
export function TopBar({
  market,
  onMarketChange,
  isDark,
  onToggleTheme,
  lang,
  onLangChange,
  dict,
}: Props) {
  return (
    <div className="topbar">
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
      <div className="topbar-right">
        <div className="market-toggle" role="group" aria-label="language">
          <button
            className={`market-toggle-btn${lang === "zh" ? " active" : ""}`}
            onClick={() => onLangChange("zh")}
          >
            中文
          </button>
          <button
            className={`market-toggle-btn${lang === "en" ? " active" : ""}`}
            onClick={() => onLangChange("en")}
          >
            EN
          </button>
        </div>
        <button className="theme-toggle" onClick={onToggleTheme} aria-label="toggle theme">
          {isDark ? <SunIcon /> : <MoonIcon />}
          <span>{isDark ? dict.themeLight : dict.themeDark}</span>
        </button>
      </div>
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
