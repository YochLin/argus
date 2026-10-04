import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";

export type FlashTone = "ok" | "error";
type Flash = (message: string, tone?: FlashTone) => void;

// A no-op default so a view rendered without the provider (a unit test, an
// ErrorBoundary fallback) can still call it.
const FlashContext = createContext<Flash>(() => {});

// useFlash returns the one shell-level toast: "已封存「X」" after an action
// succeeds, or the server's message in the error tone when one fails — the
// replacement for window.alert, which is unstyled and blocks the page.
export function useFlash(): Flash {
  return useContext(FlashContext);
}

// The design's toast stays 2.6s; an error needs longer to be read.
const FLASH_MS: Record<FlashTone, number> = { ok: 2600, error: 5000 };

export function FlashProvider({ children }: { children: ReactNode }) {
  const [note, setNote] = useState<{ message: string; tone: FlashTone } | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const flash = useCallback<Flash>((message, tone = "ok") => {
    setNote({ message, tone });
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setNote(null), FLASH_MS[tone]);
  }, []);
  useEffect(() => () => clearTimeout(timer.current), []);

  return (
    <FlashContext.Provider value={flash}>
      {children}
      {note && (
        <div className={`flash flash--${note.tone}`} role={note.tone === "error" ? "alert" : "status"}>
          {note.message}
        </div>
      )}
    </FlashContext.Provider>
  );
}
