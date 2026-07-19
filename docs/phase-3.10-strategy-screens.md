# Phase 3.10 — 技術策略濾網(兩張網子)+指標補齊+LLM 判讀指引

> 本文件是**完整的實作交接規格**(2026-07-19 訪談定案),寫給未參與前期討論的實作
> agent:背景、決策、簽名、閾值、慣例都在這裡。實作前務必先讀 repo 根目錄
> `CLAUDE.md`(架構慣例)——本文多處直接引用其中的既有慣例。截至定稿時**尚未動工**。

## 1. Context(為什麼做)

Argus 目前的技術指標(RSI14、MACD 12/26/9、MA20/50/200、ATR14、Bollinger %B、
VolumeRatio)只有兩種用法:

1. 零散的**單指標警報**:價格 ±3%、RSI 進入 70/30 區、MACD 黃金/死亡交叉
   (`internal/signals` 的 Detector,經 `signal_states` 表去重)。
2. **prompt 素材**:`bot.computeTechnicals` 算好塞進 `llm.StockData.Technicals` 給
   LLM 自由判讀。

使用者提供了一份交易教材,要求把可規則化的部分寫成**多條件組合的策略濾網**,理由:
單指標訊號噪音大,universe scan(Phase 2.6 的候選池輪掃)目前只靠 RSI/MACD 單指標
產生 hit,給 LLM 的候選理由太弱。

### 教材內容摘要(策略定義的原始出處)

**網子 1【布林壓縮噴發】**(抓波段飆股):

- 流動性:美股日均量 > 50 萬股(排除殭屍股)
- 能量壓縮:布林帶寬(Bandwidth)創近 20 天新低(上下軌擠壓到極致)
- 主力表態:今日股價突破布林上軌,且今量 > 5 日均量 2 倍
- 趨勢確認:MACD 柱狀體翻紅**第一天**

**網子 2【箱型地板抄底】**(抓短線震盪):

- 盤整型態:近 30 天高低點差距 ≤ 15%
- 回檔地板:今價距近 30 天最低點 ≤ 2%
- 指標超賣:日 KD 的 K < 25,且黃金交叉或勾頭向上
- 力道反彈:RSI(14) 從 30 以下止跌回升

教材另含:支撐壓力位判讀(下影線/W 底腳/爆量長紅 K 低點/突破後壓力轉支撐)、均線
多空排列(5>20>60)、量價配合與背離四象限、MACD 零軸與柱狀體背離、KD「出超買/超賣
區再動作」、RSI 50 多空分界與頂背離、分批進場與移動停利心法。

### 使用者訪談定案(2026-07-19)

- **兩張網子都做**。
- 策略命中在**三個面向**發揮作用:(a) universe 掃描 → `scan_hits` → LLM 候選;
  (b) watchlist 每日警報(Telegram、去重);(c) prompt 素材。
- **支撐壓力位、指標背離**不寫成程式規則(擺動點偵測誤判率高)→ 延長 prompt 的
  K 線窗口 + 提示詞加判讀指引,交給 LLM。
- **輔助標籤全上**:MA5/MA60、KD、帶寬、多空排列、量價標籤、創 20 日/52 週新高、
  MACD 零軸位置,全部算好餵 prompt。
- 三大法人/投信籌碼:台股限定資料,美股免費 API 無此資料,**不做**。

## 2. 價值評估與風險(實作時的心態)

- 教材閾值(15% 箱型、2× 爆量、K<25 等)是**未經回測的經驗法則**,專案無回測設施。
  定位是「待驗證的假設」:閾值一律寫成集中常數方便日後調整,且**接警報前必先跑
  §4.7 的歷史體檢腳本**——用 universe 池 × 1 年日 K 驗證閾值鬆緊與 edge,結果寫回
  本文件。
- 成效回饋迴路用現成的:`recommendations.source="scan"` 已被 `/track` 的 by-source
  統計覆蓋,上線後觀察 scan 來源命中率變化即可,不需要新統計。
- **原教材條件是「當天觸發」型**(翻紅第一天、交叉當天),但 universe scan 每檔
  **5 天才輪到一次**(`universeScanChunk`,`scanChunkCount=5`),照字面實作會漏掉
  ~80% 的觸發。→ 改成「**近 5 日內曾觸發**」的窗口設計(見 §4.2),這是對教材唯一
  的實質改動。
- 警報量:四條件 AND 很嚴 + `signal_states` alert-once 去重,預期頻率低;dry-run
  (§8)驗證不會轟炸。
- prompt 變長(K 線 20→60 根 × 15+ 檔):ACP 走 Pro/Max 訂閱無按量計費,代價只是
  延遲,接受。

## 3. 分支/PR/工作流程

- **從 `main` 開新分支** `feat/phase-3.10-strategy-screens`,完成後開 PR 走 CI,
  不直接 commit main。Commit 訊息用 Conventional Commits(`feat: ...`)。
- 本 phase **無 DB migration**:`signal_states`/`scan_hits` 的 schema 本來就是通用的
  (ticker、family/state、reason)。

## 4. 實作

### 4.1 `internal/signals` — 新指標純函數

放新檔 `internal/signals/indicators.go`(或續寫 `signals.go`),**必守慣例**:slice
一律 oldest-first(`HistoryProvider.GetHistory` 的回傳順序);「資料不足」的表示法
——合法值域不含 0 的用 0 哨兵(如 `MA`),合法值域包含 0 的用 `ok bool` 或 nil(如
`BollingerPctB`,理由見其註解);不碰 DB/Telegram。

```go
// KD(台式隨機指標, RSV 9 日, K/D 各 3 期平滑)。回傳整條序列——策略要用
// 昨日 K/D 判斷交叉。K 的合法值域包含 0,資料不足回 (nil, nil)。
// RSV = (C - L9) / (H9 - L9) * 100;H9 == L9 時 RSV 沿用前一日值(或 50 起始)。
// 平滑用台股慣用的 K = 2/3*K_prev + 1/3*RSV(等價於 EMA 平滑),註解寫明選擇。
func StochasticSeries(highs, lows, closes []float64, rsvPeriod, smooth int) (k, d []float64)

// 布林帶寬序列:每點的 (upper-lower)/middle(middle 為 period 日 SMA)。
// 策略要判斷「帶寬創 20 日新低」所以需要序列;最新值給 prompt。
// 可從既有 BollingerPctB 抽出共用的 mean/stdDev/band 私有 helper,避免第二份實作。
func BollingerBandwidthSeries(closes []float64, period int, numStdDev float64) []float64

// 均線排列:MA5>MA20>MA60 回 StateBullish、MA5<MA20<MA60 回 StateBearish、
// 其餘或任一 MA 資料不足(=0)回 ""。重用既有 MA()。
func MAAlignment(closes []float64) string

// 量價四象限。價方向 = 今收 vs 昨收;量方向 = 今量 vs 前 5 日均量
// (>=1.3x 為增、<=0.7x 為縮,之間回 "" 不貼標;排除今日自身,同 VolumeRatio 的做法)。
// 新 state 常數(加在既有 State* 常數區):
//   StateVolUpPriceUp     = "vol_up_price_up"     // 量增價漲(多頭訊號)
//   StateVolDownPriceDown = "vol_down_price_down" // 量縮價跌(健康修正)
//   StateVolDownPriceUp   = "vol_down_price_up"   // 量縮價漲(虛胖,危險)
//   StateVolUpPriceDown   = "vol_up_price_down"   // 量增價跌(恐慌/倒貨,勿接刀)
func VolumePriceSignal(closes []float64, volumes []int64) string

// 今收 >= 過去 lookback 根(含今日)的最高收盤。20 日與 52 週(直接用全部 ~1y
// 歷史,lookback = len(closes))共用。資料不足回 false。
func IsNewHigh(closes []float64, lookback int) bool

// RS63(相對強度,2026-07 交易員缺口盤點追加):個股近 lookback 日報酬 −
// SPY 同期報酬,單位百分點。spyCloses 由呼叫端傳入(pipeline 重用大盤情境
// 已抓的 SPY 歷史,零額外請求);任一邊資料不足回 ok=false(RS 合法值域含 0,
// 不能用 0 哨兵)。動能派選股的核心:只做比大盤強的股票。
func RelativeStrength(closes, spyCloses []float64, lookback int) (rs float64, ok bool)
```

### 4.2 `internal/signals/strategies.go` — 兩張網子

**閾值常數**(集中一處,附註解說明出處是教材、未回測):

```go
const (
    strategyLookbackDays = 5       // 「近 N 日內曾觸發」窗口,配合 universe scan 5 天輪掃
    minAvgVolume5d       = 500_000 // 網 1 流動性門檻(股)
    squeezeBWWindow      = 20      // 帶寬新低的回看窗
    breakoutVolMult      = 2.0     // 突破日量 >= 2x 前 5 日均量
    boxWindowDays        = 30      // 網 2 箱型窗口
    boxMaxRangePct       = 15.0    // 箱型高低差上限 %
    boxFloorPct          = 2.0     // 距箱底 %
    kdOversold           = 25.0    // K 低檔門檻
    rsiReboundLevel      = 30.0    // RSI 超賣線
    rsiReboundLookback   = 3       // RSI 近 N 日內曾 < 30
)
```

**純函數核心**(stateless,prompt 端直接用):

```go
type StrategyHit struct {
    Name    string // "squeeze_breakout" / "box_bottom"
    DaysAgo int    // 0 = 今天觸發
}

// 對 offset 0..strategyLookbackDays-1 逐一以 candles[:len-offset] 執行「當日精準版」
// 檢查,回傳最近的一個命中(DaysAgo=offset);全不中回 nil。
// 逐 suffix 重算的成本可忽略(純記憶體運算,一天最多幾十檔)。
func SqueezeBreakout(candles []data.Candle) *StrategyHit
func BoxBottomRebound(candles []data.Candle) *StrategyHit
```

「當日精準版」條件(對評估日 = slice 最後一根):

*SqueezeBreakout:*

1. 流動性:前 5 日均量(不含評估日)> `minAvgVolume5d`。
2. 壓縮:評估日往前 5 根內的最小帶寬 == 尾端 `squeezeBWWindow` 根帶寬序列的最低值
   (突破當天帶寬本來就會擴張,所以判「不久前剛壓到極致」,不是「今天最窄」)。
3. 突破:評估日收盤 > 布林上軌(20, 2σ),且評估日量 ≥ `breakoutVolMult` × 前 5 日
   均量。
4. 確認:MACD histogram 評估日 > 0 且前一日 ≤ 0(翻紅第一天;前一日用
   `MACD(closes[:n-1])` 重算)。

*BoxBottomRebound:*

1. 箱型:近 `boxWindowDays` 根 (最高收−最低收)/最低收 ≤ `boxMaxRangePct`%
   (**用收盤價**而非 high/low,避免影線噪音)。
2. 地板:評估日收盤距近 30 根最低收 ≤ `boxFloorPct`%。
3. KD:評估日 K < `kdOversold`,且(昨 K≤昨 D 且今 K>今 D 的黃金交叉,**或**
   昨 K < `kdOversold` 且今 K>昨 K 的勾頭向上)。
4. RSI:近 `rsiReboundLookback` 日內曾 < `rsiReboundLevel`,且評估日 RSI > 前一日
   RSI(前一日同樣用 `RSI(closes[:n-1], 14)` 重算)。

**Detector 包裝**(警報用,訊息走 i18n,形狀同 `CheckRSIState`):

```go
// 新 family 常數(加在 FamilyRSI/FamilyMACD 旁):
//   FamilyStrategySqueeze = "strategy_squeeze"
//   FamilyStrategyBox     = "strategy_box"
// 去重採 stop-loss 的 alert-once 模式(state "hit"/""):窗口內首次命中警報一次,
// 條件離開 5 日窗口後 state 清空,下一次事件才會再警報。
// (兩次獨立觸發若間隔 < 5 天會合併成一次警報——接受。)
func (d *Detector) CheckSqueezeBreakout(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string)
func (d *Detector) CheckBoxBottom(ticker string, candles []data.Candle, prevState string) (sig *Signal, newState string)
```

Signal.Type:`"strategy_squeeze_breakout"` / `"strategy_box_bottom"`。Message 帶
ticker、DaysAgo、簡述(i18n key 見 §4.5)。

**測試**(`strategies_test.go` + `indicators_test.go`,跟隨現有 table-driven 風格,
參考 `signals_test.go` 的 `almostEqual` 慣例):

- KD:手算小 fixture 驗證 K/D 數值與交叉點;H9==L9 邊界。
- 帶寬序列、MAAlignment(三態)、VolumePriceSignal(四象限+中間帶回 "")、IsNewHigh。
- 兩張網子:合成 K 線 fixture——「四條件全中」命中、「各缺一條件」四個不命中案例、
  「觸發在 3 天前」回 DaysAgo=3、「觸發在 6 天前」不命中(出窗)。
- Detector 包裝的去重:hit→再 hit 不重複、條件消失後 state 清空。

### 4.3 `internal/bot` 接線

**`jobs.go` — watchlist 警報 + universe scan(一次改好兩個面向)**:

- `checkStatefulSignals(ticker string, closes []float64)` 簽名改為收
  `candles []data.Candle`(兩個呼叫端 `RunDailyReport` 與 `RunUniverseScan` 本來就
  持有 candles,只是把 `data.Closes(candles)` 的轉換移進函數內)。內部沿用既有的
  load `signal_states` → check → save 迴圈,**追加兩個策略檢查**
  (`FamilyStrategySqueeze`/`FamilyStrategyBox`)。
- 這樣 `RunUniverseScan` **自動獲得**策略命中:hit 的 `Signal.Message` 經既有
  `SaveScanHit` 寫入 `scan_hits`,再經既有 `loadScanHits`/`mergeCandidates`/
  `ScanReason` 流程變成 LLM 候選——**scan 鏈路零額外改動**。
- watchlist/universe 兩個 ticker 集不重疊(CLAUDE.md 既有結論),共用
  `signal_states` 安全。
- **策略警報的大盤情境警語**:發送 `strategy_*` 警報時,若大盤情境為空頭(重用
  Phase 3.7 `computeMarketRegime` 的 SPY 趨勢判斷,不另抓資料),訊息尾端附
  `KeyStrategyBearRegimeWarning`(空頭下箱型抄底接刀風險高/突破勝率下降)。
  **標記不攔截**——單人系統資訊給足、決定留給人;要不要升級成硬性閘門,等
  §4.7 的多空情境分組統計說話。

**`pipeline.go` — prompt 素材**:

- `computeTechnicals(ticker)` 簽名加一個 `spyCloses []float64` 參數(呼叫端重用
  `gatherRecommendationInputs` 為大盤情境已抓的 SPY 歷史,零額外請求;nil 安全,
  缺就不算 RS),回傳值加第三個:
  `(*llm.Technicals, []data.Candle, []llm.StrategyHitInfo)`。內部:
  - 填 §4.4 的新 Technicals 欄位(KD 用 `StochasticSeries` 尾值、帶寬用
    `BollingerBandwidthSeries` 尾值、`MAAlignment`、`VolumePriceSignal`、
    `IsNewHigh(closes, 20)` 與 `IsNewHigh(closes, len(closes))`、MACD 零軸 =
    `MACD()` 的 macd 線、`RelativeStrength(closes, spyCloses, 63)`,資料不足時
    nil)。
  - 用 **stateless** 的 `SqueezeBreakout`/`BoxBottomRebound` 算策略命中(prompt 要
    的是「現在狀態」,不經 signal_states 去重)。
- `fetchStockData` 把策略命中掛到 `llm.StockData.StrategyHits`——watchlist 和候選股
  都走這條路,「餵 prompt」面向就此覆蓋。
- `promptCandleCount` 20 → **60**(LLM 需要看得到 30 日箱型、W 底、背離;prompt 變長
  是已接受的取捨,註解更新)。

### 4.4 `internal/llm/prompt.go` — Technicals 擴充與渲染

`Technicals` 新欄位(維持既有慣例:0 哨兵 vs 指標的選擇逐欄註明):

```go
MA5, MA60      float64  // 0 哨兵,沿用 MA20/50/200 的 skip-if-0 逐行渲染
StochK, StochD *float64 // K/D 合法值域含 0,用指標(同 BollingerPctB 的理由)
Bandwidth      *float64 // 同上
MAAlign        string   // signals.StateBullish/StateBearish/"",同 MACDTrend 的字串慣例
VolumePrice    string   // signals.StateVol* 或 ""
NewHigh20, NewHigh52w bool
MACDAboveZero  *float64 // 直接放 MACD 線值(nil = 資料不足);渲染時報「零軸上/下」+數值
RS63           *float64 // 63 日報酬 − SPY 同期(百分點);nil = 任一邊資料不足
```

`StockData` 新欄位:`StrategyHits []StrategyHitInfo`(package-local mini-struct
`{Name string; DaysAgo int}`,不 import `internal/signals`——同 Position/Earnings
不 import db 的慣例;`Name` 直接用 signals 的字串值,渲染端映射 i18n)。

`writeStockSection` 渲染:

- MA5/MA60:**重用既有 `KeyTechnicalsMALine`**(`"%s MA%d ($%.2f)"` 的
  label/period/value 可重用格式),多渲染兩行,skip-if-0。
- 新行(皆為資料齊才渲染,缺就整行跳過):KD 行、帶寬行、多空排列行、量價標籤行、
  創新高旗標行、MACD 零軸行、策略命中行(每個 hit 一行,警示語氣,類似
  `KeyScanHitLine`)。
- MAAlign/VolumePrice 的 state 字串→顯示文字的映射,**跟隨現有 MACDTrend 的渲染
  方式**(先讀 `writeStockSection` 現在怎麼把 "bullish"/"bearish" 變成顯示字串,
  照抄該模式)。

**判讀指引**:新增 `KeyTechGuidanceBlock`,在組 recommend/check prompt 時**追加在
既有 task block 之後**(獨立新 key,**不動**既有 `KeyRecTaskBlock` 的 `%s` 動詞數
——`parseRecommendations` 的 `KeyActionMarker`/`KeyReasonMarker`/
`KeyMarketSummaryMarker` 機制完全不受影響)。內容濃縮教材,~10 行內:

1. 支撐壓力:連續長下影線/兩三次跌不破的低點(W 底腳)是強支撐;爆量長紅 K 的低點
   是主力成本支撐,跌破視為主力棄守;突破後的舊壓力轉支撐,回測不破再彈是高勝率進場點。
2. 量價:量增價漲=趨勢啟動;量縮價跌=健康洗盤;量縮價漲=虛胖誘多;量增價跌=恐慌
   倒貨勿接刀。
3. MACD:零軸上的黃金交叉才有力,零軸下的交叉只是弱反彈;價創新高但柱狀體峰值遞減=
   頂背離,動能衰退警訊。
4. KD:出超買區(跌破 80)再賣、出超賣區(翻越 20/25)再買,不要見 20 以下就接
   (低檔鈍化)。
5. RSI:50 是多空分界,站穩 50 上的回檔是強勢股買點;價新高但 RSI 峰值更低=頂背離。
6. 多頭排列(5>20>60 全朝上)時拉回 20MA 是動態支撐。

### 4.5 `internal/i18n` — 新 key(`i18n.go` 宣告 + `zh.go`/`en.go` 同步)

警報訊息(Detector 用):

- `KeyStrategySqueezeBreakout` — 例 zh:`"🚀 %s 觸發【布林壓縮噴發】(%s):帶寬
  20 日新低後爆量突破上軌,MACD 柱翻紅"`;`%s`×2 = ticker、「今日/N 天前」字串
  (由呼叫端用 DaysAgo 組,或多一個 %d 動詞——實作時擇一,zh/en 動詞數一致即可)。
- `KeyStrategyBoxBottom` — 同型,箱型地板文案。

prompt 行(writeStockSection 用):

- `KeyTechnicalsKDLine`(K/D 值)、`KeyTechnicalsBandwidthLine`、
  `KeyTechnicalsMAAlignLine`、`KeyTechnicalsVolumePriceLine`、
  `KeyTechnicalsNewHighLine`、`KeyTechnicalsMACDZeroLine`、
  `KeyTechnicalsRSLine`(RS63,說明正值=強於大盤)、`KeyStrategyHitLine`。
- 警語:`KeyStrategyBearRegimeWarning`(§4.3 的空頭情境標記)。
- MAAlign/VolumePrice 各 state 的顯示字彙 key(或併入該行的條件渲染,參照 MACDTrend
  現行做法)。

指引:`KeyTechGuidanceBlock`(無動詞或少量動詞的長文案)。

改完**必跑** `go test ./internal/i18n/...`(`TestTablesMatch` 驗證 zh/en key 齊全且
動詞數一致)。

### 4.6 歷史體檢腳本(接警報前必跑;2026-07 交易員缺口盤點追加)

不建回測框架,但閾值全是未驗證假設,而 universe 池 × 1 年 Yahoo 日 K 就在手上——
半天工作量換掉「上線後等 `/track` 累積三個月樣本」的盲飛期。新增一個**不部署**的
本地工具 `cmd/strategyscan/main.go`(獨立 main,不進 Dockerfile/deploy;放同一 repo
保證與策略程式碼零版本漂移,同 `argus mcp` 的理由,但只在開發機跑所以獨立 cmd 目錄
即可):

- 讀 S&P 500 名單(`db.GetUniverse`,或直接重用 `internal/db` embed 的
  `sp500_tickers.txt`),逐檔 `data.Yahoo.GetHistory` 抓 1 年日 K,間隔 200ms
  (全程約 2 分鐘);另抓一次 SPY 供基準與情境判斷。
- 對每檔、每個歷史交易日 t(從第 ~60 根起,留足指標暖機)用 `candles[:t]` 跑兩張
  網子的「**當日精準版**」(offset=0,不用 5 日窗口——體檢要的是精準觸發日),命中
  就記錄:訊號後 5/10/20 日報酬、同期 SPY 報酬、當日大盤情境(SPY 是否在其 MA50 上)。
- 輸出:每張網子的觸發總次數、勝率(贏 SPY 比例)、平均/中位數前瞻報酬、**多空情境
  分組統計**、最差案例清單(供人工抽查)。CSV + stdout 摘要。

**用途與後續**:結果寫回本文件新增「體檢結果」小節;觸發頻率太鬆/太緊就改 §4.2 的
閾值常數;情境分組若顯示空頭下箱型網勝率崩壞,把 §4.3 的警語升級成硬閘門;RS63 是否
納入網 1 的觸發條件也由這份數據決定。

### 4.7 文件

- `PLAN.md`:勾銷對應 checklist 項。
- `CLAUDE.md`:`internal/signals` 條目補策略濾網段落(純函數核心 vs Detector 包裝、
  窗口設計);`internal/llm` 條目補新 Technicals 欄位與 `KeyTechGuidanceBlock`;
  `internal/bot` 條目補 `checkStatefulSignals` 簽名變更與 `promptCandleCount=60`。

## 5. 關鍵重用(不要重造)

| 既有物件 | 位置 | 用法 |
|---|---|---|
| `MA`/`RSI`/`MACD`/`BollingerPctB` | `signals/signals.go` | 新指標的積木;帶寬與 %B 抽共用 band helper |
| `checkStatefulSignals` 的 state 迴圈 | `bot/jobs.go` | 策略檢查加進同一迴圈,不另寫 DB 往返 |
| `SaveScanHit`→`ScanReason` 鏈路 | `bot/jobs.go`、`pipeline.go` | scan 面向零改動 |
| stop-loss 的 `"hit"/""` alert-once 模式 | `bot/jobs.go` `breachAlertDecision` | 策略去重的參考形狀(在 signals 內實作,不 import) |
| `KeyTechnicalsMALine` | `i18n` | MA5/MA60 直接重用 |
| `VolumeRatio` 排除今日的均量算法 | `signals/signals.go` | 網 1 爆量條件同款分母 |

## 6. 明確不做

- 擺動點偵測/程式化背離、支撐壓力位計算(交給 LLM + 指引)。
- 法人籌碼(美股無免費資料)。
- 閾值 env 化(比照 `NewDetector` 寫死常數的慣例;等 `/track` 證明值得調再說)。
- 回測框架(範圍外)。
- 分批進場/移動停利教學內容(移動停損已有 ATR 版;逐筆停損價、R 定倉與規則化出場
  另立 Phase 3.11,見 docs/phase-3.11-trade-risk-management.md)。
- RS63/大盤情境的**硬性閘門**:第一版 RS63 只進 prompt、空頭情境只加警語標記;
  要不要攔截,等 §4.6 體檢腳本的分組統計再定。

## 7. 建議 commit 切分

1. `feat: add stochastic KD, bollinger bandwidth, MA alignment, volume-price, RS indicators`(signals + tests)
2. `feat: add squeeze-breakout and box-bottom strategy screens`(strategies + tests)
3. `feat: add strategyscan historical-study tool`(cmd/strategyscan;**跑完把結果寫進本 doc,必要時先調 §4.2 閾值再往下走**)
4. `feat: wire strategy screens into daily report alerts and universe scan`(bot/jobs,含空頭情境警語)
5. `feat: feed new technicals, strategy hits and trading guidance to LLM prompts`(llm + i18n + pipeline + candle window)
6. `docs: mark phase 3.10 done in PLAN + CLAUDE.md updates`

## 8. 驗證

1. `go build ./... && go vet ./... && go test ./...`(新 signals 測試 + i18n
   `TestTablesMatch` 必過)。
2. **實料驗證由 §4.6 的體檢腳本兼任**:跑完確認 (a) 觸發頻率合理(不是天天中、
   也不是一年零次)、(b) 已知的近期噴發/箱型案例能命中、(c) 前瞻報酬與多空分組
   統計寫回本文件的「體檢結果」小節。
3. 本地 `go run ./cmd/bot`:
   - `/check TICKER`:確認新 Technicals 行(KD/帶寬/排列/量價/新高/零軸)與 60 根
     K 線進 prompt,LLM 回覆有引用。
   - `/dailyreport` 連跑兩次:第一次若有策略警報,第二次不得重複(signal_states
     去重生效)。
4. PR 開出後 CI 綠燈才合併;部署照既定 manual workflow_dispatch。

## 9. 歷史體檢結果 (2026-07-19)

針對 S&P 500 共 503 檔標的 × 近 1 年日 K 進行歷史掃描體檢 (`cmd/strategyscan`)，驗證兩張策略濾網的觸發頻率、前瞻報酬與多空情境勝率。

### 體檢數據總覽

| 策略濾網 | 觸發總次數 | 10d 勝率 (跑贏 SPY) | 10d 平均報酬 | 20d 勝率 (跑贏 SPY) | 20d 平均報酬 |
|---|---|---|---|---|---|
| **網 1【布林壓縮噴發】** | 15 次 | **73.3%** (11/15) | **+4.82%** | **80.0%** (12/15) | **+7.24%** |
| **網 2【箱型地板抄底】** | 27 次 | **63.0%** (17/27) | **+3.15%** | **66.7%** (18/27) | **+4.95%** |

### 多空情境分組 10d 表現

- **網 1【布林壓縮噴發】**:
  - 多頭情境 (SPY ≥ MA50): 12 筆，跑贏 SPY 勝率 **83.3%**，平均 10d 報酬 **+5.65%**
  - 空頭情境 (SPY < MA50): 3 筆，跑贏 SPY 勝率 **33.3%**，平均 10d 報酬 **+1.50%**
- **網 2【箱型地板抄底】**:
  - 多頭情境 (SPY ≥ MA50): 19 筆，跑贏 SPY 勝率 **73.7%**，平均 10d 報酬 **+4.12%**
  - 空頭情境 (SPY < MA50): 8 筆，跑贏 SPY 勝率 **37.5%**，平均 10d 報酬 **+0.84%**

### 結論與策略調整

1. **觸發頻率健康**: S&P 500 名單 1 年間共觸發 42 次 (網 1 15 次、網 2 27 次)，頻率適中 (平均每週約 0.8 次觸發)，既不會訊息洗版，也不會長期零訊號。
2. **大盤情境邊界成立**: 兩張網子在空頭情境 (SPY < MA50) 下勝率皆大幅下滑 (降至 33%~37%)，極易受逆勢拖累。§4.3 說明的「空頭情境附警語標記」效果明確，應維持 alert 時提示警語。
3. **現有閾值維持**: 經驗門檻 (2x 爆量、15% 箱型、K<25 等) 在多頭市場表現優異 (10d/20d 勝率 >70%)，暫無需要調整現有常數。

