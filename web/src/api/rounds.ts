import type { Candle, Market } from "./common";
import { getJSON } from "./http";

export interface Transaction {
  id: number;
  date: string;
  ticker: string;
  side: string; // "BUY" | "SELL"
  shares: number;
  price: number;
  fee: number;
  realizedPnL: number;
}

export interface RoundSummary {
  ticker: string;
  start: string;
  end: string; // "" while still open
  open: boolean;
  shares: number;
  realizedPnL: number;
  // Filled by /api/chart only; /api/rounds and rounds older than the chart's
  // 1y candles come back with hasMaeMfe=false (shown as "—").
  maePct: number;
  mfePct: number;
  hasMaeMfe: boolean;
}

export interface Rounds {
  rounds: RoundSummary[];
}

export interface Lesson {
  date: string;
  lesson: string;
}

// ResearchNote is one row of the chart page's notebook card — one upserted
// note per ticker per calendar day (see internal/db/research_notes.go),
// with a fixed-taxonomy Tag and a Pinned flag the card surfaces above the
// chronological list. Search/tag-filter/month-grouping/"load more" are all
// derived client-side from the one fetchResearchNotes(ticker) list.
export interface ResearchNote {
  id: number;
  tag: string;
  text: string;
  pinned: boolean;
  date: string;
}

export interface ResearchNotes {
  notes: ResearchNote[];
}

// NOTE_TAGS is the fixed set of tag chips the compose form offers — must
// stay in sync with internal/web/research_notes.go's researchNoteTags map.
// LEGACY_NOTE_TAGS are the pre-redesign taxonomy (技術面/籌碼面/消息面/其他): the
// server still accepts them and old rows still carry them, so the card keeps
// displaying them, but the compose form no longer offers them.
export const NOTE_TAGS = ["OBSERVATION", "VALUATION", "RISK", "CATALYST"] as const;
export type NoteTag = (typeof NOTE_TAGS)[number];
export const LEGACY_NOTE_TAGS = ["TECHNICAL", "CHIPS", "NEWS", "OTHER"] as const;

export interface RoundDetail {
  ticker: string;
  start: string;
  end: string; // "" while still open
  candles: Candle[];
  trades: Transaction[];
  maePct: number;
  mfePct: number;
  hasMaeMfe: boolean;
  // thesis/lessons (Phase 8 PR4) are attachments. Since Phase 27 P1b the
  // thesis is one overwritable text per round, editable whether the round is
  // open or closed ("" = none written); thesisEdited is false when the text is
  // only the legacy journal's last entry, shown as a default until first save
  // (see internal/web/rounds.go's roundDetailResponse doc).
  thesis: string;
  thesisEdited: boolean;
  lessons: Lesson[];
}

export function fetchRounds(market: Market = "us"): Promise<Rounds> {
  return getJSON<Rounds>(`/api/rounds?market=${market}`);
}

export function fetchRoundDetail(ticker: string, start: string): Promise<RoundDetail> {
  return getJSON<RoundDetail>(
    `/api/round-detail?ticker=${encodeURIComponent(ticker)}&start=${encodeURIComponent(start)}`,
  );
}

export function fetchResearchNotes(ticker: string): Promise<ResearchNotes> {
  return getJSON<ResearchNotes>(`/api/research-notes?ticker=${encodeURIComponent(ticker)}`);
}
