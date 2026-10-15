import { postJSON } from "./http";
import { movesNetWorth } from "./netWorth";

// --- Phase 5 §B CSV import (optional, docs/phase-5-web-dashboard.md) ---
// Mirrors internal/web/trade_import.go's importRow/importResponse.

export interface ImportRow {
  line: number;
  date: string;
  ticker: string;
  action: string;
  shares: number;
  price: number;
  fee: number;
  status: "ok" | "warning" | "duplicate" | "error" | "applied";
  message: string;
}

export interface ImportResult {
  rows: ImportRow[];
  applied: number;
  dryRun: boolean;
}

export function importCSV(csv: string, dryRun: boolean): Promise<ImportResult> {
  return postJSON("/api/import", { csv, dryRun });
}

// --- Phase 9 波次1 PR3' wealth CSV import (§8.15.1) ---
// Mirrors internal/web/wealth_import.go's wealthImportRow/wealthImportResponse.

export interface WealthImportRow {
  line: number;
  side: string;
  type: string;
  name: string;
  group: string;
  venue?: string;
  currency?: string;
  value: number;
  date: string;
  bank?: string;
  accountNote?: string;
  lender?: string;
  ratePct?: number;
  originalPrincipal?: number;
  remainingMonths?: number;
  status: "ok" | "warning" | "duplicate" | "error" | "applied";
  message: string;
}

export interface WealthImportResult {
  rows: WealthImportRow[];
  applied: number;
  dryRun: boolean;
}

export function importWealthCSV(csv: string, dryRun: boolean): Promise<WealthImportResult> {
  const result = postJSON<WealthImportResult>("/api/wealth/import", { csv, dryRun });
  return dryRun ? result : movesNetWorth(result); // a preview writes nothing
}
