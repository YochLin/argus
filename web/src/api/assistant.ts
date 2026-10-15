import { ApiError } from "./http";
import { movesNetWorth } from "./netWorth";

// AssistantMessage mirrors internal/web/wealth_assistant.go's assistantMessage
// (Phase 28): "div" is the "AI started over" divider; action "trades" asks
// for a 前往交易記錄 button under an ai turn.
export interface AssistantMessage {
  id: number;
  role: "user" | "ai" | "sys" | "div";
  text: string;
  action?: "trades";
  // draftIds are the drafts an ai turn added (its chips); result is a sys
  // message's write outcome.
  draftIds?: number[];
  result?: ApplyResult;
  createdAt: string;
}

export interface ApplyResult {
  applied: number;
  kept: number;
  // kinds written, in order (absent on results stored before they existed)
  kinds?: AssistantDraft["kind"][];
}

// AssetDraftPayload mirrors internal/assets.AssetDraft — snake_case, the
// same JSON the model's propose_asset writes.
export interface AssetDraftPayload {
  type?: string;
  name?: string;
  group?: string;
  currency?: string;
  value?: number;
  date?: string;
  venue?: string;
  bank?: string;
  account_note?: string;
  lender?: string;
  rate_pct?: number;
  original_principal?: number;
  remaining_months?: number;
  fund_code?: string;
  fund_platform?: string;
  fund_monthly_amount?: number;
  fund_next_contribution_date?: string;
}

export interface BalanceDraftPayload {
  value?: number;
  date?: string;
}

// CashflowDraftPayload mirrors internal/assets.CashflowDraft.
export interface CashflowDraftPayload {
  direction?: string;
  name?: string;
  amount?: number;
  currency?: string;
  day_of_month?: number;
  category?: string;
}

// InsuranceDraftPayload mirrors internal/assets.InsuranceDraft (always TWD).
export interface InsuranceDraftPayload {
  name?: string;
  insurer?: string;
  insured?: string;
  kind?: string;
  amount?: number;
  annual_premium?: number;
  premium_years?: number;
}

// ProfileDraftPayload mirrors internal/assets.ProfileDraft: only the fields
// given change. retirement_age and retirement_monthly_spend go together.
export interface ProfileDraftPayload {
  annual_salary?: number;
  birth_year?: number;
  retirement_monthly_contribution?: number;
  dependents?: number;
  youngest_child_age?: number;
  spouse_has_income?: boolean;
  retirement_age?: number;
  retirement_monthly_spend?: number;
}

// GoalDraftPayload mirrors internal/assets.GoalDraft (a general goal, TWD).
export interface GoalDraftPayload {
  name?: string;
  target_amount?: number;
  target_year?: number;
  saved_amount?: number;
  monthly_contribution?: number;
  note?: string;
}

export type DraftPayload =
  | AssetDraftPayload
  | BalanceDraftPayload
  | CashflowDraftPayload
  | InsuranceDraftPayload
  | ProfileDraftPayload
  | GoalDraftPayload;

// AssistantDraft mirrors wealth_drafts.go's assistantDraft: status and
// missing are computed by the server on every read. current is a balance
// draft's asset as it is now.
export type AssistantDraft = {
  id: number;
  targetId?: number;
  status: "ok" | "pending" | "error";
  missing: string[];
  error?: string;
  // errors: error's rule problems as codes (i18n asstErr<code>); absent for
  // a failed write, whose error is shown as is.
  errors?: { code: string; args?: string[] }[];
  // failed: error is why the last write failed, not a rule the draft breaks.
  failed?: boolean;
  current?: { name: string; type: string; currency: string; value: number | null };
  createdAt: string;
} & (
  | { kind: "asset"; payload: AssetDraftPayload }
  | { kind: "balance"; payload: BalanceDraftPayload }
  | { kind: "cashflow"; payload: CashflowDraftPayload }
  | { kind: "insurance"; payload: InsuranceDraftPayload }
  | { kind: "profile"; payload: ProfileDraftPayload }
  | { kind: "goal"; payload: GoalDraftPayload }
);

export interface AssistantData {
  messages: AssistantMessage[];
  drafts: AssistantDraft[];
  result?: ApplyResult;
}

// These bypass getJSON/postJSON's mock fallback on purpose: a chat that
// "succeeds" with mock data would show the user an answer nobody gave, and
// a draft "saved" to a mock would be lost. need names the field a call must
// return.
async function assistantCall(url: string, body: unknown, need: "messages" | "drafts"): Promise<AssistantData> {
  const init: RequestInit | undefined =
    body === undefined ? undefined : { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
  const res = await fetch(url, init);
  const data = (await res.json().catch(() => ({}))) as Partial<AssistantData> & { error?: string };
  if (!res.ok || !Array.isArray(data[need])) {
    throw new ApiError(res.status, data.error ?? `HTTP ${res.status}`);
  }
  return { messages: data.messages ?? [], drafts: data.drafts ?? [], result: data.result };
}

export function fetchAssistant(): Promise<AssistantData> {
  return assistantCall("/api/wealth/assistant", undefined, "messages");
}

// sendAssistant returns only the messages this turn added, and every draft.
export function sendAssistant(text: string, topic?: string): Promise<AssistantData> {
  return assistantCall("/api/wealth/assistant/chat", { text, topic }, "messages");
}

export function newAssistantChat(): Promise<AssistantData> {
  return assistantCall("/api/wealth/assistant/new", {}, "messages");
}

export function updateAssistantDraft(id: number, payload: DraftPayload): Promise<AssistantData> {
  return assistantCall("/api/wealth/assistant/drafts/update", { id, payload }, "drafts");
}

export function deleteAssistantDraft(id: number): Promise<AssistantData> {
  return assistantCall("/api/wealth/assistant/drafts/delete", { id }, "drafts");
}

export function clearAssistantDrafts(): Promise<AssistantData> {
  return assistantCall("/api/wealth/assistant/drafts/clear", {}, "drafts");
}

// applyAssistantDrafts writes every ready draft; messages is the sys note.
export function applyAssistantDrafts(): Promise<AssistantData> {
  return movesNetWorth(assistantCall("/api/wealth/assistant/apply", {}, "drafts"));
}
