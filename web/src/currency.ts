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

// shortTWD is the template's wshort(): a compact amount for subtitles
// ("NT$25.35M"); JPY switches to 億 from 1e8 like the template does.
export function shortTWD(v: number): string {
  const { symbol, value } = convertTWD(v);
  const abs = Math.abs(value);
  const sign = value < 0 ? "−" : "";
  if (active.code === "JPY" && abs >= 1e8) return `${sign}${symbol}${(abs / 1e8).toFixed(2)}億`;
  if (abs >= 1e6) return `${sign}${symbol}${(abs / 1e6).toFixed(2)}M`;
  if (abs >= 1e3) return `${sign}${symbol}${Math.round(abs / 1e3)}K`;
  return `${sign}${symbol}${Math.round(abs)}`;
}
