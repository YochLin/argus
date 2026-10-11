import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { ApiError, fetchAssistant, newAssistantChat, sendAssistant, type AssistantMessage } from "./api";
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
  writable: boolean;
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
  children,
}: {
  dict: Dictionary;
  path: string;
  writable: boolean;
  onUnauthorized: (retry: () => void) => void;
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
      .then((m) => {
        setMessages(m);
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
        .then((added) => {
          topic.current = undefined;
          setMessages((m) => [...m, ...added]);
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
    writable,
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
