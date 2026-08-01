import { defineConfig, Plugin } from "vite";
import react from "@vitejs/plugin-react";
import http from "node:http";

function getMockData(urlStr: string): any {
  const parsed = new URL(urlStr, "http://localhost");
  const path = parsed.pathname;
  const ticker = parsed.searchParams.get("ticker") || "AAPL";
  const market = parsed.searchParams.get("market") || "us";

  if (path === "/api/config") {
    return { lang: "zh" };
  }
  if (path === "/api/status") {
    return {
      watchingCount: market === "tw" ? 11 : 14,
      spyChangePct: market === "tw" ? -0.18 : 0.42,
      lastCloseDate: "2026-07-15",
      accountValue: market === "tw" ? 3850000 : 125400,
      netPnL: market === "tw" ? 520000 : 18450,
      winRate: market === "tw" ? 0.581 : 0.682,
      tradeCount: market === "tw" ? 96 : 147,
      writable: true,
    };
  }
  if (path === "/api/company-names") {
    return {
      names: {
        "2330": "台積電",
        "2454": "聯發科",
        "0050": "元大台灣50",
        "2317": "鴻海",
        "3231": "緯創",
        "2382": "廣達",
        "2603": "長榮",
        "3008": "大立光",
      },
    };
  }
  if (path === "/api/dashboard") {
    const dates = Array.from({ length: 30 }, (_, i) => {
      const d = new Date(2026, 6, 1 + i);
      return d.toISOString().slice(0, 10);
    });
    let val = 10000;
    const curve = dates.map((date) => {
      val += (Math.random() - 0.4) * 500;
      return { date, value: Math.round(val) };
    });
    const benchmark = dates.map((date, i) => ({
      date,
      value: Math.round(10000 + i * 200 + (Math.random() - 0.5) * 100),
    }));
    const drawdown = curve.map((c, i) => {
      const peak = Math.max(...curve.slice(0, i + 1).map((x) => x.value));
      return { date: c.date, value: Math.min(0, c.value - peak) };
    });
    return {
      kpis: {
        netPnL: market === "tw" ? 520000 : 18450,
        winRate: market === "tw" ? 0.581 : 0.682,
        profitFactor: market === "tw" ? 1.62 : 2.35,
        expectancy: market === "tw" ? 3870 : 420.5,
        maxDrawdown: market === "tw" ? -68000 : -2300,
        ytdReturnPct: 18.5,
        qtdReturnPct: 6.2,
        htdReturnPct: 12.1,
        benchmarkAlpha: market === "tw" ? 156000 : 5200,
      },
      curve,
      drawdown,
      benchmark,
      positions:
        market === "tw"
          ? [
              { ticker: "2330", shares: 3000, avgCost: 892, price: 1035, marketValue: 3105000, unrealizedPnL: 429000, unrealizedPnLPct: 16.03, stopPrice: 940 },
              { ticker: "2454", shares: 500, avgCost: 1210, price: 1145, marketValue: 572500, unrealizedPnL: -32500, unrealizedPnLPct: -5.37, stopPrice: 1120 },
              { ticker: "0050", shares: 2000, avgCost: 178.4, price: 196.2, marketValue: 392400, unrealizedPnL: 35600, unrealizedPnLPct: 9.98, stopPrice: 0 },
              { ticker: "2317", shares: 4000, avgCost: 168, price: 205.5, marketValue: 822000, unrealizedPnL: 150000, unrealizedPnLPct: 22.32, stopPrice: 190 },
              { ticker: "3231", shares: 2000, avgCost: 118, price: 104.5, marketValue: 209000, unrealizedPnL: -27000, unrealizedPnLPct: -11.44, stopPrice: 102 },
            ]
          : [
              { ticker: "NVDA", shares: 120, avgCost: 118.4, price: 141.22, marketValue: 16946.4, unrealizedPnL: 2738.4, unrealizedPnLPct: 19.27, stopPrice: 128 },
              { ticker: "AAPL", shares: 80, avgCost: 191.5, price: 205.83, marketValue: 16466.4, unrealizedPnL: 1146.4, unrealizedPnLPct: 7.48, stopPrice: 188 },
              { ticker: "MSFT", shares: 40, avgCost: 402.1, price: 431.55, marketValue: 17262.0, unrealizedPnL: 1178.0, unrealizedPnLPct: 7.32, stopPrice: 0 },
              { ticker: "AMD", shares: 150, avgCost: 168.2, price: 152.36, marketValue: 22854.0, unrealizedPnL: -2376.0, unrealizedPnLPct: -9.42, stopPrice: 149.5 },
              { ticker: "TSM", shares: 60, avgCost: 152.9, price: 178.44, marketValue: 10706.4, unrealizedPnL: 1532.4, unrealizedPnLPct: 16.70, stopPrice: 160 },
            ],
    };
  }
  if (path === "/api/watchlist-summary") {
    const list =
      market === "tw"
        ? [
            { ticker: "2330", price: 1035, changePct: 2.4, heldShares: 3000, sup: 940, res: 1080 },
            { ticker: "2454", price: 1145, changePct: -1.2, heldShares: 500, sup: 1120, res: 1220 },
            { ticker: "0050", price: 196.2, changePct: 0.8, heldShares: 2000, sup: 185, res: 205 },
            { ticker: "2317", price: 205.5, changePct: 3.1, heldShares: 4000, sup: 190, res: 215 },
            { ticker: "3231", price: 104.5, changePct: -2.3, heldShares: 2000, sup: 102, res: 115 },
            { ticker: "2382", price: 298.5, changePct: 1.5, heldShares: 0, sup: 280, res: 315 },
            { ticker: "2603", price: 188.0, changePct: -0.5, heldShares: 0, sup: 175, res: 198 },
          ]
        : [
            { ticker: "NVDA", price: 141.22, changePct: 3.42, heldShares: 120, sup: 128.0, res: 150.0 },
            { ticker: "AAPL", price: 205.83, changePct: 1.25, heldShares: 80, sup: 188.0, res: 215.0 },
            { ticker: "MSFT", price: 431.55, changePct: -0.45, heldShares: 40, sup: 410.0, res: 445.0 },
            { ticker: "AMD", price: 152.36, changePct: -1.85, heldShares: 150, sup: 149.5, res: 165.0 },
            { ticker: "TSM", price: 178.44, changePct: 2.15, heldShares: 60, sup: 160.0, res: 185.0 },
            { ticker: "META", price: 512.30, changePct: 1.80, heldShares: 0, sup: 480.0, res: 535.0 },
            { ticker: "GOOG", price: 176.50, changePct: -0.90, heldShares: 0, sup: 165.0, res: 185.0 },
          ];

    return {
      tickers: list.map((item) => {
        let p = item.price * 0.95;
        const sparkline = Array.from({ length: 44 }, () => {
          p += (Math.random() - 0.48) * (item.price * 0.02);
          return Number(p.toFixed(2));
        });
        return {
          ticker: item.ticker,
          price: item.price,
          changePct: item.changePct,
          sparkline,
          support: item.sup,
          resistance: item.res,
          heldShares: item.heldShares,
        };
      }),
    };
  }
  if (path === "/api/tickers") {
    return {
      tickers:
        market === "tw"
          ? ["2330", "2454", "0050", "2317", "3231", "2382", "2603"]
          : ["NVDA", "AAPL", "MSFT", "AMD", "TSM", "META", "GOOG"],
    };
  }
  if (path === "/api/chart") {
    const isTW = ticker === "2330" || ticker === "2454" || ticker === "2317" || ticker === "0050" || ticker === "3231";
    const basePrice = isTW ? 900 : 180;
    const candles: any[] = [];
    let cur = basePrice;
    for (let i = 120; i >= 0; i--) {
      const d = new Date(2026, 2, 1);
      d.setDate(d.getDate() + (120 - i));
      const open = cur + (Math.random() - 0.5) * 2;
      const high = open + Math.random() * 4;
      const low = open - Math.random() * 4;
      const close = (high + low) / 2;
      cur = close;
      candles.push({
        date: d.toISOString().slice(0, 10),
        open: Number(open.toFixed(2)),
        high: Number(high.toFixed(2)),
        low: Number(low.toFixed(2)),
        close: Number(close.toFixed(2)),
        volume: Math.floor(Math.random() * 5000000 + 1000000),
      });
    }
    return {
      ticker,
      candles,
      levels: [
        { price: Number((basePrice * 0.92).toFixed(2)), touches: 4, firstDate: "2026-03-10", lastDate: "2026-06-15" },
        { price: Number((basePrice * 1.08).toFixed(2)), touches: 3, firstDate: "2026-04-01", lastDate: "2026-07-02" },
      ],
      position: {
        ticker,
        shares: 80,
        avgCost: Number((basePrice * 0.95).toFixed(2)),
        price: Number(cur.toFixed(2)),
        value: Number((cur * 80).toFixed(2)),
        weightPct: 14.5,
        stopPrice: Number((basePrice * 0.9).toFixed(2)),
        openRisk: Number((cur * 80 * 0.05).toFixed(2)),
        openRiskPct: 0.72,
        unrealizedPnLPct: 5.26,
      },
      rounds: [
        {
          ticker,
          start: "2026-05-04",
          end: "2026-06-26",
          open: false,
          shares: 80,
          realizedPnL: isTW ? 148000 : 4820,
        },
        {
          ticker,
          start: "2026-02-10",
          end: "",
          open: true,
          shares: 80,
          realizedPnL: 0,
        },
      ],
    };
  }
  if (path === "/api/round-detail") {
    const start = parsed.searchParams.get("start") || "2026-05-04";
    const isTW = ticker === "2330" || ticker === "2454" || ticker === "2317";
    const basePrice = isTW ? 900 : 180;
    const candles: any[] = [];
    for (let i = 30; i >= 0; i--) {
      const d = new Date(start);
      d.setDate(d.getDate() + (30 - i));
      candles.push({
        date: d.toISOString().slice(0, 10),
        open: basePrice,
        high: basePrice + 10,
        low: basePrice - 5,
        close: basePrice + 5,
        volume: 2000000,
      });
    }
    return {
      ticker,
      start,
      end: "2026-06-26",
      candles,
      trades: [
        { date: start, ticker, side: "BUY", shares: 80, price: basePrice, fee: 5, realizedPnL: 0 },
        { date: "2026-06-26", ticker, side: "SELL", shares: 80, price: basePrice + 15, fee: 5, realizedPnL: 4820 },
      ],
      maePct: -3.4,
      mfePct: 14.2,
      hasMaeMfe: true,
      thesis: "Base breakout on rising volume; holding while the 20d holds and the earnings guide stays intact. Trim half into the prior high.",
      lessons: [
        { date: "2026-06-02", lesson: "Stop was too tight relative to ATR — shaken out before the real move." },
        { date: "2026-06-19", lesson: "Adding on strength worked; adding on weakness did not." },
      ],
    };
  }
  if (path === "/api/risk") {
    return {
      accountValue: market === "tw" ? 3850000 : 125400,
      cash: market === "tw" ? 850000 : 25000,
      heatPct: market === "tw" ? 8.4 : 6.2,
      heatThresholdPct: 8.0,
      positions:
        market === "tw"
          ? [
              { ticker: "2330", shares: 3000, avgCost: 892, price: 1035, value: 3105000, weightPct: 61.5, stopPrice: 940, openRisk: 285000, openRiskPct: 7.4, unrealizedPnLPct: 16.03 },
              { ticker: "2454", shares: 500, avgCost: 1210, price: 1145, value: 572500, weightPct: 11.3, stopPrice: 1120, openRisk: 12500, openRiskPct: 0.32, unrealizedPnLPct: -5.37 },
            ]
          : [
              { ticker: "NVDA", shares: 120, avgCost: 118.4, price: 141.22, value: 16946.4, weightPct: 24.5, stopPrice: 128, openRisk: 1586.4, openRiskPct: 1.26, unrealizedPnLPct: 19.27 },
              { ticker: "AAPL", shares: 80, avgCost: 191.5, price: 205.83, value: 16466.4, weightPct: 23.8, stopPrice: 188, openRisk: 1426.4, openRiskPct: 1.14, unrealizedPnLPct: 7.48 },
            ],
    };
  }
  if (path === "/api/rounds") {
    return {
      rounds:
        market === "tw"
          ? [
              { ticker: "2330", start: "2026-05-04", end: "2026-06-26", open: false, shares: 2000, realizedPnL: 148000 },
              { ticker: "2454", start: "2026-04-13", end: "2026-05-22", open: false, shares: 500, realizedPnL: -38500 },
              { ticker: "2317", start: "2026-03-02", end: "2026-04-24", open: false, shares: 3000, realizedPnL: 96000 },
              { ticker: "0050", start: "2026-02-10", end: "", open: true, shares: 2000, realizedPnL: 0 },
            ]
          : [
              { ticker: "NVDA", start: "2026-05-04", end: "2026-06-26", open: false, shares: 80, realizedPnL: 4820 },
              { ticker: "AMD", start: "2026-04-13", end: "2026-05-22", open: false, shares: 100, realizedPnL: -1240 },
              { ticker: "MSFT", start: "2026-03-02", end: "2026-04-24", open: false, shares: 30, realizedPnL: 3110 },
              { ticker: "AAPL", start: "2026-02-10", end: "", open: true, shares: 80, realizedPnL: 0 },
            ],
    };
  }
  if (path === "/api/reports") {
    const mult = market === "tw" ? 30 : 1;
    return {
      byTicker: [
        { key: market === "tw" ? "2330" : "NVDA", n: 5, winRate: 0.8, profitFactor: 3.2, avgReturnPct: 8.5, totalRealizedPnL: 4820 * mult, avgHoldingDays: 22, lowSample: false },
        { key: market === "tw" ? "2454" : "AAPL", n: 4, winRate: 0.75, profitFactor: 2.5, avgReturnPct: 5.2, totalRealizedPnL: 2450 * mult, avgHoldingDays: 15, lowSample: false },
      ],
      byHoldingDays: [
        { key: "11-25d", n: 8, winRate: 0.75, profitFactor: 2.8, avgReturnPct: 6.2, totalRealizedPnL: 5100 * mult, avgHoldingDays: 18, lowSample: false },
      ],
      byEntryMonth: [
        { key: "2026-05", n: 4, winRate: 0.75, profitFactor: 2.5, avgReturnPct: 5.8, totalRealizedPnL: 4820 * mult, avgHoldingDays: 20, lowSample: false },
      ],
      byEntryWeekday: [
        { key: "Monday", n: 6, winRate: 0.67, profitFactor: 2.1, avgReturnPct: 4.5, totalRealizedPnL: 3200 * mult, avgHoldingDays: 16, lowSample: false },
      ],
      fees: { totalFees: 45 * mult, pctOfRealizedPnL: 11.3 },
      streaks: { bestTradePnL: 4820 * mult, worstTradePnL: -1240 * mult, avgWinPnL: 2950 * mult, avgLossPnL: -850 * mult, longestWinStreak: 8, longestLossStreak: 4 },
      maeMfe: { avgCapturedPct: 48.2, n: 34, lowSample: false },
    };
  }
  if (path === "/api/monthly") {
    const mult = market === "tw" ? 30 : 1;
    return {
      years: [
        { year: 2024, months: [1200 * mult, -450 * mult, 3200 * mult, 1800 * mult, -900 * mult, 4100 * mult, 2500 * mult, 3100 * mult, -1200 * mult, 5400 * mult, 2200 * mult, 3800 * mult], total: 24850 * mult },
        { year: 2025, months: [3100 * mult, 2800 * mult, -1500 * mult, 4200 * mult, 3600 * mult, -800 * mult, 5100 * mult, 4300 * mult, 2900 * mult, -600 * mult, 4800 * mult, 6200 * mult], total: 34100 * mult },
        { year: 2026, months: [4200 * mult, 3800 * mult, -1100 * mult, 5200 * mult, 4800 * mult, -900 * mult, 4500 * mult, null, null, null, null, null], total: 20500 * mult },
      ],
    };
  }
  if (path === "/api/distributions") {
    const mult = market === "tw" ? 30 : 1;
    return {
      rMultiples: [{ ticker: market === "tw" ? "2330" : "NVDA", r: 2.5 }, { ticker: market === "tw" ? "2454" : "AMD", r: -0.8 }],
      noStopCount: 1,
      earliestRDate: "2025-11-04",
      holdingReturns: [{ ticker: market === "tw" ? "2330" : "NVDA", holdingDays: 22, realizedPnL: 4820 * mult }],
      maeReturns: [{ ticker: market === "tw" ? "2330" : "NVDA", maePct: -3.4, returnPct: 14.2 }],
      skippedMaeCount: 0,
    };
  }
  if (path === "/api/rec-performance") {
    return {
      counts: { total: 312, scorable: 248, unscorable: 31, hold: 33 },
      horizons: [5, 10, 20],
      bySource: [],
      byAction: [],
      best: [],
      worst: [],
    };
  }
  if (path === "/api/login") {
    return { ok: true };
  }
  return { message: "Mock operation completed successfully." };
}

function mockApiPlugin(): Plugin {
  return {
    name: "mock-api-plugin",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (!req.url || !req.url.startsWith("/api")) {
          return next();
        }

        const proxyReq = http.request(
          `http://127.0.0.1:8090${req.url}`,
          { method: req.method, headers: req.headers, timeout: 500 },
          (backendRes) => {
            res.writeHead(backendRes.statusCode || 200, backendRes.headers);
            backendRes.pipe(res);
          }
        );

        const serveMock = () => {
          const mockData = getMockData(req.url || "");
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(mockData));
        };

        proxyReq.on("error", serveMock);
        proxyReq.on("timeout", () => {
          proxyReq.destroy();
          serveMock();
        });

        if (req.method === "POST" || req.method === "PUT") {
          req.pipe(proxyReq);
        } else {
          proxyReq.end();
        }
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), mockApiPlugin()],
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
});
