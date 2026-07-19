export type KpiFormat = "currency" | "percent" | "ratio";
export type KpiColorMode = "pnl" | "loss" | "neutral";

interface Props {
  label: string;
  value: number;
  format: KpiFormat;
  colorMode?: KpiColorMode;
}

function formatValue(value: number, format: KpiFormat): string {
  switch (format) {
    case "currency": {
      // P&L numbers always carry an explicit sign — design doc: color is a
      // second encoding, never the only one (colorblind-safe).
      const sign = value > 0 ? "+" : value < 0 ? "-" : "";
      return `${sign}$${Math.abs(value).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
    }
    case "percent":
      return `${(value * 100).toFixed(1)}%`;
    case "ratio":
      return value.toFixed(2);
  }
}

export function KpiCard({ label, value, format, colorMode = "neutral" }: Props) {
  let colorClass = "";
  if (colorMode === "loss") {
    colorClass = "loss";
  } else if (colorMode === "pnl") {
    colorClass = value > 0 ? "profit" : value < 0 ? "loss" : "";
  }

  return (
    <div className="card">
      <div className="eyebrow">{label}</div>
      <div className={`kpi-value ${colorClass}`}>{formatValue(value, format)}</div>
    </div>
  );
}
