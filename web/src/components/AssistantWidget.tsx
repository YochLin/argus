import { useEffect, useRef, useState, type ReactNode } from "react";
import { useAssistant, useIsMobile, type Assistant } from "../assistant";
import { useFlash } from "../flash";
import type { AssistantMessage } from "../api";
import type { Dictionary } from "../i18n";
import { DraftsPane, draftChip } from "./AssistantDrafts";
import { ConfirmDialog } from "./ConfirmDialog";

// Phase 28's wealth assistant (Argus Wealth Assistant.dc.html): a launcher
// bubble, a floating panel, or a full page in place of the /w/* page body.
// The drafts column lives in AssistantDrafts.tsx.

export function AiIcon({ size = 12 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" stroke="currentColor" strokeWidth={1.5} fill="none" aria-hidden="true">
      <path d="M8 2.3l1.27 3.4 3.4 1.27-3.4 1.27L8 11.67 6.73 8.27 3.33 7l3.4-1.27Z" fill="currentColor" stroke="none" />
      <path d="M12.33 10.33l.53 1.34 1.34.53-1.34.53-.53 1.34-.53-1.34-1.34-.53 1.34-.53Z" fill="currentColor" stroke="none" opacity={0.85} />
    </svg>
  );
}

function Dots({ light }: { light?: boolean }) {
  return (
    <span className={`asst-dots${light ? " asst-dots--light" : ""}`} aria-hidden="true">
      <span />
      <span />
      <span />
    </span>
  );
}

// AssistantFloat is the widget's overlay half: the bubble, or the panel. The
// page layout is rendered by App in place of the page body instead.
export function AssistantFloat({ dict, onNavigate }: { dict: Dictionary; onNavigate: (path: string) => void }) {
  const a = useAssistant();
  const mobile = useIsMobile();
  if (!a) return null;
  if (a.mode === "bubble") return <Launcher dict={dict} a={a} />;
  if (a.mode === "page" && !mobile) return null;
  return (
    <div className="asst-panel" role="dialog" aria-label={dict.asstTitle}>
      <AssistantBody dict={dict} a={a} compact mobile={mobile} onNavigate={onNavigate} />
    </div>
  );
}

// AssistantPage is the full-page layout (mode "page" on a wide screen).
export function AssistantPage({ dict, onNavigate }: { dict: Dictionary; onNavigate: (path: string) => void }) {
  const a = useAssistant();
  if (!a) return null;
  return <AssistantBody dict={dict} a={a} compact={false} mobile={false} onNavigate={onNavigate} />;
}

// AssistantPageSwitch shows the full-page assistant in place of a /w/* page's
// body while the assistant is in page mode (a phone keeps the panel).
export function AssistantPageSwitch({ dict, onNavigate, children }: { dict: Dictionary; onNavigate: (path: string) => void; children: ReactNode }) {
  const a = useAssistant();
  const mobile = useIsMobile();
  if (a?.mode === "page" && !mobile) return <AssistantPage dict={dict} onNavigate={onNavigate} />;
  return <>{children}</>;
}

function Launcher({ dict, a }: { dict: Dictionary; a: Assistant }) {
  const busy = a.pending !== null;
  const n = a.drafts.length;
  const note = busy
    ? dict.asstNoteBusy
    : a.failed
      ? dict.asstNoteErr
      : n
        ? dict.asstNoteDrafts.replace("%s", String(n))
        : dict.asstChatBuild;
  const tone = busy ? " busy" : a.failed ? " err" : n ? " drafts" : "";
  return (
    <div className="asst-launcher asst-vars">
      <button className={`asst-launcher-note${tone}`} onClick={() => a.open()}>
        {note}
      </button>
      <button className="asst-launcher-btn" onClick={() => a.open()} aria-label={dict.asstTitle} title={dict.asstTitle}>
        {busy ? <Dots light /> : <AiIcon size={24} />}
        {n > 0 && <span className="asst-launcher-badge">{n}</span>}
      </button>
    </div>
  );
}

function AssistantBody({
  dict,
  a,
  compact,
  mobile,
  onNavigate,
}: {
  dict: Dictionary;
  a: Assistant;
  compact: boolean;
  mobile: boolean;
  onNavigate: (path: string) => void;
}) {
  const [confirmNew, setConfirmNew] = useState(false);
  const { load, loaded, tab, setTab } = a;
  useEffect(() => {
    if (!loaded) load();
  }, [loaded, load]);

  const askNew = () => setConfirmNew(true);
  const chat = <ChatPane dict={dict} a={a} compact={compact} onNewChat={askNew} onNavigate={onNavigate} />;
  const drafts = <DraftsPane dict={dict} a={a} compact={compact} onNavigate={onNavigate} />;
  const unseen = tab === "chat" ? a.newIds.length : 0;

  return (
    <div className={`asst${compact ? " asst--compact" : " asst--page"}`}>
      {compact ? (
        <>
          <div className="asst-bar">
            <span className="asst-bar-icon">
              <AiIcon size={14} />
            </span>
            <span className="asst-bar-title">{dict.asstTitle}</span>
            <button className="asst-btn-ghost asst-bar-new" onClick={askNew}>
              <RestartIcon />
              {dict.asstNewChat}
            </button>
            {!mobile && (
              <button className="asst-icon-btn" onClick={() => a.setMode("page")} aria-label={dict.asstExpand} title={dict.asstExpand}>
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6">
                  <path d="M9.5 2.5h4v4M13.5 2.5 9 7M6.5 13.5h-4v-4M2.5 13.5 7 9" />
                </svg>
              </button>
            )}
            <button className="asst-icon-btn" onClick={() => a.setMode("bubble")} aria-label={dict.asstMinimize} title={dict.asstMinimize}>
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6">
                <path d="M3.5 6 8 10.5 12.5 6" />
              </svg>
            </button>
          </div>
          <div className="asst-tabs" role="tablist">
            <button role="tab" aria-selected={tab === "chat"} className={tab === "chat" ? "on" : ""} onClick={() => setTab("chat")}>
              {dict.asstChat}
            </button>
            <button role="tab" aria-selected={tab === "drafts"} className={tab === "drafts" ? "on" : ""} onClick={() => setTab("drafts")}>
              {dict.asstTabDrafts.replace("%s", String(a.drafts.length))}
              {unseen > 0 && <span className="asst-unseen">{dict.asstUnseen.replace("%s", String(unseen))}</span>}
            </button>
          </div>
          {!a.writable && <RoBanner dict={dict} />}
          {tab === "chat" ? chat : drafts}
        </>
      ) : (
        <>
          <div className="asst-head">
            <div className="asst-head-text">
              <span className="asst-label">{dict.asstTitle}</span>
              <span className="asst-sub">{dict.asstPageSub}</span>
            </div>
            <button className="asst-btn-ghost" onClick={() => a.setMode("panel")}>
              <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6">
                <path d="M13.5 6.5h-4v-4M9.5 6.5 14 2M2.5 9.5h4v4M6.5 9.5 2 14" />
              </svg>
              {dict.asstCollapse}
            </button>
          </div>
          {!a.writable && <RoBanner dict={dict} />}
          <div className="asst-grid">
            <div className="asst-card">{chat}</div>
            <div className="asst-card">{drafts}</div>
          </div>
        </>
      )}
      {confirmNew && (
        <ConfirmDialog
          title={dict.asstNewChatQ}
          body={a.drafts.length ? dict.asstNewChatBodyDrafts.replace("%s", String(a.drafts.length)) : dict.asstNewChatBody}
          okLabel={dict.asstNewChat}
          cancelLabel={dict.cancel}
          onCancel={() => setConfirmNew(false)}
          onConfirm={() => {
            setConfirmNew(false);
            a.newChat();
          }}
        />
      )}
    </div>
  );
}

function RestartIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 16 16" stroke="currentColor" strokeWidth="1.6" fill="none">
      <path d="M2.5 8a5.5 5.5 0 1 0 1.6-3.9" />
      <polyline points="2.5,2 2.5,5 5.5,5" />
    </svg>
  );
}

function RoBanner({ dict }: { dict: Dictionary }) {
  return (
    <div className="asst-ro">
      <span className="asst-ro-tag">
        <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6">
          <rect x="3" y="7" width="10" height="7" rx="1.5" />
          <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
        </svg>
        {dict.asstRoTag}
      </span>
      <span className="asst-ro-msg">{dict.asstRoMsg}</span>
    </div>
  );
}

function AiLabel() {
  return (
    <span className="asst-ai-label">
      <AiIcon />
      ARGUS AI
    </span>
  );
}

function ChatPane({
  dict,
  a,
  compact,
  onNewChat,
  onNavigate,
}: {
  dict: Dictionary;
  a: Assistant;
  compact: boolean;
  onNewChat: () => void;
  onNavigate: (path: string) => void;
}) {
  const flash = useFlash();
  const list = useRef<HTMLDivElement>(null);
  const busy = a.pending !== null;
  const sendOff = !a.writable || busy || !a.input.trim();

  useEffect(() => {
    const el = list.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [a.messages.length, a.pending, a.failed]);

  const roBlock = () => flash(dict.asstRoToast, "error");
  const showSugs = a.messages.length === 0 && !busy && !a.failed;

  return (
    <div className="asst-pane">
      {!compact && (
        <div className="asst-pane-head">
          <span className="asst-label">{dict.asstChat}</span>
          <button className="asst-btn-ghost asst-pane-head-btn" onClick={onNewChat}>
            <RestartIcon />
            {dict.asstNewChat}
          </button>
        </div>
      )}
      <div className="asst-msgs" ref={list}>
        <div className="asst-ai">
          <AiLabel />
          <span className="asst-ai-text">{compact ? dict.asstGreetCompact : dict.asstGreet}</span>
        </div>
        {a.messages.map((m) =>
          m.role === "user" ? (
            <div key={m.id} className="asst-me">
              {m.text}
            </div>
          ) : m.role === "div" ? (
            <div key={m.id} className="asst-div">
              <span />
              <span className="asst-div-text">{dict.asstRestart}</span>
              <span />
            </div>
          ) : m.role === "sys" ? (
            <div key={m.id} className="asst-sys">
              ✓ {sysText(dict, m)}
            </div>
          ) : (
            <div key={m.id} className="asst-ai">
              <AiLabel />
              <span className="asst-ai-text">{m.text}</span>
              <Chips dict={dict} a={a} ids={m.draftIds} />
              {m.action === "trades" && (
                <button className="asst-go" onClick={() => onNavigate("/")}>
                  {dict.asstGoTrades}
                </button>
              )}
            </div>
          ),
        )}
        {busy && (
          <>
            <div className="asst-me">{a.pending}</div>
            <div className="asst-ai">
              <AiLabel />
              <div className="asst-think">
                <Dots />
                <span className="asst-think-label">{dict.asstThink}</span>
                <span className="asst-think-sec">{dict.asstSec.replace("%s", String(a.elapsed))}</span>
              </div>
              <span className="asst-note">{dict.asstThinkNote}</span>
            </div>
          </>
        )}
        {a.failed && (
          <div className="asst-err" role="alert">
            <span className="asst-err-title">⚠ {dict.asstErrTitle}</span>
            <span className="asst-err-body">{dict.asstErrBody}</span>
            <button className="asst-err-retry" onClick={() => (a.writable ? a.send(a.input) : roBlock())}>
              {dict.asstRetry}
            </button>
          </div>
        )}
        {showSugs && (
          <div className="asst-sugs">
            <span className="asst-sugs-title">{dict.asstSugTitle}</span>
            <div className="asst-sugs-list">
              {dict.asstSugs.split("|").map((s) => (
                <button key={s} className={`asst-sug${a.writable ? "" : " off"}`} onClick={() => a.send(s)}>
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
      <div className="asst-input">
        <div className={`asst-input-box${busy ? " busy" : a.failed ? " err" : ""}`}>
          <textarea
            value={a.input}
            rows={2}
            placeholder={!a.writable ? dict.asstInputPhRo : busy ? dict.asstInputPhBusy : dict.asstInputPh}
            onChange={(e) => a.setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                a.send(a.input);
              }
            }}
          />
          <button className={`asst-send${sendOff ? " off" : ""}`} aria-disabled={sendOff} onClick={() => a.send(a.input)}>
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8">
              <path d="M2 8h11M9 4l4 4-4 4" />
            </svg>
            {dict.asstSend}
          </button>
        </div>
        <span className="asst-hint">{busy ? dict.asstHintBusy : a.failed ? dict.asstHintErr : dict.asstHintIdle}</span>
      </div>
    </div>
  );
}

function sysText(dict: Dictionary, m: AssistantMessage): string {
  const r = m.result;
  if (!r) return m.text;
  return r.kept
    ? dict.asstSysApplied.replace("%n", String(r.applied)).replace("%m", String(r.kept))
    : dict.asstSysAppliedAll.replace("%s", String(r.applied));
}

// Chips are an ai turn's "已加入草稿：…" links — only for drafts still there
// (one written or deleted since has nothing to point at).
function Chips({ dict, a, ids }: { dict: Dictionary; a: Assistant; ids?: number[] }) {
  const live = (ids ?? []).flatMap((id) => a.drafts.filter((d) => d.id === id));
  if (!live.length) return null;
  return (
    <div className="asst-chips">
      {live.map((d) => (
        <button key={d.id} className="asst-chip" onClick={() => a.goDraft(d.id)}>
          <span className="asst-chip-plus">＋</span>
          <span className="asst-chip-pre">{dict.asstAddedTo}</span>
          <span>{draftChip(dict, d)}</span>
          <span className="asst-chip-arrow">↓</span>
        </button>
      ))}
    </div>
  );
}
