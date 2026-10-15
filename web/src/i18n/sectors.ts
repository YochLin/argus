import type { Lang } from "./types";

// sectorNamesZh translates Finnhub's US finnhubIndustry classification (an
// open-ended English string, e.g. "Technology", "Semiconductors") for the
// Money Flow page's sector groups. TW's own classification (FinMind's
// industry_category) already comes back in Chinese, so this only matters
// for the US market. Falls back to the original English string when
// unmapped — same "data-sourced proper noun that can't always translate"
// exception the app already makes for ticker company names.
const sectorNamesZh: Record<string, string> = {
  Technology: "科技",
  "Information Technology": "資訊科技",
  Semiconductors: "半導體",
  "Semiconductor Equipment & Materials": "半導體設備與材料",
  Software: "軟體",
  "Software - Infrastructure": "基礎軟體",
  "Software - Application": "應用軟體",
  Hardware: "硬體",
  "Computer Hardware": "電腦硬體",
  "Consumer Electronics": "消費性電子",
  "Electronic Equipment": "電子設備",
  Internet: "網路服務",
  "Internet Content & Information": "網路內容與資訊",
  "Health Care": "醫療保健",
  Healthcare: "醫療保健",
  "Health Care Providers": "醫療服務",
  "Medical Devices": "醫療器材",
  Biotechnology: "生技",
  Pharmaceuticals: "製藥",
  "Life Sciences Tools & Services": "生命科學工具與服務",
  "Financial Services": "金融服務",
  Financials: "金融",
  Banks: "銀行",
  "Diversified Financial Services": "多元化金融",
  "Consumer Finance": "消費金融",
  Insurance: "保險",
  "Capital Markets": "資本市場",
  "Consumer Cyclical": "非必需消費",
  "Consumer Defensive": "必需消費",
  Retail: "零售",
  "Internet Retail": "網路零售",
  "Specialty Retail": "專業零售",
  "Auto Manufacturers": "汽車製造",
  Automobiles: "汽車",
  "Auto Parts": "汽車零件",
  Apparel: "服飾",
  Beverages: "飲料",
  "Food Products": "食品",
  "Household Products": "家用產品",
  "Packaging & Containers": "包裝",
  Communication: "通訊",
  "Communication Services": "通訊服務",
  Telecommunication: "電信",
  Media: "媒體",
  Entertainment: "娛樂",
  "Hotels, Restaurants & Leisure": "飯店餐飲休閒",
  Airlines: "航空",
  Industrials: "工業",
  "Aerospace & Defense": "航太國防",
  Machinery: "機械",
  Transportation: "運輸",
  "Building Materials": "建材",
  Homebuilding: "營建",
  Chemicals: "化學",
  "Basic Materials": "原物料",
  "Metals & Mining": "金屬與礦業",
  Energy: "能源",
  "Oil & Gas": "石油天然氣",
  Utilities: "公用事業",
  "Real Estate": "不動產",
  REIT: "不動產投資信託",
};

// sectorLabel returns name translated for the given language, falling back
// to the original (English) string when no translation is known.
export function sectorLabel(name: string, lang: Lang): string {
  if (lang !== "zh") return name;
  return sectorNamesZh[name] ?? name;
}
