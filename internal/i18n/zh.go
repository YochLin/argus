package i18n

// zhMessages is the original Traditional Chinese (zh-TW) message table —
// the project's default language. Every format verb (%s/%d/%v/%.2f/...)
// here must appear in the same order as its en.go counterpart, since call
// sites pass positional args once and reuse them for whichever table T
// picks.
var zhMessages = map[Key]string{
	KeyUnknownCommand:        "未知指令。可用指令：\n/add <ticker>\n/remove <ticker>\n/list\n/status [ticker]\n/recommend\n/check <ticker>\n/track [天數]\n/dailyreport\n/fundamentals <ticker>\n/reset\n\n直接傳一般訊息（不加 / 開頭）則會跟我自由對話。",
	KeyAddUsage:              "用法：/add <ticker>，例如 /add AAPL",
	KeyAddFailed:             "新增失敗：%v",
	KeyAddSuccess:            "已將 *%s* 加入自選股。",
	KeyRemoveUsage:           "用法：/remove <ticker>，例如 /remove AAPL",
	KeyRemoveFailed:          "移除失敗：%v",
	KeyRemoveSuccess:         "已將 *%s* 從自選股移除。",
	KeyQueryFailed:           "查詢失敗：%v",
	KeyWatchlistEmptyHint:    "自選股清單是空的，使用 /add <ticker> 新增。",
	KeyWatchlistTitle:        "📋 *自選股清單*\n",
	KeyWatchlistEmpty:        "自選股清單是空的。",
	KeyMarketStatusTitle:     "📊 *市場即時狀態*\n\n",
	KeyQuoteUnavailable:      "*%s*：無法取得報價\n",
	KeyQuoteFailed:           "無法取得 %s 報價：%v",
	KeyAnalyzing:             "分析中，請稍候...",
	KeyWatchlistQueryFailed:  "查詢自選股失敗：%v",
	KeyLLMFailed:             "LLM 分析失敗：%v",
	KeyNoRecommendations:     "LLM 未回傳推薦標的，請稍後再試。",
	KeyRecommendationsTitle:  "🤖 *今日推薦標的*\n\n",
	KeyCheckUsage:            "用法：/check <ticker>，例如 /check TSLA",
	KeyAnalyzingTicker:       "分析中，請稍候... (%s)",
	KeyCheckResultTitle:      "📈 *%s 即時分析*\n\n%s",
	KeyFundamentalsUsage:     "用法：/fundamentals <ticker>，例如 /fundamentals AAPL",
	KeyFundamentalsNoKey:     "尚未設定 FINNHUB_API_KEY，無法查詢基本面資料。",
	KeyFundamentalsFailed:    "無法取得 %s 基本面資料：%v",
	KeyFundamentalsTitle:     "📊 *%s 基本面*\n\n",
	KeyThinking:              "思考中，請稍候...",
	KeyChatFailed:            "對話失敗：%v",
	KeyResetDone:             "已重置對話記憶。",
	KeySignalAlertTitle:      "🚨 *市場訊號提醒*\n\n",
	KeyDailyReportStart:      "📅 *每日報告開始生成...*",
	KeyNoRecommendationsToday: "今日無特別推薦標的。",
	KeyTrackUsage:            "用法：/track [天數]，例如 /track 14（預設 7 天，最多 90 天）",
	KeyTrackTitle:            "📊 *推薦成效回顧（過去 %d 天）*\n\n",
	KeyTrackEmpty:            "過去 %d 天內沒有推薦記錄。",
	KeyTrackLine:             "• %s *%s* %s  $%.2f → $%.2f (%+.2f%%) %s\n",
	KeyTrackLineNoPrice:      "• %s *%s* %s — 無推薦時價格可比對\n",
	KeyTrackSummary:          "\n命中率：%d/%d (%.0f%%)\n（BUY 上漲算命中、SELL 下跌算命中；HOLD 與無價格資料者不計入）",

	KeyQuoteLine: "*%s*  $%.2f  %s%.2f%%\n開: $%.2f　高: $%.2f　低: $%.2f",

	KeyValuationHeader:     "*估值*\n",
	KeyPE:                  "• 本益比 (P/E): %.1f\n",
	KeyPB:                  "• 股價淨值比 (P/B): %.1f\n",
	KeyPS:                  "• 股價營收比 (P/S): %.1f\n",
	KeyMarketCap:           "• 市值: $%sM\n",
	KeyBeta:                "• Beta: %.2f\n",
	Key52Week:              "• 52週高/低: $%.2f / $%.2f\n\n",
	KeyProfitabilityHeader: "*獲利能力*\n",
	KeyROE:                 "• ROE: %.1f%%\n",
	KeyROA:                 "• ROA: %.1f%%\n",
	KeyGrossMargin:         "• 毛利率: %.1f%%\n",
	KeyOperatingMargin:     "• 營業利益率: %.1f%%\n",
	KeyNetMargin:           "• 淨利率: %.1f%%\n\n",
	KeyFinStructureHeader:  "*財務結構*\n",
	KeyDebtToEquity:        "• 負債權益比: %.2f\n",
	KeyCurrentRatio:        "• 流動比率: %.2f\n",
	KeyQuickRatio:          "• 速動比率: %.2f\n\n",
	KeyGrowthHeader:        "*成長性 / 股東回饋*\n",
	KeyRevenueGrowth:       "• 營收年增率: %.1f%%\n",
	KeyEPSGrowth:           "• EPS年增率: %.1f%%\n",
	KeyEPS:                 "• EPS (TTM): %.2f\n",
	KeyBookValue:           "• 每股淨值: %.2f\n",
	KeyDividendYield:       "• 股息殖利率: %.2f%%",

	KeyStatementTitle:        "*最新財報 (%s，會計年度 %d，截至 %s)*\n\n",
	KeyIncomeStatementHeader: "損益表\n",
	KeyRevenue:               "• 營收: $%sM\n",
	KeyGrossProfit:           "• 毛利: $%sM\n",
	KeyOperatingIncome:       "• 營業利益: $%sM\n",
	KeyNetIncome:             "• 淨利: $%sM\n",
	KeyDilutedEPS:            "• 稀釋EPS: $%.2f\n\n",
	KeyBalanceSheetHeader:    "資產負債表\n",
	KeyTotalAssets:           "• 總資產: $%sM\n",
	KeyTotalLiabilities:      "• 總負債: $%sM\n",
	KeyTotalEquity:           "• 股東權益: $%sM\n\n",
	KeyCashFlowHeader:        "現金流量表\n",
	KeyOperatingCashFlow:     "• 營運現金流: $%sM\n",
	KeyCapEx:                 "• 資本支出: $%sM\n",
	KeyFreeCashFlow:          "• 自由現金流: $%sM",

	KeyPriceUp:       "上漲",
	KeyPriceDown:     "下跌",
	KeyPriceSignal:   "%s %s %.2f%% (現價 $%.2f)",
	KeyRSIOverbought: "%s RSI 超買 (%.1f)，可能回調",
	KeyRSIOversold:   "%s RSI 超賣 (%.1f)，可能反彈",
	KeyMACDBullish:   "%s MACD 呈多頭動能 (MACD %.2f > 訊號線 %.2f)",
	KeyMACDBearish:   "%s MACD 呈空頭動能 (MACD %.2f < 訊號線 %.2f)",
	KeyMACDGoldenCross: "%s MACD 黃金交叉 (MACD %.2f 上穿訊號線 %.2f)，趨勢轉多",
	KeyMACDDeathCross:  "%s MACD 死亡交叉 (MACD %.2f 下穿訊號線 %.2f)，趨勢轉空",

	KeySystemPromptAnalyst: "你是一位專業的美股分析師，只負責針對使用者提供的市場數據給出文字分析。你沒有任何工具可用，也不需要使用工具；請直接依照使用者訊息中的格式要求回覆，不要輸出與分析無關的內容。",
	KeySystemPromptChat:    "你是使用者的個人助理，請用繁體中文自然對話，記住這段對話中先前提到的內容。你沒有任何工具可用，也不需要使用工具。",

	KeyRecPromptIntro:     "你是一位美股分析師，請根據以下市場數據給出今日操作建議。\n\n",
	KeyRecWatchlistHeader: "## 我的自選股\n\n",
	KeyRecNoWatchlist:     "（無自選股）\n\n",
	KeyRecMoversHeader:    "## 大盤熱門標的\n\n",
	KeyRecNoCandidates:    "（無候選標的）\n\n",
	KeyRecTaskBlock: `
## 任務

自選股的每一檔都必須給出明確操作建議，動作只能是 BUY、SELL、HOLD 三者之一；
大盤熱門標的則只挑出你看好、值得買進的（最多 3 檔，動作為 BUY），沒有就不列。

輸出格式請嚴格遵守以下結構，每檔一個區塊，不要有額外文字：

[TICKER: AAPL]
%s BUY
%s （200字以內的中文說明，包含技術面、基本面或新聞面理由）

[TICKER: MSFT]
%s HOLD
%s ...

自選股排在前面；同組內依照你最看好的順序排列，信心最高的排第一。
`,
	KeyReasonMarker: "原因:",
	KeyActionMarker: "動作:",

	KeyCheckPromptIntro: "你是一位美股分析師，請針對以下標的給出即時市場分析。\n\n",
	KeyCheckPromptTask: `
請用繁體中文分析：
1. 今日走勢解讀（技術面）
2. 近期新聞影響（情緒面）
3. 基本面體質（若有提供基本面／財報數據，評論估值、獲利能力、財務結構是否健康）
4. 短線操作建議（買進 / 觀望 / 減碼）並說明理由

回覆請簡潔，總字數控制在 400 字以內。
`,

	KeyStockHeader:             "### %s\n",
	KeyPriceLine:               "- 現價: $%.2f　漲跌幅: %.2f%%\n",
	KeyOHLLine:                 "- 開盤: $%.2f　最高: $%.2f　最低: $%.2f\n",
	KeyVolumeLine:              "- 成交量: %d　前收: $%.2f\n",
	KeyQuoteTimeLine:           "- 報價時間: %s\n",
	KeyNewsHeader:              "- 近期新聞:\n",
	KeyNewsItem:                "  %d. [%s] %s\n",
	KeyFundamentalsSummaryLine: "- 基本面：本益比 %.1f｜股價淨值比 %.1f｜ROE %.1f%%｜毛利率 %.1f%%｜營業利益率 %.1f%%｜淨利率 %.1f%%｜負債權益比 %.2f｜營收年增率 %.1f%%｜EPS年增率 %.1f%%｜股息殖利率 %.2f%%｜Beta %.2f\n",
	KeyStatementSummaryLine:    "- 最新財報（%s，會計年度 %d，截至 %s）：營收 $%.0fM｜毛利 $%.0fM｜營業利益 $%.0fM｜淨利 $%.0fM｜總資產 $%.0fM｜總負債 $%.0fM｜股東權益 $%.0fM｜營運現金流 $%.0fM｜自由現金流 $%.0fM\n",
}
