import type { TradeResponse } from "./common";
import { getJSON, postJSON } from "./http";
import { movesNetWorth } from "./netWorth";

// --- Phase 9: wealth platform (docs/phase-9-asset-platform.md) ---
// Mirrors internal/web/wealth_assets.go and wealth_home.go.

export type AssetSide = "asset" | "liability";
export type AssetGroup = "liquid" | "growth" | "income" | "hard";
export type AllocationModel = "conserv" | "balanced" | "growth";

export interface WealthAsset {
  id: number;
  side: AssetSide;
  type: string;
  name: string;
  assetGroup: AssetGroup;
  venue: string;
  currency: string;
  source: "manual" | "import" | "sync";
  createdAt: string;
  archivedAt?: string;
  // value/cost are null when the asset has no snapshot yet — render "—",
  // never 0 (mirrors db.AssetWithValue's doc comment on the Go side).
  value: number | null;
  cost?: number | null;
  asOf?: string;
  // valueTwd is value in TWD at today's rate — absent when there is no value or
  // its currency couldn't be priced. What the wealth pages show for a foreign asset.
  valueTwd?: number;
}

export function fetchWealthAssets(includeArchived = false): Promise<{ assets: WealthAsset[] }> {
  return getJSON(`/api/wealth/assets${includeArchived ? "?archived=1" : ""}`);
}

export interface WealthAllocationRow {
  group: AssetGroup;
  marketValue: number;
  currentPct: number;
  targetPct: number;
  deviationPt: number;
}

// WealthHome mirrors wealth_home.go's wealthHomeResponse — netWorth/ytdPct/
// momPct are null when there isn't enough snapshot history yet (a fresh
// account, or a currency with no recorded FX rate for the baseline date);
// render "—", never 0/0%.
export interface WealthHome {
  asOf: string;
  netWorth: number | null;
  ytdPct: number | null;
  momPct: number | null;
  totalAssets: number | null;
  totalLiabilities: number | null;
  model: AllocationModel;
  allocation: WealthAllocationRow[];
  // staleCount: manual/import records last valued 90+ days ago (the amber banner).
  staleCount: number;
}

export function fetchWealthHome(model: AllocationModel): Promise<WealthHome> {
  return getJSON(`/api/wealth/networth?model=${model}`);
}

export interface CreateWealthAssetRequest {
  side: AssetSide;
  type: string;
  name: string;
  assetGroup: AssetGroup;
  venue?: string;
  currency?: string;
  initialValue: number;
  date?: string;
  deposit?: { bank?: string; accountNote?: string };
  loan?: {
    lender?: string;
    ratePct?: number;
    originalPrincipal?: number;
    remainingMonths?: number;
    securedAssetId?: number;
  };
}

export function createWealthAsset(req: CreateWealthAssetRequest): Promise<{ id: number }> {
  return movesNetWorth(postJSON("/api/wealth/assets", req));
}

// saveWealthAssetSnapshot backs the balance-sheet's in-place edit — always
// writes today's (or date's) value, never renames/edits the asset itself.
export function saveWealthAssetSnapshot(assetId: number, value: number, date?: string): Promise<TradeResponse> {
  return movesNetWorth(postJSON("/api/wealth/assets/snapshot", { assetId, value, date }));
}

export function archiveWealthAsset(assetId: number): Promise<TradeResponse> {
  return movesNetWorth(postJSON("/api/wealth/assets/archive", { assetId }));
}

export function unarchiveWealthAsset(assetId: number): Promise<TradeResponse> {
  return movesNetWorth(postJSON("/api/wealth/assets/unarchive", { assetId }));
}

// updateWealthAsset changes only the descriptive fields — side, type, currency
// and value are never editable (see db.AssetEdit). Details are left alone.
export function updateWealthAsset(req: {
  assetId: number;
  name: string;
  assetGroup: AssetGroup;
  venue: string;
}): Promise<TradeResponse> {
  return postJSON("/api/wealth/assets/update", req);
}

export interface WealthSnapshotRow {
  date: string;
  value: number; // in the asset's own currency
  source: "manual" | "import" | "sync";
}

// fetchWealthAssetHistory returns one asset's value records newest first, plus
// the server's today — the date the write routes judge "today's record" by.
export function fetchWealthAssetHistory(assetId: number): Promise<{ today: string; snapshots: WealthSnapshotRow[] }> {
  return getJSON(`/api/wealth/assets/history?assetId=${assetId}`);
}

// deleteWealthAssetSnapshot undoes a record logged today (the server refuses
// any other date).
export function deleteWealthAssetSnapshot(assetId: number, date: string): Promise<TradeResponse> {
  return movesNetWorth(postJSON("/api/wealth/assets/snapshot/delete", { assetId, date }));
}
