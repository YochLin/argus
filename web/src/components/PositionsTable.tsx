import type { Position } from "../api";
import type { Dictionary } from "../i18n";

interface Props {
  positions: Position[];
  dict: Dictionary;
}

function fmtMoney(v: number): string {
  return `$${v.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function fmtSignedMoney(v: number): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${fmtMoney(Math.abs(v))}`;
}

function fmtPct(v: number): string {
  const sign = v > 0 ? "+" : "";
  return `${sign}${v.toFixed(2)}%`;
}

export function PositionsTable({ positions, dict }: Props) {
  if (positions.length === 0) {
    return <div className="empty-message">{dict.noPositions}</div>;
  }
  return (
    <table className="mono">
      <thead>
        <tr>
          <th>{dict.ticker}</th>
          <th>{dict.shares}</th>
          <th>{dict.avgCost}</th>
          <th>{dict.price}</th>
          <th>{dict.marketValue}</th>
          <th>{dict.unrealizedPnL}</th>
        </tr>
      </thead>
      <tbody>
        {positions.map((p) => (
          <tr key={p.ticker}>
            <td>{p.ticker}</td>
            <td>{p.shares}</td>
            <td>{fmtMoney(p.avgCost)}</td>
            <td>{fmtMoney(p.price)}</td>
            <td>{fmtMoney(p.marketValue)}</td>
            <td className={p.unrealizedPnL >= 0 ? "profit" : "loss"}>
              {fmtSignedMoney(p.unrealizedPnL)} ({fmtPct(p.unrealizedPnLPct)})
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
