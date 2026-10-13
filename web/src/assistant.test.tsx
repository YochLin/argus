import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor, act } from "@testing-library/react";
import { AssistantProvider } from "./assistant";
import { AssistantFloat } from "./components/AssistantWidget";
import { WealthEmptyCard } from "./components/WealthOnboarding";
import { FlashProvider } from "./flash";
import { getDictionary } from "./i18n";
import * as api from "./api";

vi.mock("./api", async (importOriginal) => {
  const real = await importOriginal<typeof import("./api")>();
  return {
    ...real,
    fetchAssistant: vi.fn(),
    sendAssistant: vi.fn(),
    newAssistantChat: vi.fn(),
    updateAssistantDraft: vi.fn(),
    deleteAssistantDraft: vi.fn(),
    clearAssistantDrafts: vi.fn(),
    applyAssistantDrafts: vi.fn(),
  };
});

const m = vi.mocked(api);
const dict = getDictionary("zh");
// Testing Library collapses whitespace in the node's text, not in a string
// matcher, so the two-line greeting is matched with its newline collapsed.
const greeting = dict.asstGreetCompact.replace(/\s+/g, " ");

const msg = (id: number, role: api.AssistantMessage["role"], text: string, action?: "trades"): api.AssistantMessage => ({
  id,
  role,
  text,
  action,
  createdAt: "2026-10-11T10:00:00+08:00",
});

const data = (messages: api.AssistantMessage[], drafts: api.AssistantDraft[] = [], result?: api.ApplyResult): api.AssistantData => ({
  messages,
  drafts,
  result,
});

const deposit: api.AssistantDraft = {
  id: 1,
  kind: "asset",
  payload: { type: "deposit", name: "國泰活存", group: "liquid", currency: "TWD", value: 800000 },
  status: "ok",
  missing: [],
  createdAt: "2026-10-12T10:00:00+08:00",
};
const loan: api.AssistantDraft = {
  id: 2,
  kind: "asset",
  payload: { type: "loan", name: "房貸", group: "hard", currency: "TWD", rate_pct: 2.1 },
  status: "pending",
  missing: ["value"],
  createdAt: "2026-10-12T10:00:00+08:00",
};
const balance: api.AssistantDraft = {
  id: 3,
  kind: "balance",
  targetId: 9,
  payload: { value: 150000 },
  status: "ok",
  missing: [],
  current: { name: "玉山活存", type: "deposit", currency: "TWD", value: 120000 },
  createdAt: "2026-10-12T10:00:00+08:00",
};

function renderWidget({ writable = true, onNavigate = vi.fn(), withCard = false } = {}) {
  render(
    <FlashProvider>
      <AssistantProvider dict={dict} path="/w" writable={writable} onUnauthorized={vi.fn()}>
        {withCard && <WealthEmptyCard dict={dict} line="empty" onAdd={vi.fn()} topic="insure" />}
        <AssistantFloat dict={dict} onNavigate={onNavigate} />
      </AssistantProvider>
    </FlashProvider>,
  );
  return { onNavigate };
}

function type(text: string) {
  const box = screen.getByPlaceholderText(dict.asstInputPh);
  fireEvent.change(box, { target: { value: text } });
  fireEvent.keyDown(box, { key: "Enter" });
}

// Same reason as WealthOnboarding.test.tsx: Node's own localStorage shadows
// the test DOM's, so the mode is kept in an in-memory one.
function memoryStorage(): Storage {
  const store = new Map<string, string>();
  return {
    getItem: (k) => store.get(k) ?? null,
    setItem: (k, v) => void store.set(k, String(v)),
    removeItem: (k) => void store.delete(k),
    clear: () => store.clear(),
    key: (i) => [...store.keys()][i] ?? null,
    get length() {
      return store.size;
    },
  };
}

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  m.fetchAssistant.mockResolvedValue(data([]));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("wealth assistant", () => {
  it("opens from the bubble on the greeting and suggestions, and loads the stored conversation", async () => {
    renderWidget();
    fireEvent.click(screen.getByRole("button", { name: dict.asstTitle }));
    expect(await screen.findByText(greeting)).toBeTruthy();
    expect(screen.getByText(dict.asstSugs.split("|")[0])).toBeTruthy();
    expect(m.fetchAssistant).toHaveBeenCalledTimes(1);
  });

  it("shows the message while thinking, then the reply", async () => {
    let answer!: (v: api.AssistantData) => void;
    m.sendAssistant.mockReturnValue(new Promise((r) => (answer = r)));
    localStorage.setItem("argus.assistant.mode", "panel");
    renderWidget();

    type("我有房貸");
    expect(screen.getByText("我有房貸")).toBeTruthy();
    expect(screen.getByText(dict.asstThink)).toBeTruthy();
    expect(screen.getByPlaceholderText(dict.asstInputPhBusy)).toBeTruthy();

    await act(async () => answer(data([msg(1, "user", "我有房貸"), msg(2, "ai", "利率多少？")])));
    expect(screen.getByText("利率多少？")).toBeTruthy();
    expect(screen.queryByText(dict.asstThink)).toBeNull();
    expect(m.sendAssistant).toHaveBeenCalledWith("我有房貸", undefined);
  });

  it("on failure drops the message and puts the text back in the box", async () => {
    m.sendAssistant.mockRejectedValue(new api.ApiError(502, "down"));
    localStorage.setItem("argus.assistant.mode", "panel");
    renderWidget();

    type("我有房貸");
    expect(await screen.findByText(`⚠ ${dict.asstErrTitle}`)).toBeTruthy();
    expect((screen.getByPlaceholderText(dict.asstInputPh) as HTMLTextAreaElement).value).toBe("我有房貸");
    expect(document.querySelectorAll(".asst-me")).toHaveLength(0);
  });

  it("read-only: sending shows the toast and never calls the server", async () => {
    localStorage.setItem("argus.assistant.mode", "panel");
    renderWidget({ writable: false });
    expect(screen.getByText(dict.asstRoMsg)).toBeTruthy();
    fireEvent.click(screen.getByText(dict.asstSugs.split("|")[0]));
    expect(await screen.findByText(dict.asstRoToast)).toBeTruthy();
    expect(m.sendAssistant).not.toHaveBeenCalled();
  });

  it("renders the stored divider and the trades button", async () => {
    m.fetchAssistant.mockResolvedValue(data([msg(1, "user", "我有台積電"), msg(2, "div", ""), msg(3, "ai", "股票不在這裡記錄。", "trades")]));
    localStorage.setItem("argus.assistant.mode", "panel");
    const { onNavigate } = renderWidget();
    expect(await screen.findByText(dict.asstRestart)).toBeTruthy();
    fireEvent.click(screen.getByText(dict.asstGoTrades));
    expect(onNavigate).toHaveBeenCalledWith("/");
  });

  it("an empty card's entry opens the panel and sends its topic with the next message", async () => {
    m.sendAssistant.mockResolvedValue(data([msg(1, "user", "hi"), msg(2, "ai", "ok")]));
    renderWidget({ withCard: true });
    fireEvent.click(screen.getByText(dict.asstChatBuild, { selector: ".wealth-empty-chat" }));
    await screen.findByText(greeting);
    type("hi");
    await waitFor(() => expect(m.sendAssistant).toHaveBeenCalledWith("hi", "insure"));
  });
});

describe("wealth assistant drafts", () => {
  it("the bubble says how many drafts wait, with a badge", async () => {
    m.fetchAssistant.mockResolvedValue(data([], [deposit, loan]));
    renderWidget();
    // The bubble doesn't load by itself; opening it does.
    fireEvent.click(screen.getByRole("button", { name: dict.asstTitle }));
    await screen.findByText(dict.asstTabDrafts.replace("%s", "2"));
    fireEvent.click(screen.getByRole("button", { name: dict.asstMinimize }));
    expect(screen.getByText(dict.asstNoteDrafts.replace("%s", "2"))).toBeTruthy();
    expect(document.querySelector(".asst-launcher-badge")?.textContent).toBe("2");
  });

  it("a turn's chips open the drafts tab; cards show ready, missing and the balance change", async () => {
    m.sendAssistant.mockResolvedValue(data([msg(1, "user", "我有房貸"), { ...msg(2, "ai", "已加入草稿"), draftIds: [1, 2] }], [deposit, loan, balance]));
    localStorage.setItem("argus.assistant.mode", "panel");
    renderWidget();

    type("我有房貸");
    const chip = await screen.findByText("國泰活存 NT$800,000");
    expect(screen.getByText(dict.asstUnseen.replace("%s", "2"))).toBeTruthy();
    fireEvent.click(chip);

    expect(screen.getByText(dict.asstStPending)).toBeTruthy();
    expect(screen.getAllByText(dict.asstNewTag)).toHaveLength(2);
    expect(screen.getByText(`? ${dict.asstPendingVal}`)).toBeTruthy();
    expect(screen.getByText("NT$120,000")).toBeTruthy();
    expect(screen.getByText("+NT$30,000")).toBeTruthy();
    expect(screen.getByText(dict.asstCommitNM.replace("%n", "2").replace("%m", "1"))).toBeTruthy();
    expect(screen.getByText(dict.asstFootNote)).toBeTruthy();
  });

  it("confirm writes, then shows the result card and the sys note", async () => {
    m.fetchAssistant.mockResolvedValue(data([], [deposit, loan]));
    m.applyAssistantDrafts.mockResolvedValue(data([{ ...msg(5, "sys", ""), result: { applied: 1, kept: 1 } }], [loan], { applied: 1, kept: 1 }));
    localStorage.setItem("argus.assistant.mode", "panel");
    renderWidget();
    fireEvent.click(await screen.findByRole("tab", { name: dict.asstTabDrafts.replace("%s", "2") }));
    fireEvent.click(screen.getByText(dict.asstCommitNM.replace("%n", "1").replace("%m", "1")));

    expect(await screen.findByText(`✓ ${dict.asstAppliedTitle.replace("%s", "1")}`)).toBeTruthy();
    expect(m.applyAssistantDrafts).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("tab", { name: dict.asstChat }));
    expect(screen.getByText(`✓ ${dict.asstSysApplied.replace("%n", "1").replace("%m", "1")}`)).toBeTruthy();
  });

  it("the edit drawer saves numbers and keeps fields it doesn't show", async () => {
    const withFund = { ...loan, payload: { ...loan.payload, fund_code: "X1" } };
    m.fetchAssistant.mockResolvedValue(data([], [withFund]));
    m.updateAssistantDraft.mockResolvedValue(data([], [{ ...withFund, status: "ok", missing: [] }]));
    localStorage.setItem("argus.assistant.mode", "panel");
    renderWidget();
    fireEvent.click(await screen.findByRole("tab", { name: dict.asstTabDrafts.replace("%s", "1") }));
    fireEvent.click(screen.getByText(`? ${dict.asstPendingVal}`));

    expect(screen.getByText(dict.asstAiUnsure)).toBeTruthy();
    const amount = document.querySelector<HTMLInputElement>(".asst-input-unsure")!;
    fireEvent.change(amount, { target: { value: "6000000" } });
    fireEvent.click(screen.getByText(dict.asstSaveDraft));

    await waitFor(() => expect(screen.queryByText(dict.asstEditTitle)).toBeNull());
    expect(m.updateAssistantDraft).toHaveBeenCalledWith(2, expect.objectContaining({ value: 6000000, rate_pct: 2.1, fund_code: "X1", type: "loan" }));
  });

  it("read-only: delete and confirm only toast", async () => {
    m.fetchAssistant.mockResolvedValue(data([], [deposit]));
    localStorage.setItem("argus.assistant.mode", "panel");
    renderWidget({ writable: false });
    fireEvent.click(await screen.findByRole("tab", { name: dict.asstTabDrafts.replace("%s", "1") }));
    fireEvent.click(screen.getByRole("button", { name: dict.asstDel }));
    fireEvent.click(screen.getByText(dict.asstCommitN.replace("%s", "1")));
    expect(await screen.findAllByText(dict.asstRoToast)).not.toHaveLength(0);
    expect(m.deleteAssistantDraft).not.toHaveBeenCalled();
    expect(m.applyAssistantDrafts).not.toHaveBeenCalled();
  });
});
