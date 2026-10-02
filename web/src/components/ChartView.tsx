import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  createChart,
  type IChartApi,
  type IPriceLine,
  type ISeriesApi,
  type SeriesMarker,
  type Time,
} from "lightweight-charts";
import {
  ApiError,
  currencySymbol,
  deleteResearchNote,
  deleteTransaction,
  fetchChart,
  fetchPeers,
  fetchResearchNotes,
  fetchRoundDetail,
  LEGACY_NOTE_TAGS,
  marketOf,
  NOTE_TAGS,
  saveResearchNote,
  setResearchNotePinned,
  setThesis,
  type Candle,
  type Chart,
  type ChartLevel,
  type NoteTag,
  type Peers,
  type ResearchNote,
  type RoundDetail,
  type RoundSummary,
  type Transaction,
} from "../api";
import type { Dictionary } from "../i18n";
import { PV_COLORS, PV_STATES, pvReadout, pvState, type PvLabels } from "../pricevolume";
import { GapBands, type GapBand } from "../gapBands";
import { defaultFill, fillRows } from "../fillSnapshot";
import { DEFAULT_PAT_ON, PAT_CATS, PAT_COLORS, patKey, patText, visiblePatterns, type PatOn } from "../patterns";
import { STRAT_COLOR, stratKey, stratText } from "../strategies";
import { MA_COLORS, MA_PERIODS, loadMaOn, saveMaOn, sma } from "../ma";
import { PatternPanel } from "./PatternPanel";
import { FillSnapshotPanel } from "./FillSnapshotPanel";
import { TradesTable } from "./TradesTable";
import type { TradeMode } from "./TradeModal";

interface Props {
  dict: Dictionary;
  ticker: string;
  initialRoundStart?: string;
  onBack: () => void;
  onTickerClick?: (ticker: string) => void;
  names?: Record<string, string>;
  writable?: boolean;
  onTrade?: (mode: TradeMode, ticker: string, prefillPrice?: number) => void;
  // onUnauthorized backs the round-detail thesis edit form's 401 handling —
  // same retry-after-login convention as TradeModal/ChartListView, needed
  // here since that edit is an inline write, not routed through TradeModal.
  onUnauthorized?: (retry: () => void) => void;
}

const maxPlottedPerSide = 3;

interface ClassifiedLevel extends ChartLevel {
  isSupport: boolean;
  distance: number;
  plotted: boolean;
}

function classifyLevels(levels: ChartLevel[], lastClose: number): ClassifiedLevel[] {
  const withSide = levels.map((l) => ({
    ...l,
    isSupport: l.price < lastClose,
    distance: Math.abs(l.price - lastClose),
  }));

  const nearestSupport = withSide
    .filter((l) => l.isSupport)
    .sort((a, b) => a.distance - b.distance)
    .slice(0, maxPlottedPerSide);
  const nearestResistance = withSide
    .filter((l) => !l.isSupport)
    .sort((a, b) => a.distance - b.distance)
    .slice(0, maxPlottedPerSide);
  const plottedPrices = new Set([...nearestSupport, ...nearestResistance].map((l) => l.price));

  return withSide
    .map((l) => ({ ...l, plotted: plottedPrices.has(l.price) }))
    .sort((a, b) => a.distance - b.distance);
}

function computeQuickStats(candles: Candle[]) {
  if (!candles || candles.length === 0) {
    return null;
  }
  const n = candles.length;
  const latest = candles[n - 1];

  const slice20 = candles.slice(-20);
  const ma20 = slice20.reduce((acc, c) => acc + c.close, 0) / slice20.length;

  const slice60 = candles.slice(-60);
  const ma60 = slice60.reduce((acc, c) => acc + c.close, 0) / slice60.length;

  let trSum = 0;
  const atrCount = Math.min(14, n);
  const atrSlice = candles.slice(-atrCount);
  for (let i = 0; i < atrSlice.length; i++) {
    const c = atrSlice[i];
    if (i === 0) {
      trSum += c.high - c.low;
    } else {
      const prev = atrSlice[i - 1];
      const tr = Math.max(c.high - c.low, Math.abs(c.high - prev.close), Math.abs(c.low - prev.close));
      trSum += tr;
    }
  }
  const atr14 = trSum / atrCount;

  const c20Ago = n > 20 ? candles[n - 1 - 20].close : candles[0].close;
  const ret20 = ((latest.close - c20Ago) / c20Ago) * 100;

  const slice120 = candles.slice(-120);
  const high120 = Math.max(...slice120.map((c) => c.high));
  const low120 = Math.min(...slice120.map((c) => c.low));
  const fromHigh = ((latest.close - high120) / high120) * 100;

  const avgVol20 = slice20.reduce((acc, c) => acc + c.volume, 0) / slice20.length;
  const vsAvg20 = avgVol20 > 0 ? ((latest.volume - avgVol20) / avgVol20) * 100 : 0;

  const rangeSpan = high120 - low120 || 1;
  const rangePct = Math.min(100, Math.max(0, ((latest.close - low120) / rangeSpan) * 100));

  return {
    latestClose: latest.close,
    ma20,
    ma60,
    ma20Pct: ((latest.close - ma20) / ma20) * 100,
    ma60Pct: ((latest.close - ma60) / ma60) * 100,
    atr14,
    atrPct: (atr14 / latest.close) * 100,
    lastVolume: latest.volume,
    ret20,
    fromHigh,
    vsAvg20,
    high120,
    low120,
    rangePct,
  };
}

function noteTagLabel(dict: Dictionary, tag: string): string {
  switch (tag) {
    case "OBSERVATION":
      return dict.notesTagObservation;
    case "VALUATION":
      return dict.notesTagValuation;
    case "RISK":
      return dict.notesTagRisk;
    case "CATALYST":
      return dict.notesTagCatalyst;
    case "TECHNICAL":
      return dict.notesTagTechnical;
    case "CHIPS":
      return dict.notesTagFlow;
    case "NEWS":
      return dict.notesTagNews;
    default:
      return dict.notesTagOther;
  }
}

// Only the four current tags carry a colour; a legacy tag gets no class and
// the CSS falls back to neutral ink-3.
function noteTagClass(tag: string): string {
  return (NOTE_TAGS as readonly string[]).includes(tag) ? `note-tag-${tag.toLowerCase()}` : "";
}

// {y} year, {n} month number, {m} the dictionary's short month name — each
// language's template picks the pieces and order it needs.
function noteMonthLabel(dict: Dictionary, ym: string): string {
  const [y, m] = ym.split("-").map(Number);
  return dict.notesMonthFmt
    .replace("{y}", String(y))
    .replace("{n}", String(m))
    .replace("{m}", dict.months[m - 1]);
}

// Dictionary templates use %s placeholders; a function replacer keeps a "$"
// inside an argument (e.g. "+$1,234") from being read as a replace pattern.
function fmt(template: string, ...args: string[]): string {
  return args.reduce((t, a) => t.replace("%s", () => a), template);
}

function fmtSigned(v: number, currency: string): string {
  const sign = v > 0 ? "+" : v < 0 ? "-" : "";
  return `${sign}${currency}${Math.abs(v).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

function spct(v: number, digits: number): string {
  return `${v >= 0 ? "+" : ""}${v.toFixed(digits)}%`;
}

function pnlClass(v: number): string {
  return v > 0 ? "profit" : v < 0 ? "loss" : "";
}

function PeerPct({ v }: { v: number | null }) {
  return v === null ? <td className="tk-dim">—</td> : <td className={pnlClass(v)}>{spct(v, 1)}</td>;
}

const defaultVisibleBars = 250;

const noteShowInitial = 8;
const noteShowStep = 20;

function MAEMFEBar({ dict, maePct, mfePct }: { dict: Dictionary; maePct: number; mfePct: number }) {
  const range = Math.max(Math.abs(maePct), Math.abs(mfePct), 1) * 1.15;
  const losePct = (Math.abs(Math.min(maePct, 0)) / range) * 100;
  const winPct = (Math.max(mfePct, 0) / range) * 100;

  return (
    <div className="report-section">
      <div className="eyebrow">MAE / MFE</div>
      <div className="mae-mfe-track">
        <div className="mae-mfe-half mae-mfe-half-loss">
          <div className="mae-mfe-loss" style={{ width: `${losePct}%` }} />
        </div>
        <div className="mae-mfe-zero" />
        <div className="mae-mfe-half mae-mfe-half-win">
          <div className="mae-mfe-win" style={{ width: `${winPct}%` }} />
        </div>
      </div>
      <div className="mae-mfe-labels">
        <span className="loss">MAE {maePct.toFixed(1)}%</span>
        <span className="profit">MFE +{mfePct.toFixed(1)}%</span>
      </div>
      <div className="stat-note">{dict.maeMfeRoundNote}</div>
    </div>
  );
}

export function ChartView({
  dict,
  ticker,
  initialRoundStart,
  onBack,
  onTickerClick,
  names = {},
  writable = false,
  onTrade,
  onUnauthorized,
}: Props) {
  const [chart, setChart] = useState<Chart | null>(null);
  const [error, setError] = useState(false);
  const [peers, setPeers] = useState<Peers | null>(null);
  const [selectedRoundStart, setSelectedRoundStart] = useState<string | null>(initialRoundStart ?? null);
  const [roundMenuOpen, setRoundMenuOpen] = useState(false);
  // railTab is only set by an explicit click / round pick; until then the tab
  // follows the selection (a picked round opens 回合, otherwise 支撐壓力).
  const [railTab, setRailTab] = useState<"pat" | "lvl" | "round" | "snap" | null>(null);
  // The fill the 成交快照 tab was last pointed at; null follows the selected round.
  const [snapPick, setSnapPick] = useState<number | null>(null);
  const [wide, setWide] = useState(false);
  // The design's "量價" chip: colour volume bars by price × volume direction.
  const [pvOn, setPvOn] = useState(true);
  const [maOn, setMaOn] = useState(loadMaOn);
  const [patOn, setPatOn] = useState<PatOn>(DEFAULT_PAT_ON);
  const [stratOn, setStratOn] = useState(true);
  const [patHigh, setPatHigh] = useState(true);
  const [selPat, setSelPat] = useState<string | null>(null);
  const [roundDetail, setRoundDetail] = useState<RoundDetail | null>(null);
  const [thesisDraft, setThesisDraft] = useState("");
  const [thesisEditing, setThesisEditing] = useState(false);
  const [thesisSubmitting, setThesisSubmitting] = useState(false);
  const [thesisError, setThesisError] = useState<string | null>(null);

  const [notes, setNotes] = useState<ResearchNote[]>([]);
  const [noteComposing, setNoteComposing] = useState(false);
  const [noteDraft, setNoteDraft] = useState("");
  const [noteTag, setNoteTag] = useState<NoteTag>("OBSERVATION");
  const [noteSubmitting, setNoteSubmitting] = useState(false);
  const [noteError, setNoteError] = useState<string | null>(null);
  const [noteQuery, setNoteQuery] = useState("");
  const [noteTagFilter, setNoteTagFilter] = useState<string | null>(null);
  const [noteShow, setNoteShow] = useState(noteShowInitial);

  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<"Candlestick"> | null>(null);
  const volumeSeriesRef = useRef<ISeriesApi<"Histogram"> | null>(null);
  const roundBgRef = useRef<ISeriesApi<"Histogram"> | null>(null);
  const patShadeRef = useRef<ISeriesApi<"Histogram"> | null>(null);
  const gapsRef = useRef<GapBands | null>(null);
  const maRef = useRef<ISeriesApi<"Line">[]>([]);
  const priceLinesRef = useRef<IPriceLine[]>([]);
  const wrapObserverRef = useRef<ResizeObserver | null>(null);
  // The crosshair readout is written straight to the DOM (a React state per
  // mouse move would re-render this whole page); pvRef is what its handler reads.
  const readoutRef = useRef<HTMLSpanElement | null>(null);
  const pvRef = useRef<{ candles: Candle[]; index: Map<string, number>; labels: PvLabels } | null>(null);

  useEffect(() => {
    setChart(null);
    setError(false);
    setSelectedRoundStart(initialRoundStart ?? null);
    setRoundMenuOpen(false);
    setRailTab(null);
    setSelPat(null);
    setSnapPick(null);
    setRoundDetail(null);
    if (!ticker) {
      setError(true);
      return;
    }
    fetchChart(ticker)
      .then(setChart)
      .catch(() => setError(true));
  }, [ticker, initialRoundStart]);

  // Peers load on their own: they fan out one history fetch per same-sector
  // ticker, which must not hold up the chart. A failure just leaves the card out.
  useEffect(() => {
    setPeers(null);
    if (!ticker) return;
    fetchPeers(ticker)
      .then(setPeers)
      .catch(() => setPeers(null));
  }, [ticker]);

  useEffect(() => {
    setThesisDraft("");
    setThesisEditing(false);
    setThesisError(null);
    if (!selectedRoundStart || !ticker) {
      setRoundDetail(null);
      return;
    }
    fetchRoundDetail(ticker, selectedRoundStart)
      .then(setRoundDetail)
      .catch(() => setRoundDetail(null));
  }, [ticker, selectedRoundStart]);

  useEffect(() => {
    setNotes([]);
    setNoteComposing(false);
    setNoteDraft("");
    setNoteError(null);
    setNoteQuery("");
    setNoteTagFilter(null);
    setNoteShow(noteShowInitial);
    if (!ticker) return;
    fetchResearchNotes(ticker)
      .then((r) => setNotes(r.notes))
      .catch(() => setNotes([]));
  }, [ticker]);

  const todayStr = new Date().toISOString().slice(0, 10);
  const todayNote = notes.find((n) => n.date === todayStr) ?? null;

  function openNoteCompose() {
    setNoteDraft(todayNote?.text ?? "");
    // A legacy-tagged note being re-edited has no chip to stay selected on
    // (the compose form only offers the current four), so it starts on 觀察.
    setNoteTag(
      todayNote && (NOTE_TAGS as readonly string[]).includes(todayNote.tag)
        ? (todayNote.tag as NoteTag)
        : "OBSERVATION",
    );
    setNoteError(null);
    setNoteComposing(true);
  }

  async function submitNote() {
    if (!ticker || !noteDraft.trim()) return;
    setNoteSubmitting(true);
    setNoteError(null);
    try {
      await saveResearchNote(ticker, noteTag, noteDraft.trim());
      setNoteComposing(false);
      setNoteDraft("");
      setNotes((await fetchResearchNotes(ticker)).notes);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(submitNote);
      } else {
        setNoteError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setNoteSubmitting(false);
    }
  }

  async function toggleNotePin(note: ResearchNote) {
    if (!ticker) return;
    try {
      await setResearchNotePinned(note.id, !note.pinned);
      setNotes((await fetchResearchNotes(ticker)).notes);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(() => toggleNotePin(note));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  async function handleDeleteNote(note: ResearchNote) {
    if (!ticker) return;
    if (!window.confirm(dict.notesDeleteConfirm)) return;
    try {
      await deleteResearchNote(note.id);
      setNotes((await fetchResearchNotes(ticker)).notes);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(() => handleDeleteNote(note));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  async function submitThesis() {
    if (!ticker || !selectedRoundStart || !thesisDraft.trim()) return;
    setThesisSubmitting(true);
    setThesisError(null);
    try {
      await setThesis(ticker, thesisDraft.trim(), selectedRoundStart);
      setThesisDraft("");
      setThesisEditing(false);
      setRoundDetail(await fetchRoundDetail(ticker, selectedRoundStart));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(submitThesis);
      } else {
        setThesisError(e instanceof ApiError ? e.message : dict.error);
      }
    } finally {
      setThesisSubmitting(false);
    }
  }

  // "→ 帶入回合論點": drop a note's text into the selected round's thesis editor
  // (which exists for open and closed rounds alike since P1b).
  function quoteNote(note: ResearchNote) {
    setRailTab("round");
    setThesisDraft(note.text);
    setThesisError(null);
    setThesisEditing(true);
  }

  async function handleDeleteTx(tx: Transaction) {
    if (!ticker || !selectedRoundStart) return;
    if (!window.confirm(dict.confirmDeleteTransaction)) return;
    try {
      await deleteTransaction(tx.id);
      setRoundDetail(await fetchRoundDetail(ticker, selectedRoundStart));
      setChart(await fetchChart(ticker));
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && onUnauthorized) {
        onUnauthorized(() => handleDeleteTx(tx));
      } else {
        window.alert(e instanceof ApiError ? e.message : dict.error);
      }
    }
  }

  const containerRef = useCallback((node: HTMLDivElement | null) => {
    if (chartRef.current) {
      chartRef.current.remove();
      chartRef.current = null;
      seriesRef.current = null;
      volumeSeriesRef.current = null;
      maRef.current = [];
      roundBgRef.current = null;
      patShadeRef.current = null;
      gapsRef.current = null;
      priceLinesRef.current = [];
    }
    if (!node) return;
    const c = createChart(node, {
      layout: { background: { color: "transparent" }, textColor: "#9AA6BC", fontFamily: '"JetBrains Mono", monospace' },
      grid: { vertLines: { color: "#243044" }, horzLines: { color: "#243044" } },
      rightPriceScale: { borderColor: "#334155" },
      timeScale: { borderColor: "#334155" },
      autoSize: true,
    });

    // Created first so the round-span tint paints behind the candles. Every
    // bar is value 1 on its own hidden scale pinned to 0..1, so it fills the
    // pane's full height whatever the price range is.
    const roundBg = c.addHistogramSeries({
      priceScaleId: "rounds",
      priceLineVisible: false,
      lastValueVisible: false,
      autoscaleInfoProvider: () => ({ priceRange: { minValue: 0, maxValue: 1 } }),
    });
    c.priceScale("rounds").applyOptions({ scaleMargins: { top: 0, bottom: 0 }, visible: false });

    // The selected pattern's bars, tinted the same way (own 0..1 scale).
    const patShade = c.addHistogramSeries({
      priceScaleId: "patshade",
      priceLineVisible: false,
      lastValueVisible: false,
      autoscaleInfoProvider: () => ({ priceRange: { minValue: 0, maxValue: 1 } }),
    });
    c.priceScale("patshade").applyOptions({ scaleMargins: { top: 0, bottom: 0 }, visible: false });

    const series = c.addCandlestickSeries({
      upColor: "#10B981",
      downColor: "#EF4444",
      borderVisible: false,
      wickUpColor: "#10B981",
      wickDownColor: "#EF4444",
    });

    const vol = c.addHistogramSeries({
      priceScaleId: "vol",
      priceFormat: { type: "volume" },
      priceLineVisible: false,
      lastValueVisible: false,
    });
    c.priceScale("vol").applyOptions({
      scaleMargins: { top: 0.8, bottom: 0 },
      visible: false,
    });

    // After the candles, so the lines paint over them; filled by the 均線 effect.
    maRef.current = MA_PERIODS.map((n) =>
      c.addLineSeries({
        color: MA_COLORS[n],
        lineWidth: 1,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      }),
    );

    const gaps = new GapBands();
    series.attachPrimitive(gaps);

    // A click on a pattern marker selects it (its id is "pat:<key>"; trade
    // markers have none) and opens the 型態 tab.
    c.subscribeClick((p) => {
      const id = p.hoveredObjectId;
      if (typeof id === "string" && id.startsWith("pat:")) {
        setSelPat(id.slice(4));
        setRailTab("pat");
      }
    });

    c.subscribeCrosshairMove((p) => {
      const el = readoutRef.current;
      if (!el) return;
      const pv = pvRef.current;
      const i = pv && p.time != null ? pv.index.get(String(p.time)) : undefined;
      el.textContent = pv && i != null ? pvReadout(pv.candles, i, pv.labels) : "";
    });

    chartRef.current = c;
    seriesRef.current = series;
    volumeSeriesRef.current = vol;
    roundBgRef.current = roundBg;
    patShadeRef.current = patShade;
    gapsRef.current = gaps;
  }, []);

  // The rail goes sticky only when the two-column row is at least 1000px wide;
  // below that it wraps above the main column and must scroll with the page.
  const wrapRef = useCallback((el: HTMLDivElement | null) => {
    wrapObserverRef.current?.disconnect();
    wrapObserverRef.current = null;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => setWide(el.offsetWidth >= 1000));
    ro.observe(el);
    wrapObserverRef.current = ro;
  }, []);

  // Memoised on chart: the series effect below lists it as a dependency, and a
  // fresh array every render would re-run it (resetting the chart's zoom)
  // on every unrelated state change such as typing in the note box.
  const classified = useMemo(
    () =>
      chart && chart.candles.length > 0
        ? classifyLevels(chart.levels, chart.candles[chart.candles.length - 1].close)
        : [],
    [chart],
  );

  const hits = useMemo(() => chart?.patterns ?? [], [chart]);
  const visPats = useMemo(() => visiblePatterns(hits, patOn, patHigh), [hits, patOn, patHigh]);
  const strategies = useMemo(() => chart?.strategies ?? [], [chart]);
  const visStrats = useMemo(() => (stratOn ? strategies : []), [strategies, stratOn]);
  const patCounts = useMemo(() => {
    const n = Object.fromEntries(PAT_CATS.map((k) => [k, 0])) as Record<(typeof PAT_CATS)[number], number>;
    for (const p of hits) if (p.cat in n && (!patHigh || p.conf === "" || p.conf === "high")) n[p.cat]++;
    return n;
  }, [hits, patHigh]);

  const noteQueryLower = noteQuery.trim().toLowerCase();
  const filteredNotes = notes.filter(
    (n) =>
      (!noteTagFilter || n.tag === noteTagFilter) &&
      (!noteQueryLower || n.text.toLowerCase().includes(noteQueryLower) || n.date.includes(noteQueryLower)),
  );
  const pinnedNotes = filteredNotes.filter((n) => n.pinned);
  const unpinnedNotes = filteredNotes.filter((n) => !n.pinned);
  const visibleNotes = unpinnedNotes.slice(0, noteShow);
  const noteFiltering = noteQueryLower !== "" || noteTagFilter !== null;
  const noteFilterTags: string[] = [
    ...NOTE_TAGS,
    ...LEGACY_NOTE_TAGS.filter((tg) => notes.some((n) => n.tag === tg)),
  ];
  const canQuoteNote = writable && !!roundDetail && roundDetail.start === selectedRoundStart;

  function renderNoteRow(n: ResearchNote, monthLabel?: string) {
    return (
      <Fragment key={n.id}>
        {monthLabel && <div className="note-month-label">{monthLabel}</div>}
        <div className="note-row">
          <div className="note-row-meta">
            <span className="note-row-date">{n.date}</span>
            <span className={`note-tag-pill ${noteTagClass(n.tag)}`}>{noteTagLabel(dict, n.tag)}</span>
          </div>
          <div className="note-row-text">{n.text}</div>
          {writable && (
            <div className="note-row-actions">
              <button
                type="button"
                className={`note-row-action${n.pinned ? " active" : ""}`}
                onClick={() => toggleNotePin(n)}
              >
                {n.pinned ? dict.notesUnpinLabel : dict.notesPinLabel}
              </button>
              {canQuoteNote && (
                <button
                  type="button"
                  className="note-row-action note-row-action-quote"
                  onClick={() => quoteNote(n)}
                >
                  {dict.notesToThesis}
                </button>
              )}
              <button
                type="button"
                className="note-row-action note-row-action-danger"
                onClick={() => handleDeleteNote(n)}
              >
                {dict.notesDeleteLabel}
              </button>
            </div>
          )}
        </div>
      </Fragment>
    );
  }

  let lastNoteMonth = "";

  useEffect(() => {
    const series = seriesRef.current;
    if (!series || !chart) return;

    series.setData(
      chart.candles.map((c) => ({
        time: c.date as Time,
        open: c.open,
        high: c.high,
        low: c.low,
        close: c.close,
      })),
    );

    for (const line of priceLinesRef.current) {
      series.removePriceLine(line);
    }
    priceLinesRef.current = classified
      .filter((l) => l.plotted)
      .map((l) =>
        series.createPriceLine({
          price: l.price,
          color: l.isSupport ? "#10B981" : "#EF4444",
          lineWidth: 1,
          lineStyle: 2,
          axisLabelVisible: true,
          title: `${l.price.toFixed(2)} ×${l.touches}`,
        }),
      );

    if (chart.position) {
      priceLinesRef.current.push(
        series.createPriceLine({
          price: chart.position.avgCost,
          color: "#818cf8",
          lineWidth: 1,
          lineStyle: 0,
          axisLabelVisible: true,
          title: `${dict.avgCost} ${chart.position.avgCost.toFixed(2)}`,
        }),
      );
    }

    if (roundDetail) {
      const fromDate = roundDetail.start;
      const lastCandle = chart.candles[chart.candles.length - 1];
      const toDate = roundDetail.end || lastCandle?.date;
      if (toDate) {
        chartRef.current?.timeScale().setVisibleRange({
          from: fromDate as Time,
          to: toDate as Time,
        });
      }
    } else {
      // The endpoint serves ~2y; open on the latest year and let the user scroll back.
      const n = chart.candles.length;
      if (n > defaultVisibleBars) {
        chartRef.current?.timeScale().setVisibleLogicalRange({ from: n - defaultVisibleBars - 0.5, to: n + 0.5 });
      } else {
        chartRef.current?.timeScale().fitContent();
      }
    }
  }, [chart, roundDetail, classified, dict.avgCost]);

  // Volume bars live in their own effect so flipping the 量價 chip recolours
  // them without the main effect's fitContent() resetting the user's zoom.
  useEffect(() => {
    const vol = volumeSeriesRef.current;
    if (!vol || !chart) return;
    vol.setData(
      chart.candles.map((c, i) => {
        const st = pvOn ? pvState(chart.candles, i) : null;
        return {
          time: c.date as Time,
          value: c.volume,
          color: st ? PV_COLORS[st] : c.close >= c.open ? "rgba(16,185,129,0.4)" : "rgba(239,68,68,0.4)",
        };
      }),
    );
  }, [chart, pvOn]);

  // Moving averages, in their own effect like the volume bars so the chip
  // doesn't reset the zoom.
  useEffect(() => {
    if (!chart) return;
    MA_PERIODS.forEach((n, i) => {
      maRef.current[i]?.setData(maOn ? sma(chart.candles, n).map((p) => ({ time: p.time as Time, value: p.value })) : []);
    });
  }, [chart, maOn]);

  const pvLabels: PvLabels = useMemo(
    () => ({
      states: { upVu: dict.pvUpVu, upVd: dict.pvUpVd, dnVu: dict.pvDnVu, dnVd: dict.pvDnVd },
      price: dict.pvReadPrice,
      vol: dict.pvReadVol,
    }),
    [dict],
  );

  useEffect(() => {
    pvRef.current = chart
      ? { candles: chart.candles, index: new Map(chart.candles.map((c, i) => [c.date, i])), labels: pvLabels }
      : null;
  }, [chart, pvLabels]);

  // Round-span background: each round tints its own candles green (won), red
  // (lost) or indigo (still open); the selected round is a shade stronger.
  // Separate from the effect above so picking a round recolours immediately,
  // without waiting for its detail fetch or re-zooming the chart.
  useEffect(() => {
    const bg = roundBgRef.current;
    if (!bg || !chart) return;
    bg.setData(
      chart.candles.map((c) => {
        const r = chart.rounds.find((rd) => c.date >= rd.start && (rd.open || c.date <= rd.end));
        if (!r) return { time: c.date as Time, value: 1, color: "rgba(0,0,0,0)" };
        const rgb = r.open ? "129,140,248" : r.realizedPnL >= 0 ? "16,185,129" : "239,68,68";
        return { time: c.date as Time, value: 1, color: `rgba(${rgb},${r.start === selectedRoundStart ? 0.09 : 0.05})` };
      }),
    );
  }, [chart, selectedRoundStart]);

  // Trade markers of the selected round and the visible patterns share the one
  // marker list lightweight-charts allows, which must be sorted by time. Kept
  // apart from the series effect so picking a pattern doesn't reset the zoom.
  useEffect(() => {
    const series = seriesRef.current;
    if (!series || !chart) return;
    const currency = currencySymbol(marketOf(chart.ticker));
    const trades: SeriesMarker<Time>[] = (roundDetail?.trades ?? []).map((t) => ({
      time: t.date as Time,
      position: t.side === "BUY" ? "belowBar" : "aboveBar",
      color: t.side === "BUY" ? "#10B981" : "#EF4444",
      shape: t.side === "BUY" ? "arrowUp" : "arrowDown",
      text: `${t.side} ${t.shares}@${currency}${t.price.toFixed(2)}`,
    }));
    const pats: SeriesMarker<Time>[] = visPats
      .filter((p) => p.cat !== "gap")
      .map((p) => {
        const txt = patText(dict, p.type);
        const isSel = patKey(p) === selPat;
        return {
          time: p.end as Time,
          id: `pat:${patKey(p)}`,
          position: p.dir === "bull" ? "belowBar" : "aboveBar",
          shape: "circle",
          color: isSel ? "#a5b4fc" : PAT_COLORS[p.dir],
          size: isSel ? 1.6 : p.conf === "high" ? 1 : 0.6,
          text: isSel ? txt.name : p.conf === "high" ? txt.code : "",
        };
      });
    // A strategy alert is a square under the bar, unlabelled until picked.
    const strats: SeriesMarker<Time>[] = visStrats.map((a) => {
      const isSel = stratKey(a) === selPat;
      return {
        time: a.date as Time,
        id: `pat:${stratKey(a)}`,
        position: "belowBar",
        shape: "square",
        color: isSel ? "#a5b4fc" : STRAT_COLOR,
        size: isSel ? 1.4 : 1,
        text: isSel ? stratText(dict, a.type).name : "",
      };
    });
    series.setMarkers([...trades, ...pats, ...strats].sort((a, b) => (a.time < b.time ? -1 : a.time > b.time ? 1 : 0)));
  }, [chart, roundDetail, visPats, visStrats, selPat, dict]);

  useEffect(() => {
    gapsRef.current?.set(
      visPats
        .filter((p) => p.cat === "gap" && p.gap)
        .map(
          (p): GapBand => ({
            lo: p.gap!.lo,
            hi: p.gap!.hi,
            from: p.end,
            fill: p.gap!.fillDate,
            bull: p.dir === "bull",
            selected: patKey(p) === selPat,
          }),
        ),
      dict.patGapOpen,
    );
  }, [chart, visPats, selPat, dict.patGapOpen]);

  // The selected pattern's own bars get a tint; a gap has its band instead.
  useEffect(() => {
    const shade = patShadeRef.current;
    if (!shade || !chart) return;
    const sp = selPat ? hits.find((p) => patKey(p) === selPat) : undefined;
    const sa = selPat ? strategies.find((a) => stratKey(a) === selPat) : undefined;
    // A pattern shades its own bars (a gap has its band instead); a strategy alert, its one bar.
    const span = sp && sp.cat !== "gap" ? { from: sp.start, to: sp.end } : sa ? { from: sa.date, to: sa.date } : null;
    shade.setData(
      span
        ? chart.candles
            .filter((c) => c.date >= span.from && c.date <= span.to)
            .map((c) => ({ time: c.date as Time, value: 1, color: "rgba(129,140,248,0.16)" }))
        : [],
    );
  }, [chart, hits, strategies, selPat]);

  // Picking a pattern that's off-screen (the chart opens on the latest year of
  // two) scrolls it into view, keeping the current zoom.
  useEffect(() => {
    const ts = chartRef.current?.timeScale();
    const sp = selPat ? hits.find((p) => patKey(p) === selPat) : undefined;
    const sa = selPat ? strategies.find((a) => stratKey(a) === selPat) : undefined;
    const from = sp?.start ?? sa?.date;
    const to = sp?.end ?? sa?.date;
    if (!ts || !from || !to || !chart) return;
    const i0 = chart.candles.findIndex((c) => c.date >= from);
    const i1 = chart.candles.findIndex((c) => c.date >= to);
    const r = ts.getVisibleLogicalRange();
    if (r && i0 >= 0 && i1 >= 0 && (i0 < r.from + 3 || i1 > r.to - 3)) {
      const w = r.to - r.from;
      const mid = (i0 + i1) / 2;
      ts.setVisibleLogicalRange({ from: mid - w / 2, to: mid + w / 2 });
    }
    // Only a change of selection scrolls; a data refresh must not.
  }, [selPat]);

  if (error) {
    return (
      <>
        <button className="back-link" onClick={onBack}>
          {dict.back}
        </button>
        <div className="error-message">{dict.error}</div>
      </>
    );
  }
  if (!chart) {
    return <div className="loading">{dict.loading}</div>;
  }

  const currency = currencySymbol(marketOf(chart.ticker));
  const stats = computeQuickStats(chart.candles);
  const latestPrice = chart.candles[chart.candles.length - 1]?.close ?? 0;
  const prevClose =
    chart.candles.length > 1 ? chart.candles[chart.candles.length - 2].close : latestPrice;
  const dayChangePct = prevClose ? ((latestPrice - prevClose) / prevClose) * 100 : 0;
  const tickerName = names[chart.ticker];

  const statCells = stats
    ? [
        {
          label: dict.ma20,
          value: `${currency}${stats.ma20.toFixed(2)}`,
          note: `${stats.ma20Pct >= 0 ? dict.above : dict.below} ${spct(stats.ma20Pct, 1)}`,
          noteCls: stats.ma20Pct >= 0 ? "profit" : "loss",
        },
        {
          label: dict.ma60,
          value: `${currency}${stats.ma60.toFixed(2)}`,
          note: `${stats.ma60Pct >= 0 ? dict.above : dict.below} ${spct(stats.ma60Pct, 1)}`,
          noteCls: stats.ma60Pct >= 0 ? "profit" : "loss",
        },
        {
          label: dict.atr14,
          value: `${currency}${stats.atr14.toFixed(2)}`,
          note: `${stats.atrPct.toFixed(2)}%`,
          noteCls: "",
        },
        { label: dict.ret20, value: spct(stats.ret20, 1), note: "", noteCls: "" },
        {
          label: dict.fromHigh,
          value: spct(stats.fromHigh, 1),
          note: `${dict.rangeHigh} ${currency}${stats.high120.toFixed(2)}`,
          noteCls: "loss",
        },
        {
          label: dict.volume,
          value: `${(stats.lastVolume / 1e6).toFixed(2)}M`,
          note: `${dict.vsAvg20} ${spct(stats.vsAvg20, 0)}`,
          noteCls: stats.vsAvg20 >= 0 ? "profit" : "",
        },
      ]
    : [];

  // chart.rounds arrives newest-first; #1 is the oldest round.
  const chrono = [...chart.rounds].reverse();
  const roundNo = (r: RoundSummary) => chrono.indexOf(r) + 1;
  const roundDays = (r: RoundSummary) =>
    chart.candles.filter((c) => c.date >= r.start && (r.open || c.date <= r.end)).length;
  const roundRange = (r: RoundSummary) => `${r.start} → ${r.end ? r.end.slice(5) : dict.open}`;
  const roundPnl = (r: RoundSummary) => (r.open ? dict.roundHolding : fmtSigned(r.realizedPnL, currency));
  const roundPnlCls = (r: RoundSummary) => (r.open ? "tk-accent" : pnlClass(r.realizedPnL));
  const closedRounds = chart.rounds.filter((r) => !r.open);
  const roundsSummaryText = fmt(
    dict.roundsSummary,
    String(chart.rounds.length),
    String(closedRounds.filter((r) => r.realizedPnL >= 0).length),
    String(closedRounds.length),
    fmtSigned(
      closedRounds.reduce((a, r) => a + r.realizedPnL, 0),
      currency,
    ),
  );
  const selectedRound = chrono.find((r) => r.start === selectedRoundStart) ?? null;
  const selPos = selectedRound ? chrono.indexOf(selectedRound) : -1;
  const olderRound = selPos < 0 ? chrono[chrono.length - 1] : chrono[selPos - 1];
  const newerRound = selPos < 0 ? undefined : chrono[selPos + 1];

  function pickRound(r: RoundSummary | null) {
    setSelectedRoundStart(r ? r.start : null);
    if (r) setRailTab("round");
    setRoundMenuOpen(false);
  }

  const tab = railTab ?? (selectedRoundStart ? "round" : "pat");
  const fills = fillRows(chart.fills ?? [], chart.rounds);
  const snapFill = defaultFill(fills, snapPick, selectedRoundStart);
  const detail = roundDetail && roundDetail.start === selectedRoundStart ? roundDetail : null;
  const pos = chart.position;

  return (
    <>
      <button className="back-link" onClick={onBack}>
        {dict.back}
      </button>

      <div className="ticker-header-row">
        <div>
          <div className="ticker-header-idrow">
            <span className="ticker-header-id mono">{chart.ticker}</span>
            {tickerName && <span className="ticker-header-name">{tickerName}</span>}
          </div>
          <div className="ticker-price-row">
            <span className="ticker-price-val mono">
              {currency}
              {latestPrice.toFixed(2)}
            </span>
            <span className={`watchlist-change-badge ${dayChangePct >= 0 ? "profit" : "loss"}`}>
              {dayChangePct >= 0 ? "+" : ""}
              {dayChangePct.toFixed(2)}%
            </span>
          </div>
        </div>

        {stats && (
          <div className="ticker-stats-inline">
            {statCells.map((c) => (
              <div key={c.label}>
                <div className="ticker-stat-label">{c.label}</div>
                <div className="ticker-stat-val mono">{c.value}</div>
                <div className={`ticker-stat-note ${c.noteCls}`}>{c.note}</div>
              </div>
            ))}
          </div>
        )}

        {writable && onTrade && (
          <div className="ticker-header-actions">
            <button
              className="btn-tint-trade btn-tint-buy"
              onClick={() => onTrade("buy", chart.ticker, latestPrice)}
            >
              {dict.buy}
            </button>
            <button
              className="btn-tint-trade btn-tint-sell"
              onClick={() => onTrade("sell", chart.ticker, latestPrice)}
            >
              {dict.sell}
            </button>
            <button
              className="btn-tint-trade btn-tint-plain"
              onClick={() =>
                onTrade("stop", chart.ticker, chart.position?.stopPrice || latestPrice)
              }
            >
              {dict.stopPriceCol}
            </button>
            <button
              className="btn-tint-trade btn-tint-plain"
              onClick={() => onTrade("buyalert", chart.ticker, latestPrice)}
            >
              {dict.addBuyAlert}
            </button>
          </div>
        )}
      </div>

      {stats && (
        <div className="range-bar-wrap">
          <div className="range-bar-track">
            <span className="range-bar-marker" style={{ left: `${stats.rangePct.toFixed(0)}%` }} />
          </div>
          <div className="range-bar-labels">
            <span>
              {dict.rangeLow} {currency}
              {stats.low120.toFixed(2)}
            </span>
            <span>
              {dict.rangeNote} · {dict.pctOfRange} {stats.rangePct.toFixed(0)}%
            </span>
            <span>
              {dict.rangeHigh} {currency}
              {stats.high120.toFixed(2)}
            </span>
          </div>
        </div>
      )}

      <div className="tk-wrap" ref={wrapRef}>
        <aside className={`tk-rail${wide ? " sticky" : ""}`}>
          <div className="card">
            <div className="eyebrow">{dict.thisPosition}</div>
            {!pos ? (
              <div className="empty-message">{dict.noPositionHere}</div>
            ) : (
              <div className="position-rows">
                <div className="position-row">
                  <span className="stat-note">{dict.shares}</span>
                  <span className="position-row-val">{pos.shares.toLocaleString()}</span>
                </div>
                <div className="position-row">
                  <span className="stat-note">{dict.avgCost}</span>
                  <span className="position-row-val">
                    {currency}
                    {pos.avgCost.toFixed(2)}
                  </span>
                </div>
                <div className="position-row">
                  <span className="stat-note">{dict.marketValue}</span>
                  <span className="position-row-val">
                    {currency}
                    {pos.value.toLocaleString(undefined, { maximumFractionDigits: 0 })}
                  </span>
                </div>
                <div className="position-row">
                  <span className="stat-note">{dict.weight}</span>
                  <span className="position-row-val">{pos.weightPct.toFixed(1)}%</span>
                </div>
                <div className="position-row">
                  <span className="stat-note">{dict.unrealizedPnL}</span>
                  <span className={`position-row-val ${pnlClass(pos.unrealizedPnLPct)}`}>
                    {fmtSigned((pos.price - pos.avgCost) * pos.shares, currency)} ({spct(pos.unrealizedPnLPct, 2)})
                  </span>
                </div>
                <div className="position-row">
                  <span className="stat-note">{dict.stopPriceCol}</span>
                  <span className="position-row-val">
                    {pos.stopPrice > 0
                      ? `${currency}${pos.stopPrice.toFixed(2)} (${spct(-((pos.price - pos.stopPrice) / pos.price) * 100, 1)})`
                      : dict.noStopSet}
                  </span>
                </div>
                <div className="position-row">
                  <span className="stat-note">{dict.riskIfStopped}</span>
                  <span className={`position-row-val ${pos.openRisk !== null ? "loss" : ""}`}>
                    {pos.openRisk !== null
                      ? `-${currency}${Math.abs(pos.openRisk).toLocaleString(undefined, {
                          maximumFractionDigits: 0,
                        })} · ${dict.pctOfAccount} ${pos.openRiskPct?.toFixed(1)}%`
                      : "—"}
                  </span>
                </div>
              </div>
            )}
          </div>

          <div className="card">
            <div className="tk-tabs">
              <button
                type="button"
                className={`tk-tab${tab === "pat" ? " active" : ""}`}
                onClick={() => setRailTab("pat")}
              >
                {dict.tkTabPat}
              </button>
              <button
                type="button"
                className={`tk-tab${tab === "lvl" ? " active" : ""}`}
                onClick={() => setRailTab("lvl")}
              >
                {dict.tkTabLvl}
                <span className="tk-tab-count">{classified.length}</span>
              </button>
              <button
                type="button"
                className={`tk-tab${tab === "round" ? " active" : ""}`}
                onClick={() => setRailTab("round")}
              >
                {dict.tkTabRound}
                <span className="tk-tab-count">{chart.rounds.length}</span>
              </button>
              <button
                type="button"
                className={`tk-tab${tab === "snap" ? " active" : ""}`}
                onClick={() => setRailTab("snap")}
              >
                {dict.snapTab}
                <span className="tk-tab-count">{fills.length}</span>
              </button>
            </div>

            {tab === "snap" && (
              <FillSnapshotPanel
                dict={dict}
                ticker={chart.ticker}
                rows={fills}
                current={snapFill}
                onPick={setSnapPick}
                currency={currency}
                patternKnown={(key) => hits.some((p) => patKey(p) === key)}
                onPattern={(key) => {
                  setSelPat(key);
                  setRailTab("pat");
                }}
              />
            )}

            {tab === "pat" && (
              <PatternPanel
                dict={dict}
                hits={hits}
                visible={visPats}
                strategies={strategies}
                visibleStrategies={visStrats}
                ticker={chart.ticker}
                candles={chart.candles}
                selKey={selPat}
                onSelect={setSelPat}
              />
            )}

            {tab === "lvl" && (
              <div style={{ overflowX: "auto" }}>
                <div className="eyebrow">
                  {dict.support} / {dict.resistance}
                </div>
                {classified.length === 0 ? (
                  <div className="empty-message">{dict.noLevels}</div>
                ) : (
                  <>
                    <table className="mono">
                      <thead>
                        <tr>
                          <th>{dict.levelType}</th>
                          <th>{dict.price}</th>
                          <th>{dict.touches}</th>
                          <th>{dict.lastTouch}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {classified.map((l) => (
                          <tr key={l.price}>
                            <td className={l.isSupport ? "profit" : "loss"}>
                              {l.plotted ? "● " : ""}
                              {l.isSupport ? dict.support : dict.resistance}
                            </td>
                            <td>
                              {currency}
                              {l.price.toFixed(2)}
                            </td>
                            <td>{l.touches}</td>
                            <td>{l.lastDate}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                    <div className="tk-rail-note">{dict.levelsNote}</div>
                  </>
                )}
              </div>
            )}

            {tab === "round" && (
              <div style={{ overflowX: "auto" }}>
                {selectedRound && detail && (
                  <>
                    <div className="tk-rail-title">
                      <span className="tk-rail-title-text">
                        {dict.roundSpan} {selectedRound.start} → {selectedRound.end || dict.open}
                      </span>
                      <span className={`tk-rail-title-pnl ${roundPnlCls(selectedRound)}`}>
                        {selectedRound.open ? "—" : fmtSigned(selectedRound.realizedPnL, currency)}
                      </span>
                    </div>
                    {detail.hasMaeMfe && <MAEMFEBar dict={dict} maePct={detail.maePct} mfePct={detail.mfePct} />}
                    <div className="eyebrow tk-section-title">{dict.tradesInRound}</div>
                    <TradesTable
                      compact
                      dict={dict}
                      transactions={detail.trades}
                      currency={currency}
                      names={names}
                      onDelete={writable ? handleDeleteTx : undefined}
                    />

                    <div className="tk-section-title">
                      <div className="thesis-header">
                        <div className="eyebrow">{dict.thesisLabel}</div>
                        {detail.thesisEdited && !thesisEditing && <span className="thesis-edited-tag">{dict.thesisEdited}</span>}
                        {writable && !thesisEditing && (
                          <button
                            type="button"
                            className="thesis-edit-btn"
                            onClick={() => {
                              setThesisDraft(detail.thesis);
                              setThesisEditing(true);
                            }}
                          >
                            <svg
                              width="11"
                              height="11"
                              viewBox="0 0 16 16"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="1.6"
                              aria-hidden="true"
                            >
                              <path d="M11.5 2.5 L13.5 4.5 L5.5 12.5 L2.5 13.5 L3.5 10.5 Z" />
                            </svg>
                            <span>{detail.thesis ? dict.thesisEdit : dict.thesisAdd}</span>
                          </button>
                        )}
                      </div>
                      {!selectedRound?.open && <div className="thesis-note">{dict.currentThesisNote}</div>}
                      {detail.thesis && !thesisEditing && <div className="thesis-text">{detail.thesis}</div>}
                      {!detail.thesis && !thesisEditing && (
                        <div className="thesis-empty-box">
                          {selectedRound?.open ? dict.thesisEmptyOpen : dict.thesisEmptyClosed}
                        </div>
                      )}
                      {thesisEditing && (
                        <div style={{ marginTop: 10, display: "flex", flexDirection: "column", gap: 8 }}>
                          <label className="form-field">
                            <textarea
                              rows={4}
                              autoFocus
                              value={thesisDraft}
                              placeholder={dict.thesisPlaceholder}
                              onChange={(e) => setThesisDraft(e.target.value)}
                            />
                          </label>
                          {thesisError && <div className="error-message">{thesisError}</div>}
                          <div className="modal-actions">
                            <button
                              type="button"
                              onClick={() => {
                                setThesisEditing(false);
                                setThesisDraft("");
                                setThesisError(null);
                              }}
                            >
                              {dict.cancel}
                            </button>
                            <button
                              type="button"
                              className="btn-primary"
                              disabled={!thesisDraft.trim() || thesisSubmitting}
                              onClick={submitThesis}
                            >
                              {dict.thesisSave}
                            </button>
                          </div>
                        </div>
                      )}
                    </div>
                    {detail.lessons.length > 0 && (
                      <div className="tk-section-title">
                        <div className="eyebrow">{dict.lessonsLabel}</div>
                        <ul className="lessons-list">
                          {detail.lessons.map((l, i) => (
                            <li key={i}>
                              <span className="stat-note">{l.date}</span> {l.lesson}
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </>
                )}

                <div className={selectedRound && detail ? "tk-section-title" : ""}>
                  <div className="eyebrow">{dict.tickerRounds}</div>
                  {chart.rounds.length === 0 ? (
                    <div className="empty-message">{dict.noRoundsHere}</div>
                  ) : (
                    <>
                      <div className="tk-round-summary">{roundsSummaryText}</div>
                      <div className="tk-rail-scroll">
                        <table className="mono">
                          <thead>
                            <tr>
                              <th>#</th>
                              <th>{dict.startDate}</th>
                              <th>{dict.endDate}</th>
                              <th>MAE</th>
                              <th>MFE</th>
                              <th>{dict.realizedPnL}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {chart.rounds.map((r) => (
                              <tr
                                key={r.start}
                                className={`tk-round-row${r.start === selectedRoundStart ? " selected" : ""}`}
                                tabIndex={0}
                                onClick={() => pickRound(r)}
                                onKeyDown={(e) => {
                                  if (e.key === "Enter" || e.key === " ") {
                                    e.preventDefault();
                                    pickRound(r);
                                  }
                                }}
                              >
                                <td className="tk-dim">#{roundNo(r)}</td>
                                <td>{r.start}</td>
                                <td className={r.open ? "tk-accent" : ""}>{r.end || dict.open}</td>
                                {r.hasMaeMfe ? (
                                  <>
                                    <td className="loss">{r.maePct.toFixed(1)}%</td>
                                    <td className="profit">+{r.mfePct.toFixed(1)}%</td>
                                  </>
                                ) : (
                                  <>
                                    <td className="tk-dim">—</td>
                                    <td className="tk-dim">—</td>
                                  </>
                                )}
                                <td className={r.open ? "" : pnlClass(r.realizedPnL)}>
                                  {r.open ? "—" : fmtSigned(r.realizedPnL, currency)}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </>
                  )}
                </div>
              </div>
            )}
          </div>
        </aside>

        <div className="tk-main">
          {chart.rounds.length === 0 && (
            <div className="empty-message" style={{ marginBottom: 16 }}>
              {dict.noRoundsHere}
            </div>
          )}
          <div className="rp-row">
            {chart.rounds.length > 0 && (
              <>
                  <span className="rp-label" title={dict.roundPicker}>
                    {dict.tkRoundLabel}
                  </span>
                  <button
                    type="button"
                    className={`round-chip${selectedRound ? "" : " active"}`}
                    onClick={() => pickRound(null)}
                  >
                    {dict.allTrades}
                    <span className="rp-count">{chart.rounds.length}</span>
                  </button>
                  <div className="rp-picker">
                    <button
                      type="button"
                      className="rp-step"
                      title={dict.roundOlderTip}
                      disabled={!olderRound}
                      onClick={() => olderRound && pickRound(olderRound)}
                    >
                      ‹
                    </button>
                    <button
                      type="button"
                      className={`round-chip${selectedRound ? " active" : ""}`}
                      onClick={() => setRoundMenuOpen((o) => !o)}
                    >
                      {selectedRound ? `#${roundNo(selectedRound)} · ${roundRange(selectedRound)}` : dict.tkPickRound}
                      {selectedRound && <span className={roundPnlCls(selectedRound)}>{roundPnl(selectedRound)}</span>}
                      <span className="rp-caret">▾</span>
                    </button>
                    <button
                      type="button"
                      className="rp-step"
                      title={dict.roundNewerTip}
                      disabled={!newerRound}
                      onClick={() => newerRound && pickRound(newerRound)}
                    >
                      ›
                    </button>
                    {roundMenuOpen && (
                      <>
                        <div className="rp-backdrop" onClick={() => setRoundMenuOpen(false)} />
                        <div className="rp-menu">
                          <div className="rp-menu-summary">{roundsSummaryText}</div>
                          {chart.rounds.map((r) => (
                            <div
                              key={r.start}
                              className={`rp-menu-row${r.start === selectedRoundStart ? " selected" : ""}`}
                              onClick={() => pickRound(r)}
                            >
                              <span className="rp-menu-n">#{roundNo(r)}</span>
                              <span>{roundRange(r)}</span>
                              <span className="rp-menu-days">
                                {roundDays(r)}
                                {dict.roundDaysUnit}
                              </span>
                              <span className={`rp-menu-pnl ${roundPnlCls(r)}`}>{roundPnl(r)}</span>
                            </div>
                          ))}
                        </div>
                      </>
                    )}
                  </div>
                <span className="rp-sep" />
              </>
            )}
            <span className="rp-label">{dict.patTitle}</span>
            <button
              type="button"
              className={`round-chip${pvOn ? " active" : ""}`}
              title={dict.pvChipTip}
              onClick={() => setPvOn((v) => !v)}
            >
              <span className="pv-dot" />
              {dict.pvChip}
            </button>
            <button
              type="button"
              className={`round-chip${maOn ? " active" : ""}`}
              title={dict.maChipTip}
              onClick={() => {
                setMaOn(!maOn);
                saveMaOn(!maOn);
              }}
            >
              <span className="ma-dot" />
              {dict.maChip}
            </button>
            <button
              type="button"
              className={`round-chip${stratOn ? " active" : ""}`}
              title={dict.stratChipTip}
              onClick={() => setStratOn((v) => !v)}
            >
              <span className="pat-dot strat" />
              {dict.stratChip}
              <span className="pat-chip-count">{strategies.length}</span>
            </button>
            {PAT_CATS.map((k) => (
              <button
                key={k}
                type="button"
                className={`round-chip${patOn[k] ? " active" : ""}`}
                title={dict[`patCatTip_${k}`]}
                onClick={() => setPatOn((o) => ({ ...o, [k]: !o[k] }))}
              >
                <span className={`pat-dot ${k}`} />
                {dict[`patCat_${k}`]}
                <span className="pat-chip-count">{patCounts[k]}</span>
              </button>
            ))}
            <button
              type="button"
              className={`round-chip pat-high${patHigh ? " active" : ""}`}
              title={dict.patHighTip}
              onClick={() => setPatHigh((v) => !v)}
            >
              {dict.patHighOnly}
            </button>
          </div>

          <div className="card card--glow chart-card">
            <div className="tk-chart" ref={containerRef} />
            <div className="pv-bar">
              {pvOn && (
                <span className="pv-legend">
                  <span className="pv-legend-title">{dict.pvLegend}</span>
                  {PV_STATES.map((k) => (
                    <span key={k} className="pv-legend-item">
                      <span className="pv-swatch" style={{ background: PV_COLORS[k] }} />
                      {pvLabels.states[k]}
                    </span>
                  ))}
                </span>
              )}
              {maOn && (
                <span className="pv-legend">
                  <span className="pv-legend-title">{dict.maChip}</span>
                  {MA_PERIODS.map((n) => (
                    <span key={n} className="pv-legend-item">
                      <span className="ma-leg-line" style={{ background: MA_COLORS[n] }} />
                      {n}
                    </span>
                  ))}
                </span>
              )}
              <span className="pv-legend">
                <span className="pv-legend-title">{dict.tkTabPat}</span>
                {(["bull", "bear", "neu"] as const).map((d) => (
                  <span key={d} className="pv-legend-item">
                    <span className="pat-leg-dot" style={{ background: PAT_COLORS[d] }} />
                    {dict[`patLeg${d === "bull" ? "Bull" : d === "bear" ? "Bear" : "Neu"}` as const]}
                  </span>
                ))}
                <span className="pv-legend-item">
                  <span className="pat-leg-strat" />
                  {dict.stratLeg}
                </span>
                <span className="pv-legend-item">
                  <span className="pat-leg-gap" />
                  {dict.patLegGap}
                </span>
              </span>
              <span className="pv-readout" ref={readoutRef} />
            </div>
          </div>

          {peers && Array.isArray(peers.peers) && peers.peers.length >= 2 && (
            <div className="card peer-card">
              <div className="eyebrow">
                {dict.peerTitle} · {peers.sector}
              </div>
              <table className="mono">
                <thead>
                  <tr>
                    <th>{dict.ticker}</th>
                    <th>{dict.price}</th>
                    <th>20d</th>
                    <th>60d</th>
                    <th>120d</th>
                    <th>{dict.peerRel}</th>
                  </tr>
                </thead>
                <tbody>
                  {peers.peers.map((p) => (
                    <tr
                      key={p.ticker}
                      className={p.self ? "peer-row self" : "peer-row"}
                      title={names[p.ticker]}
                      onClick={p.self ? undefined : () => onTickerClick?.(p.ticker)}
                    >
                      <td>{p.ticker}</td>
                      <td>
                        {currency}
                        {p.price.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                      </td>
                      <PeerPct v={p.chg20} />
                      <PeerPct v={p.chg60} />
                      <PeerPct v={p.chg120} />
                      {p.self ? <td className="tk-dim">—</td> : <PeerPct v={p.rel} />}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="card" style={{ marginBottom: 16 }}>
            <div className="thesis-header">
              <div className="eyebrow">{dict.notesLabel}</div>
              <span className="note-title-meta">
                {notes.length} {dict.notesCount}
              </span>
              {noteFiltering && (
                <span className="note-title-meta match">
                  {dict.notesMatched} {filteredNotes.length} {dict.notesCount}
                </span>
              )}
              {writable && !noteComposing && (
                <button type="button" className="thesis-edit-btn" onClick={openNoteCompose}>
                  <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
                    <path d="M8 3v10M3 8h10" />
                  </svg>
                  <span>{todayNote ? dict.notesEditToggle : dict.notesAddToggle}</span>
                </button>
              )}
            </div>
            <div className="note-sub">{dict.notesNote}</div>

            {noteComposing && (
              <div className="note-compose">
                <div className="note-chips">
                  {NOTE_TAGS.map((tg) => (
                    <button
                      key={tg}
                      type="button"
                      className={`note-chip ${noteTagClass(tg)}${noteTag === tg ? " active" : ""}`}
                      onClick={() => setNoteTag(tg)}
                    >
                      {noteTagLabel(dict, tg)}
                    </button>
                  ))}
                </div>
                <label className="form-field">
                  <textarea
                    rows={4}
                    autoFocus
                    value={noteDraft}
                    placeholder={dict.notesFieldPlaceholder}
                    onChange={(e) => setNoteDraft(e.target.value)}
                  />
                </label>
                {noteError && <div className="error-message">{noteError}</div>}
                <div className="modal-actions">
                  <button
                    type="button"
                    onClick={() => {
                      setNoteComposing(false);
                      setNoteDraft("");
                      setNoteError(null);
                    }}
                  >
                    {dict.cancel}
                  </button>
                  <button
                    type="button"
                    className="btn-primary"
                    disabled={!noteDraft.trim() || noteSubmitting}
                    onClick={submitNote}
                  >
                    {dict.submit}
                  </button>
                </div>
              </div>
            )}

            <div className="note-toolbar">
              <div className="note-search">
                <svg className="note-search-icon" width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
                  <circle cx="7" cy="7" r="4.2" />
                  <path d="M10.2 10.2 13.5 13.5" />
                </svg>
                <input
                  className="note-search-input"
                  value={noteQuery}
                  placeholder={dict.notesSearchPlaceholder}
                  onChange={(e) => {
                    setNoteQuery(e.target.value);
                    setNoteShow(noteShowInitial);
                  }}
                />
                {noteQuery && (
                  <button
                    type="button"
                    className="note-search-clear"
                    onClick={() => {
                      setNoteQuery("");
                      setNoteShow(noteShowInitial);
                    }}
                  >
                    {dict.notesClearSearch}
                  </button>
                )}
              </div>
              <div className="note-chips">
                <button
                  type="button"
                  className={`note-chip${noteTagFilter === null ? " active" : ""}`}
                  onClick={() => {
                    setNoteTagFilter(null);
                    setNoteShow(noteShowInitial);
                  }}
                >
                  {dict.notesFilterAllLabel} {notes.length}
                </button>
                {noteFilterTags.map((tg) => (
                  <button
                    key={tg}
                    type="button"
                    className={`note-chip ${noteTagClass(tg)}${noteTagFilter === tg ? " active" : ""}`}
                    onClick={() => {
                      setNoteTagFilter(tg);
                      setNoteShow(noteShowInitial);
                    }}
                  >
                    {noteTagLabel(dict, tg)} {notes.filter((n) => n.tag === tg).length}
                  </button>
                ))}
              </div>
            </div>

            {pinnedNotes.length > 0 && (
              <div className="note-pinned-block">
                <div className="note-pinned-head">
                  <svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
                    <path d="M8 10v4M4.5 3.5h7l-1 5h-5z" />
                  </svg>
                  <span className="note-pinned-title">{dict.notesPinnedLabel}</span>
                  <span className="note-pinned-count">
                    {pinnedNotes.length} {dict.notesCount}
                  </span>
                </div>
                <div className="note-list">{pinnedNotes.map((n) => renderNoteRow(n))}</div>
              </div>
            )}

            {visibleNotes.length > 0 && (
              <>
                <div className="note-list">
                  {(() => {
                    lastNoteMonth = "";
                    return visibleNotes.map((n) => {
                      const month = n.date.slice(0, 7);
                      const monthLabel = month !== lastNoteMonth ? noteMonthLabel(dict, month) : undefined;
                      lastNoteMonth = month;
                      return renderNoteRow(n, monthLabel);
                    });
                  })()}
                </div>
                <div className="note-foot">
                  <span>
                    {dict.notesShowing} {visibleNotes.length} / {unpinnedNotes.length}
                  </span>
                  {noteShow > noteShowInitial && (
                    <button type="button" className="note-foot-btn" onClick={() => setNoteShow(noteShowInitial)}>
                      {dict.notesCollapse}
                    </button>
                  )}
                  {unpinnedNotes.length > visibleNotes.length && (
                    <button
                      type="button"
                      className="note-foot-more"
                      onClick={() => setNoteShow((c) => c + noteShowStep)}
                    >
                      {dict.notesLoadMore} · +{unpinnedNotes.length - visibleNotes.length}
                    </button>
                  )}
                  {unpinnedNotes.length > noteShowInitial && unpinnedNotes.length === visibleNotes.length && (
                    <span style={{ marginLeft: "auto" }}>{dict.notesAllShown}</span>
                  )}
                </div>
              </>
            )}

            {filteredNotes.length === 0 && (
              <div className="note-empty">{notes.length === 0 ? dict.notesEmptyNote : dict.notesNoMatch}</div>
            )}
          </div>
        </div>
      </div>
    </>
  );
}
