package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"argus/internal/i18n"
	"argus/internal/logger"
)

// WealthViewSummaryMarker starts the summary line of the AI view's reply.
// Same in both languages, injected into the task text so prompt and parser
// can't drift (like KeyMarketSummaryMarker).
const WealthViewSummaryMarker = "[SUMMARY]"

// WealthViewInput is what the monthly AI view starts from: the health ratios
// (service.ComputeHealthMetrics), the retirement goal's progress, and the
// titles of the rule-based checks already on the home page, so the model
// writes about something else. Every ratio is a pointer; nil leaves its line
// out rather than asking the model to read a dash. The model reads the records
// themselves through the `argus mcp wealth` tools.
type WealthViewInput struct {
	Month                 string
	DebtRatioPct          *float64
	SavingsRatePct        *float64
	ExpenseRatioPct       *float64
	LiquidityMonths       *float64
	RetirementGoalName    string
	RetirementProgressPct *float64
	RetirementSaved       float64
	RetirementTarget      float64
	Checks                []string
}

func buildWealthViewPrompt(lang i18n.Lang, in WealthViewInput) string {
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptIntro, in.Month))

	if in.DebtRatioPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptDebtRatioLine, *in.DebtRatioPct))
	}
	if in.LiquidityMonths != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptLiquidityLine, *in.LiquidityMonths))
	}
	if in.SavingsRatePct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptSavingsRateLine, *in.SavingsRatePct))
	}
	if in.ExpenseRatioPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptExpenseRatioLine, *in.ExpenseRatioPct))
	}
	if in.RetirementProgressPct != nil {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptRetirementLine, in.RetirementGoalName, *in.RetirementProgressPct, in.RetirementSaved, in.RetirementTarget))
	}
	if len(in.Checks) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyWealthViewPromptChecksHeader))
		for _, c := range in.Checks {
			sb.WriteString("- " + c + "\n")
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyWealthHealthPromptTask, WealthViewSummaryMarker))
	return sb.String()
}

// parseWealthView splits the reply into its summary line and its points. A
// reply with no points is a failed call, not an empty view.
func parseWealthView(reply string) (summary string, points []string, err error) {
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, WealthViewSummaryMarker):
			summary = strings.TrimSpace(strings.TrimPrefix(line, WealthViewSummaryMarker))
		case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "• "):
			if p := strings.TrimSpace(line[strings.Index(line, " ")+1:]); p != "" {
				points = append(points, p)
			}
		}
	}
	if len(points) == 0 {
		return "", nil, errors.New("no points in reply")
	}
	return summary, points, nil
}

// WealthView is the home page's monthly "AI 觀點": the model's own
// observations beyond the rule-based checks, from the records it reads
// through the wealth MCP tools. It runs on a one-off session, never the
// assistant's, so it doesn't land in the user's conversation. The task text
// tells it not to propose drafts and to name only pages that exist.
func (c *Client) WealthView(ctx context.Context, in WealthViewInput) (summary string, points []string, err error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	systemPrompt := i18n.T(c.lang, i18n.KeySystemPromptWealthView)
	prompt := buildWealthViewPrompt(c.lang, in)
	var lastErr error
	for _, b := range c.backends {
		for attempt := 1; attempt <= maxAttemptsPerBackend; attempt++ {
			session, err := b.provider.NewChatSession(ctx, systemPrompt, b.chatModel, MCPWealth)
			if err != nil {
				lastErr = err
			} else {
				reply, sendErr := session.Send(ctx, prompt)
				session.Close()
				if sendErr != nil {
					lastErr = sendErr
				} else if summary, points, err = parseWealthView(reply); err == nil {
					return summary, points, nil
				} else {
					lastErr = fmt.Errorf("%w: %s", err, truncateForLog(reply))
				}
			}
			logger.Errorf("llm: backend %T attempt %d/%d wealth view failed: %v", b.provider, attempt, maxAttemptsPerBackend, lastErr)
			if ctx.Err() != nil {
				return "", nil, lastErr
			}
		}
	}
	return "", nil, lastErr
}
