// The frontend's UI strings. types.ts declares the Dictionary shape, en.ts
// and zh.ts each fill it in full, sectors.ts translates data-sourced sector
// names; importers keep using `from "../i18n"`.
import { en } from "./en";
import { zh } from "./zh";
import type { Dictionary, Lang } from "./types";

export type { Dictionary, Lang } from "./types";
export { sectorLabel } from "./sectors";

const dictionaries: Record<string, Dictionary> = { en, zh };

export function getDictionary(lang: string): Dictionary {
  return dictionaries[lang] ?? dictionaries.zh;
}

// normalizeLang collapses whatever string /api/config or localStorage held
// into a valid Lang, defaulting to zh — same fallback getDictionary applies,
// but usable where the Lang itself (not just the dictionary) is needed,
// e.g. highlighting the active button in Sidebar's language toggle.
export function normalizeLang(lang: string | null): Lang {
  return lang === "en" ? "en" : "zh";
}
