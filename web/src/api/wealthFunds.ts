import { getJSON } from "./http";

// FundRow/FundScheduleItem/FundChartPoint/WealthFunds mirror wealth_funds.go's
// response shapes (Phase 9 波次4 PR10). MarketValue/Cost/PnL/ReturnPct on
// WealthFunds degrade together to null the moment any fund's currency
// couldn't be priced to TWD (§8.17.1) — MonthlyAmount/Cost/Value/PnL/
// ReturnPct on a FundRow instead degrade independently per row, and
// MonthlyAmount null/0 means the DCA schedule has stopped (render 已停扣).
export interface FundRow {
  assetId: number;
  name: string;
  code: string;
  platform: string;
  monthlyAmount: number | null;
  cost: number | null;
  value: number | null;
  pnl: number | null;
  returnPct: number | null;
  oneYearReturnPct: number | null;
  nextContributionDate?: string;
}

export interface FundScheduleItem {
  date: string;
  names: string;
  amount: number;
}

export interface FundChartPoint {
  date: string;
  cost: number | null;
  value: number | null;
}

export interface WealthFunds {
  asOf: string;
  marketValue: number | null;
  cost: number | null;
  pnl: number | null;
  returnPct: number | null;
  monthlyTotal: number;
  chart: FundChartPoint[];
  schedule: FundScheduleItem[];
  rows: FundRow[];
  lastImportDate?: string;
}

export function fetchWealthFunds(): Promise<WealthFunds> {
  return getJSON("/api/wealth/funds");
}
