package llm

import (
	"fmt"
	"strings"

	"argus/internal/data"
	"argus/internal/i18n"
)

// PriceEventFacts is one price event's fact sheet as ExplainPriceEvent sees
// it — the mirror of signals.PriceEvent, redeclared here for the same reason
// StrategyHitInfo is (this package stays independent of internal/signals).
// All three percentages carry the day's real value whenever it could be
// computed, and the *Triggered flags say which threshold actually fired; see
// signals.PriceEvent for why the two are separate.
type PriceEventFacts struct {
	Ticker              string
	GapPct              float64
	ChangePct           float64
	CumulativePct       float64
	GapTriggered        bool
	ChangeTriggered     bool
	CumulativeTriggered bool

	// ATRMultiple is the day's move as a multiple of ATR(14) and VolumeRatio
	// its volume against the 20-day average — the "how unusual is this
	// really" scale a bare percentage can't carry, since 7% is not the same
	// event for NVDA as for KO. 0 means "not enough history to say", and the
	// line is then left out entirely. Thresholds are untouched by this: the
	// scale is context for the writeup, never a gate.
	ATRMultiple float64
	VolumeRatio float64
}

// buildPriceEventPrompt is Phase 20's gap/big-move event-summary prompt (see
// docs/phase-20-price-event-log.md §4.3): the day's gap/change/cumulative
// numbers plus ticker-only news (via writeNewsItem, same convention as
// buildExplorePrompt) — no market-wide news, per §2's "LLM 素材" decision.
//
// Every number that could be computed is rendered, with the threshold that
// actually fired marked (Phase 20 後續 PR3) — a summary asked to state facts
// was previously handed only the triggering one, which made "gapped -6%,
// closed down 1%" indistinguishable from "gapped -6%, closed -8%". The
// day's change is the one line rendered unconditionally: a flat close after
// a violent gap is exactly the case worth reading, so a 0.0% there must not
// be mistaken for missing data.
func buildPriceEventPrompt(lang i18n.Lang, ev PriceEventFacts, news []data.NewsItem) string {
	trigger := func(fired bool) string {
		if !fired {
			return ""
		}
		return i18n.T(lang, i18n.KeyPriceEventTriggerSuffix)
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(lang, i18n.KeyPriceEventPromptIntro, ev.Ticker))

	if ev.GapPct != 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyPriceEventGapLine, ev.GapPct, trigger(ev.GapTriggered)))
	}
	fmt.Fprint(&sb, i18n.T(lang, i18n.KeyPriceEventChangeLine, ev.ChangePct, trigger(ev.ChangeTriggered)))
	if ev.CumulativePct != 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyPriceEventCumulativeLine, ev.CumulativePct, trigger(ev.CumulativeTriggered)))
	}
	if ev.ATRMultiple > 0 && ev.VolumeRatio > 0 {
		fmt.Fprint(&sb, i18n.T(lang, i18n.KeyPriceEventScaleLine, ev.ATRMultiple, ev.VolumeRatio))
	}

	if len(news) > 0 {
		sb.WriteString(i18n.T(lang, i18n.KeyNewsHeader))
		for i, n := range news {
			writeNewsItem(&sb, lang, i+1, n)
		}
	}

	sb.WriteString(i18n.T(lang, i18n.KeyPriceEventPromptTask))
	return sb.String()
}
