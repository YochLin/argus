export type KpiFormat = "currency" | "percent" | "percentValue" | "ratio";
export type KpiColorMode = "pnl" | "loss" | "neutral";

interface Props {
  label: string;
  // null means "not computable yet" (e.g. YTD/QTD/HTD return% with no net
  // worth baseline on record) — rendered as "—", never a misleading 0.
  value: number | null;
  format: KpiFormat;
  colorMode?: KpiColorMode;
  // currency is Phase 6's per-market symbol ("$"/"NT$", see api.ts's
  // currencySymbol) — only meaningful when format is "currency"; ignored
  // otherwise. Defaults to "$" so every pre-Phase-6 caller is unaffected.
  currency?: string;
}

function formatValue(value: number, format: KpiFormat, currency: string): string {
  switch (format) {
    case "currency": {
      // P&L numbers always carry an explicit sign — design doc: color is a
      // second encoding, never the only one (colorblind-safe).
      const sign = value > 0 ? "+" : value < 0 ? "-" : "";
      return `${sign}${currency}${Math.abs(value).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
    }
    case "percent":
      return `${(value * 100).toFixed(1)}%`;
    // percentValue is the already-scaled-percentage counterpart of
    // "percent" (see api.ts's KPIs.ytdReturnPct doc) — used for return%
    // KPIs, which are directional like a P&L number, so it also carries an
    // explicit sign the way "percent" (win rate, never negative) doesn't.
    case "percentValue": {
      const sign = value > 0 ? "+" : value < 0 ? "-" : "";
      return `${sign}${Math.abs(value).toFixed(1)}%`;
    }
    case "ratio":
      return value.toFixed(2);
  }
}

export function KpiCard({ label, value, format, colorMode = "neutral", currency = "$" }: Props) {
  if (value === null) {
    return (
      <div className="card">
        <div className="eyebrow">{label}</div>
        <div className="kpi-value">—</div>
      </div>
    );
  }

  let colorClass = "";
  if (colorMode === "loss") {
    colorClass = "loss";
  } else if (colorMode === "pnl") {
    colorClass = value > 0 ? "profit" : value < 0 ? "loss" : "";
  }

  return (
    <div className="card">
      <div className="eyebrow">{label}</div>
      <div className={`kpi-value ${colorClass}`}>{formatValue(value, format, currency)}</div>
    </div>
  );
}
