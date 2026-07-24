package i18n

// zhMessages is the original Traditional Chinese (zh-TW) message table —
// the project's default language. Every format verb (%s/%d/%v/%.2f/...)
// here must appear in the same order as its en.go counterpart, since call
// sites pass positional args once and reuse them for whichever table T
// picks.
var zhMessages = map[Key]string{
	KeyUnknownCommand:            "未知指令。可用指令：\n/add <ticker>\n/remove <ticker>\n/list\n/status [ticker]\n/recommend\n/check <ticker>\n/track [天數]\n/buy <ticker> <股數> <價格> [手續費]\n/sell <ticker> <股數> <價格> [手續費]\n/stop <ticker> [價格]\n/portfolio\n/insight\n/cash [金額]\n/thesis <ticker> [論點文字]\n/review <ticker>\n/dailyreport\n/fundamentals <ticker>\n/universe [add|remove] <ticker>\n/reset\n\n直接傳一般訊息（不加 / 開頭）則會跟我自由對話。",
	KeyAddUsage:                  "用法：/add <ticker>，例如 /add AAPL",
	KeyAddFailed:                 "新增失敗：%v",
	KeyAddSuccess:                "已將 *%s* 加入自選股。",
	KeyRemoveUsage:               "用法：/remove <ticker>，例如 /remove AAPL",
	KeyRemoveFailed:              "移除失敗：%v",
	KeyRemoveSuccess:             "已將 *%s* 從自選股移除。",
	KeyQueryFailed:               "查詢失敗：%v",
	KeyWatchlistEmptyHint:        "自選股清單是空的，使用 /add <ticker> 新增。",
	KeyWatchlistTitle:            "📋 *自選股清單*\n",
	KeyWatchlistEmpty:            "自選股清單是空的。",
	KeyMarketStatusTitle:         "📊 *市場即時狀態*\n\n",
	KeyQuoteUnavailable:          "*%s*：無法取得報價\n",
	KeyQuoteFailed:               "無法取得 %s 報價：%v",
	KeyAnalyzing:                 "分析中，請稍候...",
	KeyWatchlistQueryFailed:      "查詢自選股失敗：%v",
	KeyLLMFailed:                 "LLM 分析失敗：%v",
	KeyNoRecommendations:         "LLM 未回傳推薦標的，請稍後再試。",
	KeyRecommendationsTitle:      "🤖 *今日推薦標的*\n\n",
	KeyRecommendUsage:            "用法：/recommend [tw|us]（不帶參數＝依序跑美股與台股兩份報告）",
	KeyRecWatchlistSectionTitle:  "📋 *自選股與持倉*\n",
	KeyRecCandidatesSectionTitle: "🆕 *新標的推薦*\n",
	KeySizingLine:                "💰 風險倉位建議：可承受虧損 $%.2f，停損參考 $%.2f（現價−2×ATR），建議股數 %d\n",
	KeyMarketNewsSummaryTitle:    "📰 *市場新聞摘要*\n\n",
	KeyCheckUsage:                "用法：/check <ticker>，例如 /check TSLA",
	KeyAnalyzingTicker:           "分析中，請稍候... (%s)",
	KeyCheckResultTitle:          "📈 *%s 即時分析*\n\n%s",
	KeyFundamentalsUsage:         "用法：/fundamentals <ticker>，例如 /fundamentals AAPL",
	KeyFundamentalsNoKey:         "尚未設定 FINNHUB_API_KEY，無法查詢基本面資料。",
	KeyFundamentalsFailed:        "無法取得 %s 基本面資料：%v",
	KeyFundamentalsTitle:         "📊 *%s 基本面*\n\n",
	KeyThinking:                  "思考中，請稍候...",
	KeyChatFailed:                "對話失敗：%v",
	KeyResetDone:                 "已重置對話記憶。",
	KeySignalAlertTitle:          "🚨 *市場訊號提醒*\n\n",
	KeyDailyReportStart:          "📅 *每日報告開始生成...*",
	KeyNoRecommendationsToday:    "今日無特別推薦標的。",
	KeyTrackUsage:                "用法：/track [天數]，例如 /track 14（預設 7 天，最多 90 天）",
	KeyTrackTitle:                "📊 *推薦成效回顧（過去 %d 天）*\n\n",
	KeyTrackEmpty:                "過去 %d 天內沒有推薦記錄。",
	KeyTrackLine:                 "• %s *%s* %s  $%.2f → $%.2f (%+.2f%%) %s\n",
	KeyTrackLineVsSPY:            "• %s *%s* %s  $%.2f → $%.2f (%+.2f%% vs %s %+.2f%%) %s\n",
	KeyTrackLineNoPrice:          "• %s *%s* %s — 無推薦時價格可比對\n",
	KeyTrackSummary:              "\n命中率：%d/%d (%.0f%%)\n（有同期基準指數（SPY/0050）資料時，BUY 需跑贏基準、SELL 需跑輸基準才算命中；缺基準資料則以絕對漲跌計算；HOLD 與無價格資料者不計入）",
	KeyTrackAvgReturnLine:        "平均報酬：BUY %+.2f%%（%d 筆）｜SELL %+.2f%%（%d 筆）\n",
	KeyTrackBySourceHeader:       "\n按來源分組：\n",
	KeyTrackBySourceLine:         "- %s：%d/%d (%.0f%%)\n",
	KeyTrackByMarketHeader:       "\n按市場分組：\n",
	KeyTrackByMarketLine:         "- %s：%d/%d (%.0f%%)\n",

	KeyBuyUsage:               "用法：/buy <ticker> <股數> <價格> [手續費] [日期]，例如 /buy AAPL 10 205.5 或 /buy AAPL 10 205.5 1.5 2026-01-15（回填歷史交易日期）",
	KeyBuyFailed:              "記錄買入失敗：%v",
	KeyBuySuccess:             "✅ 已記錄買入 *%s* %g 股 @ $%.2f（手續費 $%.2f）\n目前持倉：%g 股，平均成本 $%.2f",
	KeySellUsage:              "用法：/sell <ticker> <股數> <價格> [手續費] [日期]，例如 /sell AAPL 5 210 或 /sell AAPL 5 210 1 2026-01-15（回填歷史交易日期）",
	KeySellFailed:             "記錄賣出失敗：%v",
	KeySellSuccess:            "✅ 已記錄賣出 *%s* %g 股 @ $%.2f（手續費 $%.2f）\n已實現損益：%+.2f\n剩餘持倉：%g 股",
	KeySellNoPosition:         "沒有 *%s* 的持倉可供賣出。",
	KeySellInsufficientShares: "*%s* 持股不足，無法賣出這麼多股。",

	KeyStopUsage:            "用法：/stop <ticker> [價格]（不帶價格則查詢目前設定與候選停損價），例如 /stop AAPL 190",
	KeyStopNoPosition:       "沒有 *%s* 的持倉，無法設定停損。",
	KeyStopInvalidPrice:     "停損價 $%.2f 必須低於最新收盤 $%.2f（做多停損只能設在價格下方）。",
	KeyStopSet:              "✅ 已設定 *%s* 停損 $%.2f（距現價 %.1f%%，單股風險 $%.2f）",
	KeyStopShow:             "*%s* 目前停損：$%.2f\n",
	KeyStopNotSet:           "*%s* 尚未設定個股停損，目前退回全域停損門檻 %.1f%%。\n",
	KeyStopCandidatesHeader: "候選停損參考價：\n",
	KeyStopCandidateLine:    "  • %s：$%.2f\n",
	KeyStopLow10Label:       "近10日低",
	KeyStopLow20Label:       "近20日低",
	KeyStopATRLabel:         "現價−2×ATR",
	KeyBuyStopSuggestion:    "\n💡 停損參考：\n",
	KeyBuyStopAddOnNote:     "*%s* 目前停損 $%.2f，加碼後可用 /stop 重新設定。\n",

	KeyPortfolioTitle:      "💼 *資產總覽*\n\n",
	KeyPortfolioEmpty:      "目前沒有持倉。使用 /buy <ticker> <股數> <價格> 記錄買入。",
	KeyPortfolioLine:       "*%s*  %g 股 @ 均價 $%.2f\n現價 $%.2f　市值 $%.2f　未實現損益 %+.2f (%+.2f%%)\n\n",
	KeyPortfolioSummary:    "—\n總市值：$%.2f\n累計已實現損益：%+.2f",
	KeyPortfolioSectionUS:  "*美股 (USD)*\n",
	KeyPortfolioSectionTW:  "*台股 (TWD)*\n",
	KeyPortfolioSummaryTWD: "—\n總市值：NT$%.2f\n累計已實現損益：%+.2f",
	KeyPortfolioLotSuffix:  "（= %d 張）",
	KeyJobPanic:            "⚠️ *%s* 執行時發生未預期錯誤，已中止：%v",

	KeyInsightResultTitle: "💡 *投資組合見解*\n\n%s",

	KeyWeeklyReviewStart:             "📅 正在產生本週組合回顧...",
	KeyWeeklyReviewResultTitle:       "📅 *每週組合回顧*\n\n%s",
	KeyWeeklyNetWorthLine:            "📈 總市值：$%.2f（較一週前 %+.1f%%）\n",
	KeyWeeklyNetWorthLineWithCash:    "📈 總市值：$%.2f（較一週前 %+.1f%%）｜含現金總資產：$%.2f\n",
	KeyWeeklyNetWorthLineTWD:         "📈 台股總市值：NT$%.2f（較一週前 %+.1f%%）\n",
	KeyWeeklyNetWorthLineWithCashTWD: "📈 台股總市值：NT$%.2f（較一週前 %+.1f%%）｜含現金總資產：NT$%.2f\n",
	KeyWeeklyEarningsPreviewTitle:    "\n📅 *下週財報前瞻*\n",
	KeyWeeklyEarningsPreviewLine:     "- %s：%s（%d 天後）\n",

	KeyMonthlyReportTitle:           "📅 *%s 淨值月報*\n\n",
	KeyMonthlyReportSparklineLine:   "走勢：%s\n",
	KeyMonthlyReportChangeLine:      "月底淨值：$%.2f（較上月末 %+.2f%%）\n",
	KeyMonthlyReportDrawdownLine:    "月內最大回落：%.2f%%\n",
	KeyMonthlyReportRealizedLine:    "當月已實現損益：$%.2f\n",
	KeyMonthlyReportSPYLine:         "同期 SPY：%+.2f%%\n",
	KeyMonthlyReportTWBenchmarkLine: "同期 0050：%+.2f%%\n",
	KeyMonthlyReportTxCountLine:     "當月交易筆數：%d\n",
	KeyMonthlyReportCashLine:        "含現金總資產：$%.2f（現金 $%.2f）\n",

	KeyCashUsage:         "用法：/cash <金額>＝USD；/cash twd <金額> 或 /cash usd <金額> 指定幣別；查詢請不帶參數",
	KeyCashCurrent:       "目前登記現金：$%.2f",
	KeyCashCurrentTWD:    "目前登記現金 (TWD)：NT$%.2f",
	KeyCashNotSet:        "尚未登記現金水位，使用 /cash <金額> 設定。",
	KeyCashSetSuccess:    "已將現金水位設為 $%.2f。",
	KeyCashSetSuccessTWD: "已將現金水位 (TWD) 設為 NT$%.2f。",
	KeyCashSetFailed:     "設定現金水位失敗：%v",

	KeyStopLossAlertTitle:       "🛑 *停損提醒*\n\n",
	KeyStopLossAlertLine:        "• *%s* 成本 $%.2f → 現價 $%.2f，未實現損失 %.1f%%，已達停損門檻\n",
	KeyStopPriceHit:             "• *%s* 停損價 $%.2f，現價 $%.2f，已跌破停損價\n",
	KeyTrailingStopAlertTitle:   "📉 *移動停損提醒*\n\n",
	KeyTrailingStopAlertLine:    "• *%s* 持有期間最高收盤 $%.2f → 現價 $%.2f，回落 %.1f%%，已達移動停損門檻\n",
	KeyTrailingStopAlertLineATR: "• *%s* 持有期間最高收盤 $%.2f → 現價 $%.2f，回落 %.1f%%，已達移動停損門檻（門檻 %.1f%%，%.1f×ATR）\n",

	KeyTargetReached: "🎯 *%s* 已達 %.1fR 獲利目標！目標價 $%.2f，現價 $%.2f——考慮獲利了結一半，剩餘部位改用 5MA 防守。\n",
	KeyMA5Break:      "📉 *%s* 跌破 5 日均線 $%.2f（現價 $%.2f），強勢段防線失守，考慮了結剩餘部位。\n",

	KeyChatContextHeader:       "[背景資料，非即時報價，僅供參考]\n",
	KeyChatContextTickerNoData: "- %s：尚無收盤資料\n",
	KeyChatContextWatchLine:    "- %s：%s 收盤 $%.2f (%+.2f%%)\n",
	KeyChatContextPositionLine: "- %s：%s 收盤 $%.2f (%+.2f%%)｜持倉 %g 股，均價 $%.2f，未實現損益 %+.2f%%\n",
	KeyChatContextFooter:       "[背景資料結束，請針對以下訊息回覆]\n\n",

	KeyFetchingArticle:    "正在讀取文章內容，請稍候...",
	KeyArticleFetchFailed: "無法擷取這個網址的內容：%v\n（可能是付費牆或該網站阻擋自動存取，也可以直接把文章內文貼給我）",
	KeyArticleTaskBlock:   "使用者在聊天中分享了一篇文章連結。請先用 3-5 句摘要文章重點，再說明這篇文章跟使用者的自選股/持倉有沒有關聯、有什麼潛在影響——如果沒有實質關聯，直接說沒有即可，不要牽強附會。\n\n標題：%s\n網址：%s\n\n內文：\n%s\n\n使用者的訊息：%s\n",

	KeyEarningsAlertTitle: "📅 *財報日提醒*\n\n",
	KeyEarningsAlertLine:  "• *%s* 將於 %s 公布財報（%d 天內）\n",

	KeyUniverseUsage:         "用法：/universe [add|remove] TICKER\n不帶參數會顯示掃描池摘要。",
	KeyUniverseSummary:       "*掃描池*\n總計：%d 檔\n",
	KeyUniverseSourceLine:    "- %s：%d\n",
	KeyUniverseAddUsage:      "用法：/universe add TICKER",
	KeyUniverseAddSuccess:    "已將 %s 加入掃描池。",
	KeyUniverseAddFailed:     "加入 %s 失敗：%v",
	KeyUniverseRemoveUsage:   "用法：/universe remove TICKER",
	KeyUniverseRemoveSuccess: "已將 %s 從掃描池移除。",
	KeyUniverseRemoveFailed:  "移除 %s 失敗：%v",
	KeyUniverseDelistedTitle: "📉 *S&P 500 成分股異動*\n\n以下標的已不在最新 S&P 500 清單中，是否要從掃描池移除由你決定：\n\n",
	KeyUniverseDelistedLine:  "• %s（可用 /universe remove %s 移除）\n",

	KeyThesisUsage:      "用法：/thesis <ticker> [論點文字]\n只帶 ticker 會查詢目前記錄的論點；帶文字會覆寫設定。",
	KeyThesisSetFailed:  "記錄 %s 的論點失敗：%v",
	KeyThesisSetSuccess: "已記錄 *%s* 的持有論點：%s",
	KeyThesisNotSet:     "*%s* 目前沒有記錄論點，用 /thesis %s <文字> 補上。",
	KeyThesisCurrent:    "*%s* 目前的持有論點：%s",
	KeyBuyThesisNudge:   "\n💭 *%s* 還沒有記錄持有論點，要不要用 /thesis %s <文字> 補一句？",

	KeyReviewUsage:            "用法：/review <ticker>，查詢該檔最近一次已了結交易的復盤",
	KeyReviewNoClosedTrade:    "*%s* 目前沒有已了結的交易紀錄可供復盤。",
	KeyTradeReviewResultTitle: "📓 *交易復盤：%s*\n\n%s",

	KeyConfirmButton:                "✅ 確認",
	KeyRejectButton:                 "❌ 取消",
	KeyPendingBuyConfirm:            "📝 *交易提案待確認*\n買入 *%s* %g 股 @ $%.2f（手續費 $%.2f，日期 %s）\n請按下方按鈕確認或取消：",
	KeyPendingSellConfirm:           "📝 *交易提案待確認*\n賣出 *%s* %g 股 @ $%.2f（手續費 $%.2f，日期 %s）\n請按下方按鈕確認或取消：",
	KeyPendingActionRejected:        "已取消，未記錄任何交易。",
	KeyPendingActionAlreadyResolved: "這筆提案已經處理過了，不會重複執行。",
	KeyPendingActionExecFailed:      "提案內容解析失敗，未執行任何動作，請改用 /buy 或 /sell 手動記錄。",

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

	KeyPriceUp:         "上漲",
	KeyPriceDown:       "下跌",
	KeyPriceSignal:     "%s %s %.2f%% (現價 $%.2f)",
	KeyRSIOverbought:   "%s RSI 超買 (%.1f)，可能回調",
	KeyRSIOversold:     "%s RSI 超賣 (%.1f)，可能反彈",
	KeyMACDBullish:     "%s MACD 呈多頭動能 (MACD %.2f > 訊號線 %.2f)",
	KeyMACDBearish:     "%s MACD 呈空頭動能 (MACD %.2f < 訊號線 %.2f)",
	KeyMACDGoldenCross: "%s MACD 黃金交叉 (MACD %.2f 上穿訊號線 %.2f)，趨勢轉多",
	KeyMACDDeathCross:  "%s MACD 死亡交叉 (MACD %.2f 下穿訊號線 %.2f)，趨勢轉空",

	KeyStrategySqueezeBreakout:   "🚀 %s 觸發【布林壓縮噴發】(%s): 帶寬 20 日新低後爆量突破上軌，MACD 柱翻紅",
	KeyStrategyBoxBottom:         "🎯 %s 觸發【箱型地板抄底】(%s): 30 日箱型底 + KD 低檔金叉/勾頭 + RSI 止跌反彈",
	KeyDaysAgoToday:              "今日",
	KeyDaysAgoN:                  "%d 天前",
	KeyTechnicalsKDLine:          "- KD(9,3,3)：K=%.1f，D=%.1f\n",
	KeyTechnicalsBandwidthLine:   "- 布林帶寬：%.1f%%\n",
	KeyTechnicalsMAAlignLine:     "- 均線排列(5/20/60)：%s\n",
	KeyTechnicalsVolumePriceLine: "- 量價對照：%s\n",
	KeyTechnicalsNewHighLine:     "- 新高紀錄：%s\n",
	KeyTechnicalsMACDZeroLine:    "- MACD 零軸位置：%s (MACD=%.2f)\n",
	KeyTechnicalsRSLine:          "- 相對強度 RS63：%s (超越 SPY %+.1f%%)\n",
	KeyStrategyHitLine:           "- ⚠️ 策略觸發：%s (%s)\n",
	KeyStrategyBearRegimeWarning: "⚠️ 警告：當前大盤趨勢為空頭/走弱，逆勢策略警報勝率較低，切勿盲目抄底/追高。",
	KeyTechGuidanceBlock:         "\n【技術面與量價判讀指引】\n1. 支撐壓力：連續長下影線/雙重底(W底)為強支撐；爆量長紅 K 低點為主力成本支撐，跌破即棄守；突破後舊壓力轉支撐。\n2. 量價配合：量增價漲=趨勢啟動；量縮價跌=健康洗盤；量縮價漲=虛胖誘多；量增價跌=恐慌倒貨勿接刀。\n3. MACD：零軸上金叉力道較強，零軸下金叉多為弱反彈；價格創新高但柱狀體峰值遞減為頂背離警訊。\n4. KD/RSI：KD 出超賣區(翻越20/25)再動作，避免低檔鈍化接刀；RSI 50 為多空分界線，站穩 50 之拉回為強勢點。\n5. 多頭排列 (MA5 > MA20 > MA60 全朝上) 時拉回 MA20 為動態支撐點。\n6. 給出 BUY 建議時，原因中務必點名一個具體的建議停損位（結構性支撐位，如近10/20日低點，或現價減2倍ATR），並說明依據——這是給人工執行的參考，不是要你去算股數。\n",
	KeyVolUpPriceUp:              "量增價漲 (多頭發力)",
	KeyVolDownPriceDown:          "量縮價跌 (健康洗盤)",
	KeyVolDownPriceUp:            "量縮價漲 (虛胖誘多)",
	KeyVolUpPriceDown:            "量增價跌 (恐慌倒貨)",
	KeyNewHigh20And52:            "創 20 日與 52 週新高",
	KeyNewHigh20:                 "創 20 日新高",
	KeyNewHigh52:                 "創 52 週新高",
	KeyMACDAboveZero:             "零軸之上 (強勢區)",
	KeyMACDBelowZero:             "零軸之下 (弱勢區)",
	KeyRSStronger:                "強於大盤",
	KeyRSWeaker:                  "弱於大盤",
	KeyStrategySqueezeName:       "布林壓縮噴發",
	KeyStrategyBoxName:           "箱型地板抄底",

	KeySystemPromptAnalyst: "你是一位專業的美股分析師，只負責針對使用者提供的市場數據給出文字分析。你沒有任何工具可用，也不需要使用工具；請直接依照使用者訊息中的格式要求回覆，不要輸出與分析無關的內容。",
	KeySystemPromptChat:    "你是使用者的個人助理，請用繁體中文自然對話，記住這段對話中先前提到的內容。你沒有任何工具可用，也不需要使用工具。",

	KeyRecPromptIntro:      "你是一位美股分析師，請根據以下市場數據給出今日操作建議。\n\n",
	KeyRecTWMarketNote:     "本批為台灣上市櫃股票，金額為新台幣（TWD），且個股單日漲跌幅上限為 ±10%。\n\n",
	KeyMarketRegimeHeader:  "## 大盤情境\n\n",
	KeyMarketRegimeSPYLine: "• SPY 現價 $%.2f，MA200 $%.2f，MA50 $%.2f — %s\n",
	KeyMarketRegimeVIXLine: "• VIX %.2f — %s\n",
	KeyRiskOn:              "多頭（現價站上 MA200）",
	KeyRiskOff:             "空頭（現價跌破 MA200）",
	KeyVIXCalm:             "平靜",
	KeyVIXNormal:           "正常",
	KeyVIXPanic:            "高恐慌",
	KeyRecMarketNewsHeader: "## 大盤新聞\n\n",
	KeyRecWatchlistHeader:  "## 我的自選股\n\n",
	KeyRecNoWatchlist:      "（無自選股）\n\n",
	KeyRecMoversHeader:     "## 大盤熱門標的\n\n",
	KeyRecNoCandidates:     "（無候選標的）\n\n",
	KeyRecMarketSummaryTask: `
在逐檔分析之前，請先輸出一個區塊，第一行必須是：
%s
接著用 3–5 條重點統整以上市場新聞，並指出可能受惠或受害的類股/標的（總長 150 字以內）。
之後再接續逐檔分析區塊。
`,
	KeyRecTaskBlock: `
## 任務

自選股的每一檔都必須給出明確操作建議，動作只能是 BUY、SELL、HOLD 三者之一；
大盤熱門標的則只挑出你看好、值得買進的（最多 3 檔，動作為 BUY），沒有就不列。

評估每檔新聞時，請留意每則是利多還是利空，並融入你的原因說明。

給出 BUY 建議時，請一併評估目前價位是否為合理的進場時機——現價落在 52 週區間的位置、是否站上/跌破均線、
成交量是否明顯放大於近期均量（顯示是否有真實買盤認同這個價位，而非量能萎縮的假突破）、分析師評等分佈
近期是否轉趨樂觀，藉此判斷市場對這檔股票的熱度與買盤意願，而非只靠估值倍數或分析師目標價（目標價通常
落後於股價、參考價值有限）。若動能明顯不佳（例如剛創高後拉回、量能萎縮、評等轉趨保守），即使基本面或
新聞面正向，也應在原因中一併說明進場時機的疑慮，而不是只談基本面轉強。

若某檔下方列出了「上次建議」，且這次的動作與上次不同，請在原因中明確說明是什麼改變了
（新資訊、技術訊號翻轉、或原本的論點已經站不住腳），不要只是換個說法重複同一個結論。

若上方出現「大盤情境」區塊，請將其納入新倉 BUY 建議的判斷：空頭或 VIX 偏高時，對新倉
應採用更保守的標準，而不是不論大盤情境一律套用同一套門檻。

給出 BUY 建議前，請先在心中列出最強的看空理由（為什麼這檔現在不該買），並在原因中簡短
說明這個理由為什麼不成立或影響有限；給出 SELL 建議則反過來，先列出最強的看多理由再
說明為何不足以支撐續抱。這是為了避免你只挑對結論有利的資料說故事、忽略對立證據。

輸出格式請嚴格遵守以下結構，每檔一個區塊，不要有額外文字：

[TICKER: AAPL]
%s BUY
%s （200字以內的中文說明，包含技術面、基本面或新聞面理由）

[TICKER: MSFT]
%s HOLD
%s ...

自選股排在前面；同組內依照你最看好的順序排列，信心最高的排第一。
`,
	KeyReasonMarker:        "原因:",
	KeyActionMarker:        "動作:",
	KeyMarketSummaryMarker: "[MARKET SUMMARY]",

	KeyExplorePromptIntro: "你是一位美股分析師，請根據以上市場新聞，提名目前不在下列清單上、但可能因為這些新聞而值得留意的美股標的。\n\n",
	KeyExploreExcludeLine: "以下標的已經在清單上，請勿重複提名：%s\n\n",
	KeyExploreTaskBlock: `
## 任務

請提名最多 %d 檔上面新聞提到、但不在排除清單上的美股標的（真實存在的美股 ticker，不要虛構）。
如果新聞中沒有值得提名的標的，就不要輸出任何區塊。

輸出格式請嚴格遵守以下結構，每檔一個區塊，不要有額外文字：

%s TICKER]
%s 一句話說明為什麼這檔標的值得留意（50字以內）
`,
	KeyExploreMarker:      "[EXPLORE:",
	KeyExploreReasonLabel: "LLM 提名: %s",

	KeyCheckPromptIntro: "你是一位美股分析師，請針對以下標的給出即時市場分析。\n\n",
	KeyCheckPromptTask: `
請用繁體中文分析：
1. 今日走勢解讀（技術面）
2. 近期新聞影響（情緒面）
3. 基本面體質（若有提供基本面／財報數據，評論估值、獲利能力、財務結構是否健康）
4. 短線操作建議（買進 / 觀望 / 減碼）並說明理由

回覆請簡潔，總字數控制在 400 字以內。
`,

	KeyInsightPromptIntro:       "你是一位美股分析師，請針對以下整體持倉組合給出組合層級的見解（不是逐檔重複個股分析）。\n\n",
	KeyInsightPositionValueLine: "持倉總市值：$%.2f\n",
	KeyInsightCashLine:          "現金水位：$%.2f\n總資產（持倉＋現金）：$%.2f\n",
	KeyInsightCashLineTWD:       "現金水位 (TWD)：NT$%.2f\n",
	KeyInsightPromptTask: `
請用繁體中文分析整個投資組合（不要逐檔重複已列出的數據，聚焦組合層級的判斷）：
1. 集中度風險：單一持股或單一產業是否佔比過高（產業請根據你自己對這些標的的認識判斷，不用等資料提供）
2. 持有理由檢視：若某檔附有「持有論點」，請直接挑戰它——對照近期新聞與基本面，這個論點是否還站得住腳，
   哪裡已經變了；沒有附論點的標的，則檢視當初可能的持有邏輯是否仍然成立
3. 相對大盤表現：若附有「相對大盤」數據，指出哪些持倉其實在跑輸 SPY，光是上漲不代表這筆投資是對的
4. 加碼／減碼／停損建議：哪些部位該加碼、該減碼、或該設停損，並說明理由
5. 若有提供現金水位，簡短評論目前的持倉／現金配置是否合理

回覆請簡潔，總字數控制在 600 字以內。
`,

	KeyWeeklyReviewPromptIntro: "你是一位美股分析師，請針對以下整體持倉組合給出本週回顧（不是逐檔重複個股分析）。\n\n",
	KeyWeeklyReviewTrackHeader: "\n本週推薦追蹤成效：\n",
	KeyWeeklyReviewPromptTask: `
請用繁體中文分析整個投資組合（不要逐檔重複已列出的數據，聚焦組合層級的判斷）：
1. 集中度風險：單一持股或單一產業是否佔比過高（產業請根據你自己對這些標的的認識判斷，不用等資料提供）
2. 持有理由檢視：若某檔附有「持有論點」，請直接挑戰它——對照近期新聞與基本面，這個論點是否還站得住腳，
   哪裡已經變了；沒有附論點的標的，則檢視當初可能的持有邏輯是否仍然成立
3. 相對大盤表現：若附有「相對大盤」數據，指出哪些持倉其實在跑輸 SPY，光是上漲不代表這筆投資是對的
4. 加碼／減碼／停損建議：哪些部位該加碼、該減碼、或該設停損，並說明理由
5. 若有提供現金水位，簡短評論目前的持倉／現金配置是否合理
6. 若上方附有「本週推薦追蹤成效」數據，簡短點評哪一類建議（可依動作 BUY/SELL 或來源 watchlist/movers/scan
   區分）最準、哪一類最不準；若沒有這項數據則略過此點

回覆請簡潔，總字數控制在 600 字以內。
`,

	KeyTradeReviewPromptIntro: "你是一位美股分析師，請針對以下這筆已了結的 *%s* 交易做覆盤分析。\n\n",
	KeyTradeReviewLegLine:     "• %s %g 股 @ $%.2f（%s）\n",
	KeyTradeReviewPnLLine:     "已實現損益：%+.2f｜持有天數：%d 天\n",
	KeyTradeReviewRangeLine:   "持有期間價格區間：最高 $%.2f／最低 $%.2f\n",
	KeyRMultipleLine:          "- R 倍數：%.1fx\n",
	KeyTradeReviewRecsHeader:  "\n持有期間的推薦紀錄：\n",
	KeyTradeReviewRecLine:     "- %s %s：%s\n",
	KeyTradeReviewPromptTask: `
請用繁體中文做誠實、不粉飾的覆盤，結果賺錢但過程判斷有誤時也要照樣指出：
1. 進出場時點：買進、賣出的價位相對於持有期間的最高／最低點分別落在哪裡？是否因為出場
   太早而少賺、或出場太晚而多虧？
2. 論點對照：若上方附有「持有論點」，實際結果是否兌現了這個論點？即使賺錢，若理由跟
   論點無關，也要指出來；沒有附論點則略過此點。
3. 建議對照：若上方附有持有期間的推薦紀錄，這些建議與使用者實際的買賣動作是否一致
   （例如系統說 SELL 卻抱了兩週、或系統說 HOLD 卻賣掉了）；沒有附推薦紀錄則略過此點。
4. 收斂成一條具體、可執行的教訓，不要空泛的「要更謹慎」之類的話。

回覆最後另起一行，開頭必須是：
%s <這條教訓，2–4 句話以內>

回覆請簡潔，總字數控制在 300 字以內。
`,
	KeyLessonMarker: "教訓:",

	KeyPastLessonsHeader:   "- 過去這檔的交易教訓：\n",
	KeyPastLessonLine:      "  · %s：%s\n",
	KeyRecentLessonsHeader: "## 近期交易教訓（跨標的，僅供參考）\n\n",
	KeyRecentLessonLine:    "- %s（%s）：%s\n",

	KeyStockHeader:             "### %s\n",
	KeyPriceLine:               "- 現價: $%.2f　漲跌幅: %.2f%%\n",
	KeyOHLLine:                 "- 開盤: $%.2f　最高: $%.2f　最低: $%.2f\n",
	KeyVolumeLine:              "- 成交量: %d　前收: $%.2f\n",
	KeyQuoteTimeLine:           "- 報價時間: %s\n",
	KeyNewsHeader:              "- 近期新聞:\n",
	KeyNewsItem:                "  %d. [%s] %s\n",
	KeyFundamentalsSummaryLine: "- 基本面：本益比 %.1f｜股價淨值比 %.1f｜ROE %.1f%%｜毛利率 %.1f%%｜營業利益率 %.1f%%｜淨利率 %.1f%%｜負債權益比 %.2f｜營收年增率 %.1f%%｜EPS年增率 %.1f%%｜股息殖利率 %.2f%%｜Beta %.2f｜EPS(TTM) %.2f｜流動比率 %.2f｜市值 $%.0fM｜距52週高點 %+.1f%%｜距52週低點 %+.1f%%\n",
	KeyMonthRevenueYoYLine:     "- 月營收年增率：%+.1f%%\n",
	KeyAnalystRatingLine:       "- 分析師評等（%s）：強力買進 %d｜買進 %d｜持有 %d｜賣出 %d｜強力賣出 %d\n",
	KeyAnalystRatingTrendLine:  "  較上月：買進方 %+d｜賣出方 %+d\n",
	KeyStatementSummaryLine:    "- 最新財報（%s，會計年度 %d，截至 %s）：營收 $%.0fM｜毛利 $%.0fM｜營業利益 $%.0fM｜淨利 $%.0fM｜總資產 $%.0fM｜總負債 $%.0fM｜股東權益 $%.0fM｜營運現金流 $%.0fM｜自由現金流 $%.0fM\n",
	KeyPositionLine:            "- 持倉: %g 股　平均成本 $%.2f　未實現損益 %+.2f%%\n",
	KeyPrevRecLine:             "- 上次建議: %s @ $%.2f（%d 天前）\n",
	KeyEarningsLine:            "- ⚠️ 財報日: %s（%d 天內），留意波動\n",
	KeyScanHitLine:             "- 🔎 掃描命中: %s\n",
	KeyThesisLine:              "- 📝 持有論點: %s\n",
	KeyVsSPYLine:               "- 相對大盤: 持有期間 %+.1f%% vs SPY 同期 %+.1f%%\n",
	KeyTechnicalsSummaryLine:   "- 技術面：RSI(14) %.1f｜MACD %s\n",
	KeyTechnicalsMALine:        "- %s MA%d ($%.2f)\n",
	KeyVolumeRatioLine:         "- 量能：近20日均量的 %.1f 倍\n",
	KeyATRLine:                 "- 波動度 ATR(14)：$%.2f（現價的 %.1f%%）\n",
	KeyBollingerLine:           "- 布林通道位置 %%B：%.0f%%（0%%=下軌，100%%=上軌，突破區間會低於 0%% 或高於 100%%）\n",
	KeyCandlesHeader:           "- 近 %d 個交易日 K 線（由舊到新）：\n",
	KeyCandleLine:              "  %s 開 %.2f 高 %.2f 低 %.2f 收 %.2f 量 %d\n",
	KeyTrendBullish:            "多頭",
	KeyTrendBearish:            "空頭",
	KeyTrendUnknown:            "數據不足",
	KeyAboveMA:                 "站上",
	KeyBelowMA:                 "跌破",

	KeyMCPTickerHeader: "## %s\n",

	KeyMCPNoQuote: "查不到 %s 的報價資料。",

	KeyMCPNoHistory:     "查不到 %s 的歷史價格資料。",
	KeyMCPHistoryResult: "%s 近 %d 個交易日日 K（由舊到新）：\n%s",

	KeyMCPNoNews:   "%s 目前沒有相關新聞。",
	KeyMCPNewsItem: "%d. [%s] %s（%s）\n   %s\n",

	KeyMCPNoFundamentals:        "查不到 %s 的基本面資料。",
	KeyMCPNoFinancialStatements: "查不到 %s 的財報資料。",

	KeyMCPNoEarnings:   "查詢的標的在未來 %d 天內沒有排定財報。",
	KeyMCPEarningsItem: "- %s：%s（%s）\n",

	KeyMCPNoMovers:     "目前沒有市場熱門標的資料。",
	KeyMCPMoversResult: "目前市場熱門標的：%s\n",

	KeyMCPRecentRecsTitle:       "🗒 *歷史推薦記錄（過去 %d 天，由新到舊）*\n\n",
	KeyMCPRecentRecsTitleTicker: "🗒 *%s 的歷史推薦記錄（過去 %d 天，由新到舊）*\n\n",
	KeyMCPRecentRecsTruncated:   "（僅顯示最近 %d 筆，共 %d 筆）\n\n",
	KeyMCPRecentRecLine:         "• %s *%s* %s @ $%.2f（來源：%s）\n  %s\n\n",
	KeyMCPRecentRecLineNoPrice:  "• %s *%s* %s（來源：%s）\n  %s\n\n",
	KeyMCPRecentRecsEmptyTicker: "%s 在過去 %d 天內沒有推薦記錄。",

	KeyMCPTradeInvalidInput:   "交易提案輸入無效：ticker、股數、價格皆須為有效正數，日期（若有提供）格式須為 YYYY-MM-DD。",
	KeyMCPTradeProposalFailed: "建立交易提案失敗：%v",
	KeyMCPTradeProposalBuy:    "已建立買入提案：*%s* %g 股 @ $%.2f（提案 #%d）。這只是提案，需要使用者在 Telegram 上按下確認鍵才會真正入帳，不算已經完成。",
	KeyMCPTradeProposalSell:   "已建立賣出提案：*%s* %g 股 @ $%.2f（提案 #%d）。這只是提案，需要使用者在 Telegram 上按下確認鍵才會真正入帳，不算已經完成。",

	KeyCheckButton:         "🔍 檢查",
	KeyBuyButton:           "🟢 買進",
	KeySellButton:          "🔴 賣出",
	KeyBuyCommandTemplate:  "點擊以下指令複製，補上股數與價格後送出：\n```\n/buy %s <股數> <價格>\n```",
	KeySellCommandTemplate: "點擊以下指令複製，補上股數與價格後送出：\n```\n/sell %s <股數> <價格>\n```",

	KeyDailyReportMarketClosed:   "📅 今天美股休市，暫停產出每日報告。",
	KeyTWDailyReportMarketClosed: "📅 今天台股休市，暫停產出台股日報。",
}
