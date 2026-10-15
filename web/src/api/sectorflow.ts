import type { Market } from "./common";
import { getJSON } from "./http";

// SectorFlow mirrors internal/web/sectorflow.go's SectorFlowResponse (Phase
// 18). Cap is real market cap for US, a trading-value proxy for TW — same
// field name regardless of source (see that file's doc comment); the TW
// page renders sectorFlowTWCapNote so the difference isn't silently implied
// as apples-to-apples. Ready false means no scan has completed for this
// market yet (fresh process restart), not "market has zero sectors."
export interface SectorFlowItem {
  ticker: string;
  cap: number;
  flow: number;
  changePct: number;
  held: boolean;
}

export interface SectorFlowSector {
  name: string;
  netFlow: number;
  dollarVolume: number;
  marketCap: number;
  changePct: number;
  tickerCount: number;
  heldCount: number;
  items: SectorFlowItem[];
}

export interface SectorFlow {
  ready: boolean;
  computedAt: string;
  sectors: SectorFlowSector[];
}

export type SectorFlowTimeframe = "1d" | "1w" | "1m";

export function fetchSectorFlow(market: Market = "us", tf: SectorFlowTimeframe = "1d"): Promise<SectorFlow> {
  return getJSON<SectorFlow>(`/api/sectorflow?market=${market}&tf=${tf}`);
}

// refreshSectorFlow mirrors internal/web/sectorflow.go's
// POST /api/sectorflow/refresh — a manual trigger for the same scan the
// nightly cron job runs, since the cache is process-only and otherwise only
// fills on its 06:10/14:55 CST schedule. Unlike the Phase 10 write
// endpoints (trade.ts), this needs no auth (see that handler's doc comment), so
// it doesn't go through postJSON's ApiError/401-retry handling.
export async function refreshSectorFlow(market: Market = "us"): Promise<{ message: string }> {
  const res = await fetch(`/api/sectorflow/refresh?market=${market}`, { method: "POST" });
  if (!res.ok) {
    const data: unknown = await res.json().catch(() => ({}));
    throw new Error((data as { error?: string })?.error ?? `HTTP ${res.status}`);
  }
  return res.json();
}
