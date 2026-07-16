package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"argus/internal/db"
	"argus/internal/i18n"
)

// tradePayload is the JSON shape stored in a db.PendingAction's Payload for
// PendingActionRecordBuy/PendingActionRecordSell. internal/bot keeps its own
// copy with matching json tags to decode this once the user confirms — the
// two packages don't share an import for it (mcptools can't import bot, and
// there's no third package both already depend on for this), same
// deliberate small duplication as formatFundamentals/commaf elsewhere in
// this package (see server.go's package doc comment).
type tradePayload struct {
	Ticker string  `json:"ticker"`
	Shares float64 `json:"shares"`
	Price  float64 `json:"price"`
	Fee    float64 `json:"fee"`
	Date   string  `json:"date"`
}

type tradeInput struct {
	Ticker string  `json:"ticker" jsonschema:"US stock ticker symbol, e.g. AAPL"`
	Shares float64 `json:"shares" jsonschema:"number of shares, must be positive"`
	Price  float64 `json:"price" jsonschema:"price per share in USD, must be positive"`
	Fee    float64 `json:"fee,omitempty" jsonschema:"optional transaction fee in USD, default 0"`
	Date   string  `json:"date,omitempty" jsonschema:"optional trade date as YYYY-MM-DD; defaults to today if omitted"`
}

// registerTradeWriteTools adds Phase 4's "交易記錄工具化" tools —
// record_buy/record_sell — when ts.writeDB is non-nil, same gate as
// watchlist_write_tools.go. Unlike add_to_watchlist/remove_from_watchlist,
// these never touch positions/transactions directly: a real trade is a
// higher-stakes, less-reversible write (wrong shares/price corrupts cost
// basis and realized P&L), so this only files a db.PendingAction proposal
// and reports its id back to the model. This MCP subprocess has no Telegram
// bot of its own to show a confirm/reject keyboard with — internal/bot picks
// up "pending" rows after the chat turn that created them, sends the
// confirmation message, and only calls db.RecordBuy/RecordSell once the user
// taps Confirm (see bot.go's pending-action handling and PLAN.md's Phase 4
// "寫入把關基建" item).
func registerTradeWriteTools(s *mcp.Server, ts *toolset) {
	if ts.writeDB == nil {
		return
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "record_buy",
		Description: "Propose recording a stock purchase (BUY). This does NOT record the trade immediately — it creates a proposal the user must confirm via a Telegram button before it actually affects their position/transaction history. Tell the user a confirmation request has been sent.",
	}, ts.recordBuy)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "record_sell",
		Description: "Propose recording a stock sale (SELL). This does NOT record the trade immediately — it creates a proposal the user must confirm via a Telegram button before it actually affects their position/transaction history. Tell the user a confirmation request has been sent.",
	}, ts.recordSell)
}

func (ts *toolset) recordBuy(ctx context.Context, _ *mcp.CallToolRequest, in tradeInput) (*mcp.CallToolResult, any, error) {
	return ts.proposeTrade(db.PendingActionRecordBuy, in, i18n.KeyMCPTradeProposalBuy)
}

func (ts *toolset) recordSell(ctx context.Context, _ *mcp.CallToolRequest, in tradeInput) (*mcp.CallToolResult, any, error) {
	return ts.proposeTrade(db.PendingActionRecordSell, in, i18n.KeyMCPTradeProposalSell)
}

// proposeTrade validates in, JSON-encodes it into a tradePayload, and files
// it as a db.PendingAction of actionType. Validation happens here (not left
// to the eventual db.RecordBuy/RecordSell call) so a malformed model call
// never even reaches the user as a confirmation prompt.
func (ts *toolset) proposeTrade(actionType string, in tradeInput, resultKey i18n.Key) (*mcp.CallToolResult, any, error) {
	ticker := normalizeTicker(in.Ticker)
	date := strings.TrimSpace(in.Date)
	if date == "" {
		date = time.Now().In(cst).Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, nil, ts.mcpErr(i18n.KeyMCPTradeInvalidInput)
	}
	if ticker == "" || in.Shares <= 0 || in.Price <= 0 || in.Fee < 0 {
		return nil, nil, ts.mcpErr(i18n.KeyMCPTradeInvalidInput)
	}

	payload, err := json.Marshal(tradePayload{Ticker: ticker, Shares: in.Shares, Price: in.Price, Fee: in.Fee, Date: date})
	if err != nil {
		return nil, nil, ts.mcpErr(i18n.KeyMCPTradeProposalFailed, err)
	}

	id, err := ts.writeDB.CreatePendingAction(actionType, string(payload))
	if err != nil {
		return nil, nil, ts.mcpErr(i18n.KeyMCPTradeProposalFailed, err)
	}

	return textResult(i18n.T(ts.lang, resultKey, ticker, in.Shares, in.Price, id)), nil, nil
}
