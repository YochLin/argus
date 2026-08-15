// Package sinopac is a thin REST client for the Shioaji daemon
// (`shioaji server start`, see deploy/shioaji.service) — Sinopac Securities'
// (永豐證券) Python trading SDK, which exposes its whole API as a local
// HTTP server rather than requiring a shelled-out Python process. The
// daemon is started and logged into separately by the operator (token
// decryption needs a key only they hold); this package only ever talks to
// an already-running daemon.
package sinopac

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client talks to the Shioaji daemon's REST API, live-verified 2026-08-14
// against daemon v1.7.2 (/openapi.json). Two response-shape quirks that
// don't match the OpenAPI spec's field lists at a glance:
//   - /data/regulatory_punish and /data/regulatory_notice return a single
//     object of parallel arrays (pandas DataFrame.to_dict("list") shape),
//     not an array of row objects like every other list endpoint.
//   - /data/scanner's "ascending" flag is inverted from its name:
//     ascending=true returns rank-1-first (highest amount/volume/etc),
//     ascending=false returns near-zero noise. Confirmed by direct
//     comparison against real market data, not documented anywhere.
type Client struct {
	http    *http.Client
	baseURL string
}

// New builds a Client. addr is either a filesystem path to the daemon's
// unix socket (~/.shioaji/server-8080.sock, the default) or a host:port for
// its TCP listener — distinguished by whether addr contains a "/".
func New(addr string) *Client {
	c := &http.Client{Timeout: 10 * time.Second}
	baseURL := "http://shioaji"
	if strings.Contains(addr, "/") {
		c.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", addr)
			},
		}
	} else {
		baseURL = "http://" + addr
	}
	return &Client{http: c, baseURL: baseURL}
}

type apiError struct {
	Message string `json:"message"`
}

// sessionRetryBackoff paces retries for a Solace session drop
// ("SessionNotEstablished") — live-observed on the after-hours simulation
// daemon: every endpoint (even quotes) fails for 1-2 minutes after the
// upstream session drops, then self-heals with no daemon restart needed.
var sessionRetryBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		var reqBody io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reqBody = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("sinopac: %s: %w", path, err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("sinopac: %s: %w", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			var apiErr apiError
			json.Unmarshal(data, &apiErr) //nolint:errcheck // best-effort; falls through to raw body below
			msg := apiErr.Message
			if msg == "" {
				msg = string(data)
			}
			lastErr = fmt.Errorf("sinopac: %s: %s", path, msg)
			if strings.Contains(msg, "SessionNotEstablished") && attempt < len(sessionRetryBackoff) {
				time.Sleep(sessionRetryBackoff[attempt])
				continue
			}
			return lastErr
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(data, out)
	}
}

// Contract identifies a security for Snapshots. SecurityType is always
// "STK" for the equities this package deals with; Exchange is "TSE"
// (TWSE-listed) or "OTC" (TPEx-listed).
type Contract struct {
	SecurityType string `json:"security_type"`
	Exchange     string `json:"exchange"`
	Code         string `json:"code"`
}

// Snapshot is one contract's current quote, matching Shioaji's
// api_v1.data.snapshots.Snapshot exactly (see Client's doc comment).
type Snapshot struct {
	Datetime     string  `json:"datetime"`
	Code         string  `json:"code"`
	Exchange     string  `json:"exchange"`
	Open         float64 `json:"open"`
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
	Close        float64 `json:"close"`
	ChangePrice  float64 `json:"change_price"`
	ChangeRate   float64 `json:"change_rate"`
	TotalVolume  int64   `json:"total_volume"`
	YesterdayVol float64 `json:"yesterday_volume"`
}

func (c *Client) Snapshots(ctx context.Context, contracts []Contract) ([]Snapshot, error) {
	var out []Snapshot
	err := c.do(ctx, http.MethodPost, "/api/v1/data/snapshots", map[string]any{"contracts": contracts}, &out)
	return out, err
}

// ScannerItem is one ranked entry from Scanner, matching Shioaji's
// api_v1.data.scanner.ScannerItem exactly.
type ScannerItem struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Close       float64 `json:"close"`
	ChangePrice float64 `json:"change_price"`
	TotalVolume int64   `json:"total_volume"`
	TotalAmount float64 `json:"total_amount"`
}

// ScannerType matches Shioaji's ScannerType enum.
type ScannerType string

const (
	ScannerAmountRank ScannerType = "AmountRank"
	ScannerVolumeRank ScannerType = "VolumeRank"
)

// Scanner returns the top count ranked tickers for typ on date
// (YYYY-MM-DD). Always passes ascending:true — see Client's doc comment on
// why that (not false) is what returns rank-1-first.
func (c *Client) Scanner(ctx context.Context, typ ScannerType, date string, count int) ([]ScannerItem, error) {
	var out []ScannerItem
	req := map[string]any{"scanner_type": typ, "date": date, "ascending": true, "count": count}
	err := c.do(ctx, http.MethodPost, "/api/v1/data/scanner", req, &out)
	return out, err
}

// regulatoryColumns is the parallel-arrays shape both regulatory endpoints
// return (see Client's doc comment) — Code always present, the rest
// endpoint-specific and left as raw JSON for the caller to pick apart.
type regulatoryColumns struct {
	Code      []string `json:"code"`
	StartDate []string `json:"start_date"`
	EndDate   []string `json:"end_date"`
	Reason    []string `json:"reason"`
}

// RegulatoryPunish returns the codes currently under TWSE/TPEx disposition
// trading (處置股), each with its effective date range.
func (c *Client) RegulatoryPunish(ctx context.Context) (map[string]string, error) {
	var cols regulatoryColumns
	if err := c.do(ctx, http.MethodGet, "/api/v1/data/regulatory_punish", nil, &cols); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(cols.Code))
	for i, code := range cols.Code {
		reason := "處置"
		if i < len(cols.EndDate) {
			reason += "（至 " + cols.EndDate[i] + "）"
		}
		out[code] = reason
	}
	return out, nil
}

// RegulatoryNotice returns the codes currently flagged as attention stocks
// (注意股) with TWSE/TPEx's stated reason.
func (c *Client) RegulatoryNotice(ctx context.Context) (map[string]string, error) {
	var cols regulatoryColumns
	if err := c.do(ctx, http.MethodGet, "/api/v1/data/regulatory_notice", nil, &cols); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(cols.Code))
	for i, code := range cols.Code {
		if i < len(cols.Reason) {
			out[code] = cols.Reason[i]
		} else {
			out[code] = "注意"
		}
	}
	return out, nil
}

// AccountBalance is the account's cash balance as of Date, matching
// Shioaji's api_v1.portf.account_balance.AccountBalance exactly.
type AccountBalance struct {
	AccBalance float64 `json:"acc_balance"`
	Date       string  `json:"date"`
	Errmsg     string  `json:"errmsg"`
}

func (c *Client) AccountBalance(ctx context.Context) (AccountBalance, error) {
	var out AccountBalance
	err := c.do(ctx, http.MethodPost, "/api/v1/portfolio/account_balance", map[string]any{}, &out)
	return out, err
}

// Position is one currently-open stock holding, matching Shioaji's
// api_v1.portf.positions.StockPosition. ID feeds PositionDetail.
type Position struct {
	ID        int     `json:"id"`
	Code      string  `json:"code"`
	Direction string  `json:"direction"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

// PositionUnit lists all currently-open stock positions, in individual
// shares (unit=Share) rather than board lots (unit=Common, the default) —
// Phase 3's Trade reconstruction needs exact share counts, not 張.
func (c *Client) PositionUnit(ctx context.Context) ([]Position, error) {
	var out []Position
	err := c.do(ctx, http.MethodPost, "/api/v1/portfolio/position_unit", map[string]any{"unit": "Share"}, &out)
	return out, err
}

// PositionDetail is one still-open BUY lot within a position, matching
// Shioaji's api_v1.portf.position_detail.StockPositionDetail. Dseq is the
// brokerage's own per-deal sequence number, used verbatim as an
// idempotency key (see internal/sinopac/trades.go). Price's per-share-vs-
// total-amount semantics are unconfirmed against a real statement — see
// that package's doc comment.
type PositionDetail struct {
	Date      string  `json:"date"`
	Code      string  `json:"code"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
	Dseq      string  `json:"dseq"`
	Direction string  `json:"direction"`
}

// PositionDetail returns the lot-level breakdown of the position identified
// by detailID (a Position.ID from PositionUnit).
func (c *Client) PositionDetail(ctx context.Context, detailID int) ([]PositionDetail, error) {
	var out []PositionDetail
	err := c.do(ctx, http.MethodPost, "/api/v1/portfolio/position_detail", map[string]any{"detail_id": detailID}, &out)
	return out, err
}

// ProfitLoss is one realized SELL, matching Shioaji's
// api_v1.portf.profit_loss.StockProfitLoss. Unlike PositionDetail, it
// already carries everything Phase 3 needs (quantity/price/date/dseq) —
// no separate profit_loss_detail call required.
type ProfitLoss struct {
	Code     string  `json:"code"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
	PnL      float64 `json:"pnl"`
	Date     string  `json:"date"`
	Dseq     string  `json:"dseq"`
}

// ProfitLoss returns realized sells between begin and end (both YYYY-MM-DD,
// inclusive) — the daemon's default (omitted dates) always returns empty,
// so callers must always pass both.
func (c *Client) ProfitLoss(ctx context.Context, begin, end string) ([]ProfitLoss, error) {
	var out []ProfitLoss
	req := map[string]any{"begin_date": begin, "end_date": end, "unit": "Share"}
	err := c.do(ctx, http.MethodPost, "/api/v1/portfolio/profit_loss", req, &out)
	return out, err
}
