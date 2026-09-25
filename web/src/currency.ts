// Wealth pages' 顯示幣別 (design: top-bar selector). Every wealth value is
// TWD server-side; this module holds the one active display currency and
// fmtMoney reads it, so no view needs to know about conversion. App owns the
// React state that triggers the re-render — the module var is only what
// fmtMoney reads during it.
export const DISPLAY_CURRENCIES = ["TWD", "USD", "JPY", "EUR", "CNY"] as const;
export type DisplayCurrency = (typeof DISPLAY_CURRENCIES)[number];

export const CURRENCY_SYMBOL: Record<DisplayCurrency, string> = {
  TWD: "NT$",
  USD: "$",
  JPY: "¥",
  EUR: "€",
  CNY: "CN¥",
};

let active: { code: DisplayCurrency; twdPerUnit: number } = { code: "TWD", twdPerUnit: 1 };

export function setDisplayCurrency(code: DisplayCurrency, twdPerUnit: number) {
  active = { code, twdPerUnit };
}

// convertTWD returns the display symbol and amount for a TWD value.
export function convertTWD(v: number): { symbol: string; value: number } {
  return { symbol: CURRENCY_SYMBOL[active.code], value: v / active.twdPerUnit };
}
