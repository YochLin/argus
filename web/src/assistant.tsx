import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import {
  ApiError,
  applyAssistantDrafts,
  clearAssistantDrafts,
  deleteAssistantDraft,
  fetchAssistant,
  newAssistantChat,
  sendAssistant,
  updateAssistantDraft,
  type ApplyResult,
  type AssistantData,
  type AssistantDraft,
  type AssistantMessage,
  type DraftPayload,
} from "./api";
import { useFlash } from "./flash";
import type { Dictionary } from "./i18n";

// The wealth assistant's state (Phase 28, docs/phase-28-wealth-ai-chat.md
// §10). It lives above the router, not in a page, because the design makes
// the assistant a floating widget on every /w/* page: switching pages while
// a reply is on its way must not drop it.

export type AssistantMode = "bubble" | "panel" | "page";

export interface Assistant {
  mode: AssistantMode;
  setMode: (m: AssistantMode) => void;
  // open is what an entry point calls: show the panel, and send topic (the
  // page it was opened from) with the next message so the AI starts there.
  open: (topic?: string) => void;
  messages: AssistantMessage[];
  loaded: boolean;
  load: () => void;
  // pending is the message on its way: shown under the conversation with a
  // thinking bubble until the reply lands (or back in the input on failure).
  pending: string | null;
  elapsed: number;
  failed: boolean;
  input: string;
  setInput: (s: string) => void;
  send: (text: string) => void;
  newChat: () => void;
  // forget drops what this page holds after the server emptied the wealth
  // account (the settings page's reset): no conversation, no drafts.
  forget: () => void;
  writable: boolean;

  // Drafts (B2). newIds are the drafts the last turn added (the cards' 新
  // tag and the phone tab's +n), cleared once the drafts tab is opened;
  // applied is the last write's outcome, shown as the result card until the
  // next turn or clear. focusId is the card a chip just pointed at.
  drafts: AssistantDraft[];
  newIds: number[];
  applied: ApplyResult | null;
  tab: "chat" | "drafts";
  setTab: (t: "chat" | "drafts") => void;
  focusId: number | null;
  goDraft: (id: number) => void;
  saveDraft: (id: number, payload: DraftPayload) => Promise<boolean>;
  deleteDraft: (id: number) => void;
  clearDrafts: () => void;
  applyDrafts: () => void;
}

const AssistantContext = createContext<Assistant | null>(null);

// useAssistant is null outside the provider — WealthEmptyCard checks that to
// hide its chat button in a test that renders it bare.
export function useAssistant(): Assistant | null {
  return useContext(AssistantContext);
}

const MODE_KEY = "argus.assistant.mode";

function loadMode(): AssistantMode {
  try {
    const m = localStorage.getItem(MODE_KEY);
    if (m === "panel" || m === "page") return m;
  } catch {
    // per-browser convenience only
  }
  return "bubble";
}

export function AssistantProvider({
  dict,
  path,
  writable,
  onUnauthorized,
  onApplied,
  children,
}: {
  dict: Dictionary;
  path: string;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
  // onApplied: drafts were written, so the page behind the assistant is stale.
  onApplied: () => void;
  children: ReactNode;
}) {
  const flash = useFlash();
  const [mode, setModeState] = useState<AssistantMode>(loadMode);
  const [messages, setMessages] = useState<AssistantMessage[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [pending, setPending] = useState<string | null>(null);
  const [elapsed, setElapsed] = useState(0);
  const [failed, setFailed] = useState(false);
  const [input, setInput] = useState("");
  const [drafts, setDrafts] = useState<AssistantDraft[]>([]);
  const [newIds, setNewIds] = useState<number[]>([]);
  const [applied, setApplied] = useState<ApplyResult | null>(null);
  const [tab, setTabState] = useState<"chat" | "drafts">("chat");
  const [focusId, setFocusId] = useState<number | null>(null);
  const focusTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const topic = useRef<string | undefined>(undefined);
  const loading = useRef(false);

  const setMode = useCallback((m: AssistantMode) => {
    setModeState(m);
    try {
      localStorage.setItem(MODE_KEY, m);
    } catch {
      // per-browser convenience only
    }
  }, []);

  // The full-page layout replaces the page body, so leaving for another page
  // folds it back into the panel rather than keeping the user stuck on it.
  const lastPath = useRef(path);
  useEffect(() => {
    if (path !== lastPath.current && mode === "page") setMode("panel");
    lastPath.current = path;
  }, [path, mode, setMode]);

  const load = useCallback(() => {
    if (loading.current) return;
    loading.current = true;
    fetchAssistant()
      .then((d) => {
        setMessages(d.messages);
        setDrafts(d.drafts);
        setLoaded(true);
      })
      .catch(() => {
        loading.current = false; // try again next open
      });
  }, []);

  useEffect(() => {
    if (pending === null) return;
    setElapsed(0);
    const iv = setInterval(() => setElapsed((n) => n + 1), 1000);
    return () => clearInterval(iv);
  }, [pending]);

  const send = useCallback(
    (raw: string) => {
      const text = raw.trim();
      if (!writable) {
        flash(dict.asstRoToast, "error");
        return;
      }
      if (!text || pending !== null) return;
      setPending(text);
      setInput("");
      setFailed(false);
      sendAssistant(text, topic.current)
        .then((d) => {
          topic.current = undefined;
          setMessages((m) => [...m, ...d.messages]);
          setDrafts(d.drafts);
          setNewIds(d.messages.flatMap((m) => m.draftIds ?? []));
          setApplied(null);
        })
        .catch((e) => {
          // The design's error state: the message isn't kept, the text goes
          // back in the box for a retry.
          setInput(text);
          if (e instanceof ApiError && e.status === 401) {
            onUnauthorized(() => send(text));
          } else if (e instanceof ApiError && e.status === 409) {
            flash(dict.asstStillAnswering, "error");
          } else {
            setFailed(true);
          }
        })
        .finally(() => setPending(null));
    },
    [writable, pending, dict, flash, onUnauthorized],
  );

  const newChat = useCallback(() => {
    newAssistantChat()
      .then(() => {
        setMessages([]);
        setFailed(false);
        topic.current = undefined;
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) onUnauthorized(() => newChat());
        else flash(e instanceof ApiError && e.status === 409 ? dict.asstStillAnswering : dict.error, "error");
      });
  }, [dict, flash, onUnauthorized]);

  const open = useCallback(
    (t?: string) => {
      topic.current = t;
      if (mode === "bubble") setMode("panel");
    },
    [mode, setMode],
  );

  const setTab = useCallback((t: "chat" | "drafts") => {
    setTabState(t);
    if (t === "drafts") setNewIds([]);
  }, []);

  // goDraft is a chip's click: show the drafts and outline that card for a
  // moment (DraftsPane scrolls to it). Unlike opening the tab, it keeps the
  // 新 tags — the design's goDraft does too.
  const goDraft = useCallback((id: number) => {
    setTabState("drafts");
    setFocusId(id);
    clearTimeout(focusTimer.current);
    focusTimer.current = setTimeout(() => setFocusId(null), 1800);
  }, []);

  // draftCall runs one drafts write: read-only and the shared errors are
  // handled here, and the server's list replaces ours. It resolves whether
  // the call went through.
  function draftCall(call: () => Promise<AssistantData>, after?: (d: AssistantData) => void): Promise<boolean> {
    if (!writable) {
      flash(dict.asstRoToast, "error");
      return Promise.resolve(false);
    }
    return call()
      .then((d) => {
        setDrafts(d.drafts);
        after?.(d);
        return true;
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) onUnauthorized(() => void draftCall(call, after));
        else if (e instanceof ApiError && e.status === 409) flash(dict.asstStillAnswering, "error");
        else flash(e instanceof ApiError ? e.message : dict.error, "error");
        return false;
      });
  }

  const value: Assistant = {
    mode,
    setMode,
    open,
    messages,
    loaded,
    load,
    pending,
    elapsed,
    failed,
    input,
    setInput,
    send,
    newChat,
    forget: () => {
      setMessages([]);
      setDrafts([]);
      setNewIds([]);
      setApplied(null);
      setFailed(false);
      setInput("");
      topic.current = undefined;
    },
    writable,
    drafts,
    newIds,
    applied,
    tab,
    setTab,
    focusId,
    goDraft,
    saveDraft: (id, payload) => draftCall(() => updateAssistantDraft(id, payload)),
    deleteDraft: (id) => void draftCall(() => deleteAssistantDraft(id)),
    clearDrafts: () => void draftCall(clearAssistantDrafts, () => setApplied(null)),
    applyDrafts: () =>
      void draftCall(applyAssistantDrafts, (d) => {
        setMessages((m) => [...m, ...d.messages]);
        setNewIds([]);
        if (d.result) setApplied(d.result);
        if (d.result?.applied) onApplied();
      }),
  };
  return <AssistantContext.Provider value={value}>{children}</AssistantContext.Provider>;
}

// useIsMobile tracks the breakpoint below which the panel and the page both
// become one full-screen column with 對話/草稿 tabs (the design's `mobile`).
export function useIsMobile(): boolean {
  const query = "(max-width: 640px)";
  const [mobile, setMobile] = useState(() => typeof window !== "undefined" && !!window.matchMedia?.(query).matches);
  useEffect(() => {
    const mq = window.matchMedia?.(query);
    if (!mq) return;
    const on = () => setMobile(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return mobile;
}
