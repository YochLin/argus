package i18n

// zhMessages is the original Traditional Chinese (zh-TW) message table —
// the project's default language. Every format verb (%s/%d/%v/%.2f/...)
// here must appear in the same order as its en.go counterpart, since call
// sites pass positional args once and reuse them for whichever table T
// picks.
var zhMessages = map[Key]string{
	KeyUnknownCommand:                "未知指令。可用指令：\n/add <ticker>\n/remove <ticker>\n/list\n/status [ticker]\n/recommend\n/check <ticker>\n/track [天數]\n/buy <ticker> <股數> <價格> [手續費]\n/sell <ticker> <股數> <價格> [手續費]\n/stop <ticker> [價格]\n/buyalert <ticker> [價格|remove 價格]\n/portfolio\n/obuy /osell /oassign /oexercise\n/option <ticker> [call|put|csp|cc]\n/insight\n/cash [金額]\n/thesis <ticker> [論點文字]\n/review <ticker>\n/events [ticker]\n/dailyreport\n/morningreport\n/monthlyreport\n/fundamentals <ticker>\n/universe [add|remove] <ticker>\n/reset\n/paper [reset]\n/podcast <url>\n\n直接傳一般訊息（不加 / 開頭）則會跟我自由對話。",
	KeyAddUsage:                      "用法：/add <ticker>，例如 /add AAPL",
	KeyAddFailed:                     "新增失敗：%v",
	KeyAddSuccess:                    "已將 *%s* 加入自選股。",
	KeyRemoveUsage:                   "用法：/remove <ticker>，例如 /remove AAPL",
	KeyRemoveFailed:                  "移除失敗：%v",
	KeyRemoveSuccess:                 "已將 *%s* 從自選股移除。",
	KeyQueryFailed:                   "查詢失敗：%v",
	KeyWatchlistEmptyHint:            "自選股清單是空的，使用 /add <ticker> 新增。",
	KeyWatchlistTitle:                "📋 *自選股清單*\n",
	KeyWatchlistEmpty:                "自選股清單是空的。",
	KeyMarketStatusTitle:             "📊 *市場即時狀態*\n\n",
	KeyQuoteUnavailable:              "*%s*：無法取得報價\n",
	KeyQuoteFailed:                   "無法取得 %s 報價：%v",
	KeyAnalyzing:                     "分析中，請稍候...",
	KeyWatchlistQueryFailed:          "查詢自選股失敗：%v",
	KeyLLMFailed:                     "LLM 分析失敗：%v",
	KeyNoRecommendations:             "LLM 未回傳推薦標的，請稍後再試。",
	KeyRecParseFailed:                "無法解析 LLM 回覆的推薦內容（格式與預期不符），原始回覆已寫入 log：%v",
	KeyRecommendationsTitle:          "🤖 *今日推薦標的*\n\n",
	KeyRecommendUsage:                "用法：/recommend [tw|us]（不帶參數＝依序跑美股與台股兩份報告）",
	KeyRecWatchlistSectionTitle:      "📋 *自選股與持倉*\n",
	KeyRecCandidatesSectionTitle:     "🆕 *新標的推薦*\n",
	KeyRecWatchlistNoneActionable:    "自選股與持倉今日無需操作。",
	KeyRecCandidatesAnalyzedNone:     "已分析 %d 檔候選標的，今日沒有值得推薦的新標的。",
	KeyRecCandidatesUnavailable:      "今日無候選標的可分析（漲跌幅榜取得失敗或無策略命中）。",
	KeySizingLine:                    "💰 風險倉位建議：可承受虧損 %s，停損參考 %s（現價−2×ATR），建議股數 %d\n",
	KeyMarketNewsSummaryTitle:        "📰 *市場新聞摘要*\n\n",
	KeyCheckUsage:                    "用法：/check <ticker>，例如 /check TSLA",
	KeyAnalyzingTicker:               "分析中，請稍候... (%s)",
	KeyCheckResultTitle:              "📈 *%s 即時分析*\n\n%s",
	KeyFundamentalsUsage:             "用法：/fundamentals <ticker>，例如 /fundamentals AAPL",
	KeyFundamentalsNoKey:             "尚未設定 FINNHUB_API_KEY，無法查詢基本面資料。",
	KeyFundamentalsFailed:            "無法取得 %s 基本面資料：%v",
	KeyFundamentalsTitle:             "📊 *%s 基本面*\n\n",
	KeyThinking:                      "思考中，請稍候...",
	KeyChatFailed:                    "對話失敗：%v",
	KeyResetDone:                     "已重置對話記憶。",
	KeySignalAlertTitle:              "🚨 *市場訊號提醒*\n\n",
	KeyDailyReportStart:              "📅 *每日報告開始生成...*",
	KeyMorningBriefingStart:          "📅 *美股日報生成中...*",
	KeyMorningBriefingMarketClosed:   "📅 昨日美股休市，暫停產出美股日報。",
	KeyTWMorningBriefingStart:        "📅 *台股盤前晨報生成中...*",
	KeyTWMorningBriefingMarketClosed: "📅 台股休市中，暫停產出盤前晨報。",
	KeyNoRecommendationsToday:        "LLM 未回傳推薦標的，請稍後再試。",
	KeyTrackUsage:                    "用法：/track [天數]，例如 /track 14（預設 7 天，最多 90 天）",
	KeyTrackTitle:                    "📊 *推薦成效回顧（過去 %d 天）*\n\n",
	KeyTrackEmpty:                    "過去 %d 天內沒有推薦記錄。",
	KeyTrackLine:                     "• %s *%s* %s  %s → %s (%+.2f%%) %s\n",
	KeyTrackLineVsSPY:                "• %s *%s* %s  %s → %s (%+.2f%% vs %s %+.2f%%) %s\n",
	KeyTrackLineNoPrice:              "• %s *%s* %s — 無推薦時價格可比對\n",
	KeyTrackSummary:                  "\n命中率：%d/%d (%.0f%%)\n（有同期基準指數（SPY/0050）資料時，BUY 需跑贏基準、SELL 需跑輸基準才算命中；缺基準資料則以絕對漲跌計算；HOLD 與無價格資料者不計入）",
	KeyTrackAvgReturnLine:            "平均報酬：BUY %+.2f%%（%d 筆）｜SELL %+.2f%%（%d 筆）\n",
	KeyTrackBySourceHeader:           "\n按來源分組：\n",
	KeyTrackBySourceLine:             "- %s：%d/%d (%.0f%%)\n",
	KeyTrackByMarketHeader:           "\n按市場分組：\n",
	KeyTrackByMarketLine:             "- %s：%d/%d (%.0f%%)\n",

	KeyBuyUsage:               "用法：/buy <ticker> <股數> <價格> [手續費] [日期]，例如 /buy AAPL 10 205.5 或 /buy AAPL 10 205.5 1.5 2026-01-15（回填歷史交易日期）",
	KeyBuyFailed:              "記錄買入失敗：%v",
	KeyBuySuccess:             "✅ 已記錄買入 *%s* %g 股 @ %s（手續費 %s）\n目前持倉：%g 股，平均成本 %s",
	KeySellUsage:              "用法：/sell <ticker> <股數> <價格> [手續費] [日期]，例如 /sell AAPL 5 210 或 /sell AAPL 5 210 1 2026-01-15（回填歷史交易日期）",
	KeySellFailed:             "記錄賣出失敗：%v",
	KeySellSuccess:            "✅ 已記錄賣出 *%s* %g 股 @ %s（手續費 %s）\n已實現損益：%+.2f\n剩餘持倉：%g 股",
	KeyUndoUsage:              "用法：/undo <ticker>，刪除該股票最近一筆交易紀錄（僅限最後一筆，用於修正輸入錯誤）",
	KeyUndoNoTx:               "%s 沒有可刪除的交易紀錄",
	KeyUndoNotFound:           "找不到這筆交易紀錄（可能已被刪除）",
	KeyUndoNotLatest:          "只能刪除該股票最近一筆交易紀錄，較早的紀錄請改用反向買賣沖銷",
	KeyUndoFailed:             "刪除交易失敗：%v",
	KeyUndoBuySuccess:         "✅ 已刪除 *%s* 買入 %g 股 @ %s 的交易紀錄，持倉與現金已還原",
	KeyUndoSellSuccess:        "✅ 已刪除 *%s* 賣出 %g 股 @ %s 的交易紀錄，持倉與現金已還原",
	KeySellNoPosition:         "沒有 *%s* 的持倉可供賣出。",
	KeySellInsufficientShares: "*%s* 持股不足，無法賣出這麼多股。",
	KeyFeeAutoNote:            "\n手續費為系統依券商折扣試算，實際以對帳單為準。",

	KeyStopUsage:            "用法：/stop <ticker> [價格]（不帶價格則查詢目前設定與候選停損價），例如 /stop AAPL 190",
	KeyStopNoPosition:       "沒有 *%s* 的持倉，無法設定停損。",
	KeyStopInvalidPrice:     "停損價 %s 必須低於最新收盤 %s（做多停損只能設在價格下方）。",
	KeyStopSet:              "✅ 已設定 *%s* 停損 %s（距現價 %.1f%%，單股風險 %s）",
	KeyStopShow:             "*%s* 目前停損：%s\n",
	KeyStopNotSet:           "*%s* 尚未設定個股停損，目前退回全域停損門檻 %.1f%%。\n",
	KeyStopCandidatesHeader: "候選停損參考價：\n",
	KeyStopCandidateLine:    "  • %s：%s\n",
	KeyStopLow10Label:       "近10日低",
	KeyStopLow20Label:       "近20日低",
	KeyStopATRLabel:         "現價−2×ATR",
	KeyBuyStopSuggestion:    "\n💡 停損參考：\n",
	KeyBuyStopAddOnNote:     "*%s* 目前停損 %s，加碼後可用 /stop 重新設定。\n",

	KeyBuyAlertUsage:       "用法：/buyalert <ticker> [價格 | remove <價格>]（不帶價格則列出目前設定的購買點提醒），例如 /buyalert AAPL 190",
	KeyBuyAlertQueryFailed: "查詢失敗：%v",
	KeyBuyAlertSet:         "✅ 已為 *%s* 設定購買點提醒 %s，將於股價%s時通知。",
	KeyBuyAlertListHeader:  "*%s* 的購買點提醒：\n",
	KeyBuyAlertLine:        "  • %s — 股價%s時通知\n",
	KeyBuyAlertEmpty:       "*%s* 尚未設定購買點提醒。",
	KeyBuyAlertRemoved:     "已移除 *%s* %s 的購買點提醒。",
	KeyBuyAlertNotFound:    "找不到 *%s* %s 的購買點提醒。",
	KeyBuyAlertDirBelow:    "跌到或低於目標價",
	KeyBuyAlertDirAbove:    "漲到或高於目標價",

	KeyPortfolioTitle:              "💼 *資產總覽*\n\n",
	KeyPortfolioEmpty:              "目前沒有持倉。使用 /buy <ticker> <股數> <價格> 記錄買入。",
	KeyPortfolioLine:               "*%s*  %g 股 @ 均價 %s\n現價 %s　市值 %s　未實現損益 %+.2f (%+.2f%%)\n\n",
	KeyPortfolioSummary:            "—\n總市值：$%.2f\n累計已實現損益：%+.2f",
	KeyPortfolioSectionUS:          "*美股 (USD)*\n",
	KeyPortfolioSectionTW:          "*台股 (TWD)*\n",
	KeyPortfolioSummaryTWD:         "—\n總市值：NT$%.2f\n累計已實現損益：%+.2f",
	KeyPortfolioSummaryWithCash:    "—\n總市值：$%.2f\n累計已實現損益：%+.2f\n含現金總資產：$%.2f（現金 $%.2f）",
	KeyPortfolioSummaryWithCashTWD: "—\n總市值：NT$%.2f\n累計已實現損益：%+.2f\n含現金總資產：NT$%.2f（現金 NT$%.2f）",
	KeyPortfolioLotSuffix:          "（= %d 張）",
	KeyJobPanic:                    "⚠️ *%s* 執行時發生未預期錯誤，已中止：%v",

	KeyInsightResultTitleUS: "💡 *投資組合見解｜美股*\n\n%s",
	KeyInsightResultTitleTW: "💡 *投資組合見解｜台股*\n\n%s",

	KeyWeeklyReviewStart:                  "📅 正在產生本週組合回顧...",
	KeyWeeklyReviewResultTitleUS:          "📅 *每週組合回顧｜美股*\n\n%s",
	KeyWeeklyReviewResultTitleTW:          "📅 *每週組合回顧｜台股*\n\n%s",
	KeyWeeklyNetWorthLine:                 "📈 總市值：$%.2f（較一週前 %+.1f%%）\n",
	KeyWeeklyNetWorthLineWithCash:         "📈 總市值：$%.2f（較一週前 %+.1f%%）｜含現金總資產：$%.2f\n",
	KeyWeeklyNetWorthLineTWD:              "📈 台股總市值：NT$%.2f（較一週前 %+.1f%%）\n",
	KeyWeeklyNetWorthLineWithCashTWD:      "📈 台股總市值：NT$%.2f（較一週前 %+.1f%%）｜含現金總資產：NT$%.2f\n",
	KeyWeeklyEarningsPreviewTitle:         "\n📅 *下週財報前瞻*\n",
	KeyWeeklyEarningsPreviewLine:          "- %s：%s（%d 天後）\n",
	KeyWeeklyEarningsPreviewLineEstimated: "- %s：法定財報截止日 %s（%d 天後，估算，非公司公告日）\n",

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
	KeyStopLossAlertLine:        "• *%s* 成本 %s → 現價 %s，未實現損失 %.1f%%，已達停損門檻\n",
	KeyStopPriceHit:             "• *%s* 停損價 %s，現價 %s，已跌破停損價\n",
	KeyTrailingStopAlertTitle:   "📉 *移動停損提醒*\n\n",
	KeyTrailingStopAlertLine:    "• *%s* 持有期間最高收盤 %s → 現價 %s，回落 %.1f%%，已達移動停損門檻\n",
	KeyTrailingStopAlertLineATR: "• *%s* 持有期間最高收盤 %s → 現價 %s，回落 %.1f%%，已達移動停損門檻（門檻 %.1f%%，%.1f×ATR）\n",

	KeyTargetReached: "🎯 *%s* 已達 %.1fR 獲利目標！目標價 %s，現價 %s——考慮獲利了結一半，剩餘部位改用 5MA 防守。\n",
	KeyMA5Break:      "📉 *%s* 跌破 5 日均線 %s（現價 %s），強勢段防線失守，考慮了結剩餘部位。\n",

	KeyBuyAlertTitle:   "📈 *購買點提醒*\n\n",
	KeyBuyAlertHitLine: "• *%s* 目標價 %s，現價 %s — 已觸發購買點提醒（股價%s）\n",

	KeyChatContextHeader:       "[背景資料，非即時報價，僅供參考]\n",
	KeyChatContextTickerNoData: "- %s：尚無收盤資料\n",
	KeyChatContextWatchLine:    "- %s：%s 收盤 %s (%+.2f%%)\n",
	KeyChatContextPositionLine: "- %s：%s 收盤 %s (%+.2f%%)｜持倉 %g 股，均價 %s，未實現損益 %+.2f%%\n",
	KeyChatContextFooter:       "[背景資料結束，請針對以下訊息回覆]\n\n",

	KeyFetchingArticle:    "正在讀取文章內容，請稍候...",
	KeyArticleFetchFailed: "無法擷取這個網址的內容：%v\n（可能是付費牆或該網站阻擋自動存取，也可以直接把文章內文貼給我）",
	KeyArticleTaskBlock:   "使用者在聊天中分享了一篇文章連結。請先用 3-5 句摘要文章重點，再說明這篇文章跟使用者的自選股/持倉有沒有關聯、有什麼潛在影響——如果沒有實質關聯，直接說沒有即可，不要牽強附會。\n\n標題：%s\n網址：%s\n\n內文：\n%s\n\n使用者的訊息：%s\n",

	KeyEarningsAlertTitle:         "📅 *財報日提醒*\n\n",
	KeyEarningsAlertLine:          "• *%s* 將於 %s 公布財報（%d 天內）\n",
	KeyEarningsAlertLineEstimated: "• *%s* 法定財報截止日為 %s（%d 天內，估算，非公司公告日）\n",

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
	KeyPendingBuyConfirm:            "📝 *交易提案待確認*\n買入 *%s* %g 股 @ %s（手續費 %s，日期 %s）\n請按下方按鈕確認或取消：",
	KeyPendingSellConfirm:           "📝 *交易提案待確認*\n賣出 *%s* %g 股 @ %s（手續費 %s，日期 %s）\n請按下方按鈕確認或取消：",
	KeyPendingActionFromSinopac:     "\n（來源：永豐證券同步）",
	KeySinopacNotConfigured:         "永豐 Shioaji 尚未設定（SHIOAJI_ADDR 未設定）。",
	KeySinopacSyncFailed:            "永豐同步失敗：%v",
	KeySinopacSyncNone:              "永豐同步：近 7 天沒有新的手動交易。",
	KeySinopacSyncDryRunTitle:       "🔍 *永豐同步（僅列出，未建立提案）*",
	KeySinopacSyncTitle:             "✅ *永豐同步：已建立 %d 筆交易提案*",
	KeySinopacSyncNoDseq:            "（無 dseq，以合成鍵去重）",
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

	KeyStrategySqueezeBreakout:            "🚀 %s 觸發【布林壓縮噴發】(%s): 帶寬 20 日新低後爆量突破上軌，MACD 柱翻紅",
	KeyStrategyBoxBottom:                  "🎯 %s 觸發【箱型地板抄底】(%s): 30 日箱型底 + KD 低檔金叉/勾頭 + RSI 止跌反彈",
	KeyDaysAgoToday:                       "今日",
	KeyDaysAgoN:                           "%d 天前",
	KeyTechnicalsKDLine:                   "- KD(9,3,3)：K=%.1f，D=%.1f\n",
	KeyTechnicalsBandwidthLine:            "- 布林帶寬：%.1f%%\n",
	KeyTechnicalsMAAlignLine:              "- 均線排列(5/20/60)：%s\n",
	KeyTechnicalsVolumePriceLine:          "- 量價對照：%s\n",
	KeyTechnicalsNewHighLine:              "- 新高紀錄：%s\n",
	KeyTechnicalsMACDZeroLine:             "- MACD 零軸位置：%s (MACD=%.2f)\n",
	KeyTechnicalsRSLine:                   "- 相對強度 RS63：%s (超越 SPY %+.1f%%)\n",
	KeyStrategyHitLine:                    "- ⚠️ 策略觸發：%s (%s)\n",
	KeyStrategyBearRegimeWarning:          "⚠️ 警告：當前大盤趨勢為空頭/走弱，逆勢策略警報勝率較低，切勿盲目抄底/追高。",
	KeyStrategyUnvalidated:                "ℹ️ 網 3 未通過出樣本驗證：四個獨立樣本的超額報酬皆為負、約 4σ（cmd/strategyscan）。僅供參考，勿單獨作為進場依據。",
	KeyStrategyUnvalidatedBoxBottom:       "ℹ️ 網 2 未通過出樣本驗證：大型股與中型股、兩個時間切片的超額報酬皆為負或不顯著（cmd/strategyscan）。僅供參考，勿單獨作為進場依據。",
	KeyStrategyUnvalidatedTrustFollow:     "ℹ️ 網 5 未通過出樣本驗證：兩個時間切片的超額報酬一負一不顯著（cmd/strategyscan，對 TWSE T86 正典資料源的重測）。僅供參考，勿單獨作為進場依據。",
	KeyStrategyUnvalidatedSqueezeBreakout: "ℹ️ 網 1 未通過出樣本驗證：S&P 500 與台股，兩個時間切片的超額報酬皆未過 1 個標準誤（cmd/strategyscan）。僅供參考，勿單獨作為進場依據。",
	KeyStrategyUnvalidatedTrendPullback:   "ℹ️ 網 4 未通過出樣本驗證：S&P 500 與台股，兩個時間切片的超額報酬皆未過 1 個標準誤（cmd/strategyscan）。僅供參考，勿單獨作為進場依據。",
	KeyTechGuidanceBlock:                  "\n【技術面與量價判讀指引】\n1. 支撐壓力：連續長下影線/雙重底(W底)為強支撐；爆量長紅 K 低點為主力成本支撐，跌破即棄守；突破後舊壓力轉支撐。\n2. 量價配合：量增價漲=趨勢啟動；量縮價跌=健康洗盤；量縮價漲=虛胖誘多；量增價跌=恐慌倒貨勿接刀。\n3. MACD：零軸上金叉力道較強，零軸下金叉多為弱反彈；價格創新高但柱狀體峰值遞減為頂背離警訊。\n4. KD/RSI：KD 出超賣區(翻越20/25)再動作，避免低檔鈍化接刀；RSI 50 為多空分界線，站穩 50 之拉回為強勢點。\n5. 多頭排列 (MA5 > MA20 > MA60 全朝上) 時拉回 MA20 為動態支撐點。\n6. 給出 BUY 建議時，原因中務必點名一個具體的建議停損位（結構性支撐位，如近10/20日低點，或現價減2倍ATR），並說明依據——這是給人工執行的參考，不是要你去算股數。\n",
	KeyVolUpPriceUp:                       "量增價漲 (多頭發力)",
	KeyVolDownPriceDown:                   "量縮價跌 (健康洗盤)",
	KeyVolDownPriceUp:                     "量縮價漲 (虛胖誘多)",
	KeyVolUpPriceDown:                     "量增價跌 (恐慌倒貨)",
	KeyNewHigh20And52:                     "創 20 日與 52 週新高",
	KeyNewHigh20:                          "創 20 日新高",
	KeyNewHigh52:                          "創 52 週新高",
	KeyMACDAboveZero:                      "零軸之上 (強勢區)",
	KeyMACDBelowZero:                      "零軸之下 (弱勢區)",
	KeyRSStronger:                         "強於大盤",
	KeyRSWeaker:                           "弱於大盤",
	KeyStrategySqueezeName:                "布林壓縮噴發",
	KeyStrategyBoxName:                    "箱型地板抄底",

	KeySystemPromptAnalyst: "你是一位專業的美股分析師，只負責針對使用者提供的市場數據給出文字分析。你沒有任何工具可用，也不需要使用工具；請直接依照使用者訊息中的格式要求回覆，不要輸出與分析無關的內容。",
	KeySystemPromptChat:    "你是使用者的個人助理，請用繁體中文自然對話，記住這段對話中先前提到的內容。你沒有任何工具可用，也不需要使用工具。",

	KeyRecPromptIntro:           "你是一位美股分析師，請根據以下市場數據給出今日操作建議。\n\n",
	KeyRecPromptIntroTW:         "你是一位台股分析師，請根據以下市場數據給出今日操作建議。\n\n",
	KeyRecTWMarketNote:          "本批為台灣上市櫃股票，金額為新台幣（TWD），且個股單日漲跌幅上限為 ±10%。\n\n",
	KeyMarketRegimeHeader:       "## 大盤情境\n\n",
	KeyMarketRegimeSPYLine:      "• %s 現價 %s，MA200 %s，MA50 %s — %s\n",
	KeyMarketRegimeVIXLine:      "• VIX %.2f — %s\n",
	KeyMarketRegimeVolProxyLine: "• 波動度代理指標（ATR14/收盤價）%.1f%% — 台股無官方波動率指數，僅供參考\n",
	KeyRiskOn:                   "多頭（現價站上 MA200）",
	KeyRiskOff:                  "空頭（現價跌破 MA200）",
	KeyVIXCalm:                  "平靜",
	KeyVIXNormal:                "正常",
	KeyVIXPanic:                 "高恐慌",
	KeyRecMarketNewsHeader:      "## 大盤新聞\n\n",
	KeyRecWatchlistHeader:       "## 我的自選股\n\n",
	KeyRecNoWatchlist:           "（無自選股）\n\n",
	KeyRecMoversHeader:          "## 大盤熱門標的\n\n",
	KeyRecNoCandidates:          "（無候選標的）\n\n",
	KeyRecMarketSummaryTask: `
在逐檔分析之前，請先輸出一個區塊，第一行必須是：
%s
接著用 3–5 條重點統整以上市場新聞，並指出可能受惠或受害的類股/標的（總長 150 字以內）。
之後再接續逐檔分析區塊。
`,
	KeyRecTaskBlock: `
## 任務

逐一查看上方的標的，但只輸出真正需要使用者注意的項目：區塊內有「持倉:」那行的
每一檔都必須給出明確操作建議，動作只能是 BUY、SELL、HOLD 三者之一，並維持完整分析；
沒有「持倉:」的自選股只有在你認為值得現在買進時才輸出（動作為 BUY），否則整個區塊不要輸出；
大盤熱門標的維持相同規則，只挑出你看好、值得買進的（最多 5 檔，動作為 BUY），沒有就不列。

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

輸出格式請嚴格遵守以下結構，每個要輸出的標的一個區塊，不要有額外文字：

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

	KeyCheckPromptIntro:   "你是一位美股分析師，請針對以下標的給出即時市場分析。\n\n",
	KeyCheckPromptIntroTW: "你是一位台股分析師，請針對以下標的給出即時市場分析。\n\n",
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
3. 相對大盤表現：若附有「相對大盤」數據，指出哪些持倉其實跑輸該檔所屬市場的大盤指標（數據中已標明比較基準），
   光是上漲不代表這筆投資是對的
4. 加碼／減碼／停損建議：哪些部位該加碼、該減碼、或該設停損，並說明理由
5. 若有提供現金水位，簡短評論目前的持倉／現金配置是否合理

回覆請簡潔，總字數控制在 600 字以內。
`,

	KeyWeeklyReviewPromptIntro: "你是一位股票分析師，請針對以下整體持倉組合給出本週回顧（不是逐檔重複個股分析）。\n\n",
	KeyWeeklyReviewTrackHeader: "\n本週推薦追蹤成效：\n",
	KeyWeeklyReviewPromptTask: `
請用繁體中文分析整個投資組合（不要逐檔重複已列出的數據，聚焦組合層級的判斷）：
1. 集中度風險：單一持股或單一產業是否佔比過高（產業請根據你自己對這些標的的認識判斷，不用等資料提供）
2. 持有理由檢視：若某檔附有「持有論點」，請直接挑戰它——對照近期新聞與基本面，這個論點是否還站得住腳，
   哪裡已經變了；沒有附論點的標的，則檢視當初可能的持有邏輯是否仍然成立
3. 相對大盤表現：若附有「相對大盤」數據，指出哪些持倉其實跑輸該檔所屬市場的大盤指標（數據中已標明比較基準），
   光是上漲不代表這筆投資是對的
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

	KeyTradeFollowupBlock: "\n賣出後走勢（第 %d 個交易日收盤）：賣出均價 $%.2f → 收盤 $%.2f（%+.1f%%），期間最高 $%.2f／最低 $%.2f\n",
	KeyTradeFollowupPromptTask: `
請用繁體中文，事後回頭評估這筆賣出，誠實、不粉飾：
1. 這個出場點對不對：對照上方「賣出後走勢」的數字，這筆賣出是賣飛了（賣完繼續漲）、賣在相對高點／
   止損正確（賣完轉弱），還是時點差不多？用數字說話，不要空泛評語。
2. 若上方附有「持有論點」或推薦紀錄，一併對照——賣出後的走勢有沒有印證或推翻它們？
3. 下次遇到同樣的型態，出場機制該怎麼調整（分批出場？移動停利？停損位放寬或收緊？）——給具體建議，
   不要「要更謹慎」這種空話。
4. 提醒自己：這只是賣出後 5 個交易日的短期樣本，不要把短期雜訊講成鐵律。

回覆最後另起一行，開頭必須是：
%s <這條教訓，2–4 句話以內>

回覆請簡潔，總字數控制在 300 字以內。
`,

	KeyTradeFollowupResultTitle:      "📓 *賣後追蹤：%s*（第 %d 個交易日）\n%s\n\n%s",
	KeyTradeFollowupVerdictSoldEarly: "賣飛了：賣出後續漲",
	KeyTradeFollowupVerdictGoodExit:  "賣在相對高點／止損正確",
	KeyTradeFollowupVerdictNeutral:   "出場時點差不多",

	KeyPriceEventPromptIntro:    "你是一位股票分析師，*%s* 今天觸發了值得記錄的價格事件，請根據以下資料整理這次事件的情況說明。\n\n",
	KeyPriceEventGapLine:        "開盤跳空：%+.1f%%（相對前收）%s\n",
	KeyPriceEventChangeLine:     "當日漲跌：%+.1f%%（收盤相對前收）%s\n",
	KeyPriceEventCumulativeLine: "近期累積漲跌：%+.1f%%%s\n",
	KeyPriceEventTriggerSuffix:  "　← 本次觸發門檻",
	KeyPriceEventScaleLine:      "波動尺度：當日變動約為 ATR(14) 的 %.1f 倍，成交量為近 20 日均量的 %.1f 倍\n",
	KeyPriceEventPromptTask: `
請用繁體中文，依下列要求寫 3–5 句話：
1. 先陳述上方跳空／漲跌／累積的事實數字；標「← 本次觸發門檻」的那一項才是這次記錄的原因，其餘是同一天的背景事實（例如跳空後收盤幾乎持平，代表盤中買盤把跌幅收了回來，這種對比值得講）
2. 若上方附有相關新聞，依新聞推測可能原因（括號內為發佈日期）；若沒有附上任何新聞，請直接說「無對應新聞，原因不明」，不要憑空臆測
3. 若上方附有波動尺度，請用它說明這次變動有多異常（例如「約 3 倍 ATR、量能 2.4 倍」），不要只重複百分比
4. 這是事實性描述，不要給任何買賣操作建議或語氣
`,
	KeyPriceEventResultTitle:        "📊 *價格事件：%s*\n\n%s",
	KeyPriceEventOverflowLine:       "另有以下標的今日也觸發價格事件（超過本次上限，未附摘要）：\n%s完整記錄請見 /events",
	KeyPriceEventOverflowTickerLine: "  • %s：跳空 %+.1f%% ／ 漲跌 %+.1f%% ／ 累積 %+.1f%%\n",
	KeyEventsListTitle:              "📊 *近期價格事件*\n",
	KeyEventsTickerTitle:            "📊 *%s 價格事件紀錄*\n",
	KeyEventsLine:                   "%s │ %s │ 跳空 %+.1f%% ／ 漲跌 %+.1f%% ／ 累積 %+.1f%% │ %s\n",
	KeyEventsNoSummary:              "（無摘要）",
	KeyEventsEmpty:                  "目前沒有價格事件紀錄。",

	KeyMorningBriefingPromptIntro:     "你是一位美股分析師，請根據以下資料撰寫一篇風格類似晨間簡報的美股日報，以自然流暢的敘事語氣呈現，而非條列式的操作建議。\n\n",
	KeyMorningBriefingIndicesHeader:   "## 大盤收盤\n\n",
	KeyMorningBriefingIndexLine:       "%s：%.2f（%+.2f%%）\n",
	KeyMorningBriefingVIXLine:         "VIX：%.2f\n",
	KeyMorningBriefingNewsHeader:      "\n## 宏觀與市場新聞\n\n",
	KeyMorningBriefingWatchlistHeader: "\n## 觀察名單\n\n",
	KeyMorningBriefingMoversHeader:    "\n## 大盤熱門標的\n\n",
	KeyMorningBriefingTaskBlock: `
## 任務

請撰寫 %s 的美股日報，格式與語氣請參考以下範本：

📅 美股日報｜[日期，中文年月日＋星期]

[一句話點出當天最大主線，例如財報、總經事件、或市場情緒轉折]

📊 大盤收盤
[依上方指數資料，每個指數一行，🟢 代表上漲、🔴 代表下跌]

🌐 宏觀背景
[根據上方新聞，用一段文字說明總經、Fed、經濟數據等背景；沒有相關新聞就精簡帶過或省略]

👀 觀察名單
[根據上方自選股資料，每檔一行：漲跌 emoji、漲跌幅、$TICKER 現價，附上簡短原因或新聞]

🔥 科技焦點／💥 跨板塊熱點
[根據上方大盤熱門標的與新聞，挑出當天真正值得一提的個股或類股動態；沒有明顯亮點可省略此段]

🚀 未上市觀察
[僅在上方新聞明確提到未上市公司／IPO 相關消息時才寫這段；沒有依據就整段省略，不要虛構]

有任何問題隨時可以討論或是問我

請只根據上方實際提供的資料撰寫，沒有資料支持的段落請整段省略，不要編造內容。
`,

	KeyTWMorningBriefingPromptIntro: "你是一位台股分析師，現在是台股開盤前 30 分鐘，請根據以下資料（前一交易日台股收盤、隔夜美股表現、VIX、市場新聞）撰寫一篇盤前晨報，以自然流暢的敘事語氣呈現，而非條列式的操作建議。請記得台股採當日 ±10% 漲跌幅限制，價格請以新台幣表示。\n\n",
	KeyTWMorningBriefingTaskBlock: `
## 任務

請撰寫 %s 的台股盤前晨報，格式與語氣請參考以下範本：

📅 台股盤前晨報｜[日期，中文年月日＋星期]

[一句話點出今日開盤前最值得留意的主線，例如隔夜美股表現、VIX 變化、或重大新聞]

📊 前一交易日收盤與隔夜美股
[依上方指數資料，每個指數一行，🟢 代表上漲、🔴 代表下跌]

🌐 市場新聞
[根據上方新聞，用一段文字說明對今日台股開盤可能的影響；沒有相關新聞就精簡帶過或省略]

👀 觀察名單
[根據上方自選股資料，每檔一行：漲跌 emoji、漲跌幅、$TICKER 現價，附上簡短原因或新聞]

🔥 熱門標的
[根據上方大盤熱門標的與新聞，挑出今日開盤前真正值得一提的個股或類股動態；沒有明顯亮點可省略此段]

有任何問題隨時可以討論或是問我

請只根據上方實際提供的資料撰寫，沒有資料支持的段落請整段省略，不要編造內容。
`,

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
	KeyNewsItem:                "  %d. [%s] %s（%s）\n",
	KeyNewsSummaryLine:         "     %s\n",
	KeyNewsTickersLine:         "     相關標的: %s\n",
	KeyFundamentalsSummaryLine: "- 基本面：本益比 %.1f｜股價淨值比 %.1f｜ROE %.1f%%｜毛利率 %.1f%%｜營業利益率 %.1f%%｜淨利率 %.1f%%｜負債權益比 %.2f｜營收年增率 %.1f%%｜EPS年增率 %.1f%%｜股息殖利率 %.2f%%｜Beta %.2f｜EPS(TTM) %.2f｜流動比率 %.2f｜市值 $%.0fM｜距52週高點 %+.1f%%｜距52週低點 %+.1f%%\n",
	KeyMonthRevenueYoYLine:     "- 月營收年增率：%+.1f%%\n",
	KeyAnalystRatingLine:       "- 分析師評等（%s）：強力買進 %d｜買進 %d｜持有 %d｜賣出 %d｜強力賣出 %d\n",
	KeyAnalystRatingTrendLine:  "  較上月：買進方 %+d｜賣出方 %+d\n",
	KeyInsiderTxLine:           "- 內部人交易（近期公開市場申報，最近一筆 %s）：買進 %d 筆共 %d 股｜賣出 %d 筆共 %d 股\n",
	KeyEarningsSurpriseLine:    "- 近 %d 季財報：%d 勝 %d 負，平均 surprise %+.1f%%（最近一季 %s：%+.1f%%）\n",
	KeyInstitutionalFlowLine:   "- 三大法人買賣超（%s）：外資 %+d｜投信 %+d｜自營商 %+d｜合計 %+d 股\n",
	KeyStatementSummaryLine:    "- 最新財報（%s，會計年度 %d，截至 %s）：營收 $%.0fM｜毛利 $%.0fM｜營業利益 $%.0fM｜淨利 $%.0fM｜總資產 $%.0fM｜總負債 $%.0fM｜股東權益 $%.0fM｜營運現金流 $%.0fM｜自由現金流 $%.0fM\n",
	KeyValuationEPSLine:        "- SEC 年度 EPS（最近財報年度迄 %s）：%.2f\n",
	KeyValuationPELine:         "- 依目前股價換算 P/E：%.1f\n",
	KeyValuationPercentileLine: "- 估值百分位（僅與自身歷史比較，不可跨市場比較）：%.0f%%\n",
	KeyCashFlowQualityLine:     "- 現金流品質（同財報年度營運現金流 / 淨利）：%.2f\n",
	KeyPositionLine:            "- 持倉: %g 股　平均成本 $%.2f　未實現損益 %+.2f%%\n",
	KeyPrevRecLine:             "- 上次建議: %s @ $%.2f（%d 天前）\n",
	KeyEarningsLine:            "- ⚠️ 財報日: %s（%d 天內），留意波動\n",
	KeyEarningsLineEstimated:   "- ⚠️ 法定財報截止日（估算）: %s（%d 天內），留意波動\n",
	KeyScanHitLine:             "- 🔎 掃描命中: %s\n",
	KeyThesisLine:              "- 📝 持有論點: %s\n",
	KeyVsSPYLine:               "- 相對大盤: 持有期間 %+.1f%% vs %s 同期 %+.1f%%\n",
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

	KeyMCPNoInsiderTx:   "查不到 %s 的內部人交易資料。",
	KeyMCPInsiderTxItem: "- %s（%s）：%+d 股，代碼 %s，價格 %.2f\n",

	KeyMCPNoInstitutionalFlow:     "查不到 %s 的三大法人買賣超資料。",
	KeyMCPInstitutionalFlowResult: "%s 三大法人買賣超（%s）：外資 %+d｜投信 %+d｜自營商 %+d｜合計 %+d 股",

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

	KeyEvalTitle:                 "=== 離線推薦評估報告 ===\n",
	KeyEvalFetching:              "抓取歷史資料中... (%d/%d %s)\n",
	KeyEvalOverviewTitle:         "\n[總覽：動作 × 視窗]\n",
	KeyEvalSourceTitle:           "\n[來源分解：來源 × 視窗]\n",
	KeyEvalMarketTitle:           "\n[市場：%s]\n",
	KeyEvalStatsLine:             "  %s %d日 (n=%d)：命中率 %.1f%%，平均報酬 %+.2f%%，中位數 %+.2f%%，平均超額 %+.2f%%\n",
	KeyEvalExtremesTitle:         "\n[%d 日超額報酬 極端案例]\n",
	KeyEvalExtremesBest:          "最佳 Top %d：\n",
	KeyEvalExtremesWorst:         "最差 Top %d：\n",
	KeyEvalExtremeLine:           "  %s %s %s @ $%.2f -> %d日：%+.2f%%（超額 %+.2f%%，MFE %+.2f%% / MAE %+.2f%%）\n",
	KeyEvalCountsTitle:           "\n[誠實計數]\n",
	KeyEvalCountHold:             "  未評分（HOLD/無動作）：%d 筆\n",
	KeyEvalCountUnscorable:       "  無法評分：%d 筆\n",
	KeyEvalCountUnscorableReason: "    - %s：%d 筆\n",
	KeyEvalCountImmature:         "  視窗未到期（%d日）：%d 筆\n",
	KeyEvalCSVSaved:              "\n已輸出明細 CSV：%s\n",
	KeyEvalNoData:                "沒有可評分的推薦記錄。\n",

	KeyBacktestTitle:           "=== 虛擬帳戶回測報告（%s）===\n",
	KeyBacktestOverviewTitle:   "\n[總覽]\n",
	KeyBacktestOverviewLine:    "起始資金 $%.2f，期末權益 $%.2f，總報酬 %+.2f%%，CAGR %+.2f%%，最大回撤 %.2f%%\n交易 %d 筆，勝率 %.1f%%，獲利因子 %.2f，期望值 $%.2f\n",
	KeyBacktestBenchmarkLine:   "對照大盤 %s 同期買進持有：%+.2f%%（超額 %+.2f%%）\n",
	KeyBacktestExitReasonTitle: "\n[出場原因分解]\n",
	KeyBacktestExitReasonLine:  "  %s：%d 筆，勝率 %.1f%%，平均報酬 %+.2f%%\n",
	KeyBacktestSourceTitle:     "\n[來源分解]\n",
	KeyBacktestSourceLine:      "  %s：%d 筆\n",
	KeyBacktestCountsTitle:     "\n[誠實計數]\n",
	KeyBacktestCountHold:       "  未動作（HOLD/無動作）：%d 筆\n",
	KeyBacktestCountNoHistory:  "  抓不到歷史資料而跳過：%d 檔\n",
	KeyBacktestCountSkippedBuy: "  資金不足而跳過的 BUY：%d 筆\n",

	KeyPaperDisabled:   "虛擬帳戶功能未啟用（未設定 PAPER_DB_PATH）。",
	KeyPaperTitle:      "🧪 *虛擬帳戶*\n\n",
	KeyPaperSectionUS:  "*美股 (USD)*\n",
	KeyPaperSectionTW:  "*台股 (TWD)*\n",
	KeyPaperLine:       "*%s* %g 股 @ 均價 $%.2f\n現價 $%.2f 停損 $%.2f（距停損 %.1f%%）未實現損益 %+.2f (%+.2f%%)\n\n",
	KeyPaperSummary:    "—\n現金：$%.2f\n總權益：$%.2f（起始 $%.2f，報酬 %+.2f%%）\n累計已實現損益：%+.2f",
	KeyPaperSummaryTWD: "—\n現金：NT$%.2f\n總權益：NT$%.2f（起始 NT$%.2f，報酬 %+.2f%%）\n累計已實現損益：%+.2f",
	KeyPaperBuyAlert:   "🧪 虛擬帳戶｜買進 %s %g 股 @ %.2f（停損 %.2f）｜現金 $%.2f\n",
	KeyPaperSellAlert:  "🧪 虛擬帳戶｜賣出 %s %g 股 @ %.2f（%s）已實現損益 %+.2f｜現金 $%.2f\n",
	KeyPaperResetUsage: "用法：/paper reset confirm",
	KeyPaperResetDone:  "🧪 虛擬帳戶已清空並重新播種。",

	KeyOptionUsage:                "用法：/obuy <OCC 代號> <口數> <權利金> [手續費] [日期]，或 /osell 同樣格式，例如 /obuy AAPL260918C00320000 2 5.40",
	KeyOptionTradeFailed:          "記錄期權交易失敗：%v",
	KeyOptionCrossesZero:          "%s 的口數會讓部位穿越零點（多單變空單或反之），請先平倉再反向開倉",
	KeyOptionTradeSuccess:         "✅ 已記錄 %s *%s* %g 口 @ $%.2f（手續費 $%.2f）\n已實現損益：%+.2f\n目前部位：%g 口，平均權利金 $%.2f",
	KeyOptionNakedCallWarning:     "\n⚠️ 裸賣警告：%s 需鎖定 %g 股，但目前僅持有 %g 股，風險無上限",
	KeyOptionResolutionUsage:      "用法：/oassign <OCC 代號> [日期] 或 /oexercise 同樣格式",
	KeyOptionNoPosition:           "找不到 %s 的期權部位",
	KeyOptionAssignRequiresShort:  "%s 是多單，/oassign 只適用於空單（被指派只會發生在賣方）；若是你要履約，請用 /oexercise",
	KeyOptionExerciseRequiresLong: "%s 是空單，/oexercise 只適用於多單（履約只會發生在買方）；若是被指派，請用 /oassign",
	KeyOptionResolveFailed:        "處理期權到期／指派失敗：%v",
	KeyOptionResolveSuccess:       "✅ %s *%s*\n已實現損益：%+.2f",
	KeyPortfolioOptionsSection:    "\n📜 *期權部位*",
	KeyPortfolioOptionLine:        "%s\n口數：%g，平均權利金 $%.2f，現價 $%.2f\n市值：$%.2f\n到期日：%s（剩餘 %d 天）",
	KeyPortfolioOptionUnavailable: "%s 報價暫時無法取得：%v",
	KeyOptionExpiryConfirm:        "期權 %s 已到期，標的收盤 $%.2f、履約價 $%.2f，建議：%s\n請確認",

	KeyOptionSelectUsage:        "用法：/option <ticker> [call|put|csp|cc]，省略策略時預設 call，例如 /option AAPL 或 /option AAPL csp",
	KeyOptionSelectFailed:       "選契約失敗：%v",
	KeyOptionSelectNoCandidates: "%s 目前沒有符合 %s 篩選條件（流動性或 delta/到期天數）的契約",
	KeyOptionSelectLine:         "%s\n現價 $%.2f  Delta %.3f  IV %.1f%%  未平倉量 %d  價差 %.1f%%  剩餘 %d 天",

	KeyStrategyTrendBreakout: "🚀 %s 觸發【趨勢突破】(%s)：創新高 + 均線多頭排列 + 攻擊量 + 乖離未過大",
	KeyStrategyTrendPullback: "🎯 %s 觸發【趨勢回檔】(%s)：季線上彎 + 拉回月線量縮 + KD 低檔轉折 + 止跌 K 線",
	KeyStrategyBreakoutName:  "趨勢突破",
	KeyStrategyPullbackName:  "趨勢回檔",
	KeyStrategyTrustFollow:   "🐳 %s 觸發【主力跟單】(%s)：投信近 %d 日買超佔量 %.1f%% + 站上季線且上彎",

	KeyRestrictedStockAlertTitle: "⚠️ *處置/注意股提醒*\n\n",
	KeyRestrictedStockAlertLine:  "• *%s* 已列入處置/注意股：%s\n",

	KeyPodcastUsage:         "用法：/podcast <podcast 逐字稿網址>",
	KeyPodcastFetching:      "正在讀取節目內容...",
	KeyPodcastFetchFailed:   "讀取失敗：%v",
	KeyPodcastAnalyzing:     "正在分析逐字稿中的市場觀點...",
	KeyPodcastAnalyzeFailed: "分析失敗：%v",
	KeyPodcastNoInsights:    "這集沒有抽到任何市場相關的觀點。",
	KeyPodcastSavedHeader:   "已從這集抽出 %d 則觀點並存檔：\n\n",
	KeyPodcastSavedLine:     "• %s [%s] %s\n",
	KeyPodcastMacroLabel:    "總體經濟",
	KeyPodcastPromptIntro:   "使用者分享了一段股市相關 podcast/影音節目的逐字稿，內容可能包含業配、開場閒聊、聽眾 Q&A 等與投資無關的段落。請只從市場分析段落抽取重點，忽略業配、廣告、以及與投資無關的個人閒聊。\n\n標題：%s\n網址：%s\n\n逐字稿：\n%s\n\n",
	KeyPodcastTaskBlock: `
## 任務

找出逐字稿中提到的個股觀點與總體經濟觀點（沒有對應個股時代號留空），每個觀點各輸出一個區塊，不要有額外文字：

[TICKER: 代號或留空]
%s US 或 TW（沒有對應個股則留空）
%s 觀點只能是 BULLISH、BEARISH、NEUTRAL、WATCH 四者之一
%s 一句話貼近原文的重點，不要自行延伸腦補（100字以內）

如果逐字稿提到某檔標的的需求成長或利多，而你根據自己的產業知識判斷有明確、有機會受惠的下游供應鏈廠商
（美股或台股皆可），也請額外為該廠商輸出一個區塊，並多加一行：
%s <被提到的原標的代號>：<受惠的原因，例如什麼需求成長>
這行代表是你自己根據產業知識做的推論，不是逐字稿原文直接點名的公司，請只在關聯明確時才輸出，
不要為了填滿而亂猜。

如果整段逐字稿沒有任何值得抽取的市場相關內容，不要輸出任何區塊。
`,
	KeyPodcastMarketMarker:     "市場:",
	KeyPodcastStanceMarker:     "觀點:",
	KeyPodcastDerivedMarker:    "推論自:",
	KeyPodcastDerivedLine:      "　└ 推論自：%s（非逐字稿原文直接提及）\n",
	KeyPodcastDuplicateWarning: "⚠️ 這個連結先前已經分析過，已有 %d 則記錄，這次分析完成後將會覆蓋成新的結果。",
}
