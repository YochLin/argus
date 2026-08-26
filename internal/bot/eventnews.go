package bot

import (
	"time"

	"argus/internal/data"
	"argus/internal/llm"
	"argus/internal/signals"
)

// priceEventFacts converts a detected event into the fact sheet the LLM
// prompt takes. Two structs rather than one shared type is deliberate:
// internal/llm doesn't import internal/signals (see llm.StrategyHitInfo for
// the same split), so this three-line copy is what keeps that boundary.
func priceEventFacts(ev signals.PriceEvent) llm.PriceEventFacts {
	return llm.PriceEventFacts{
		Ticker:              ev.Ticker,
		GapPct:              ev.GapPct,
		ChangePct:           ev.ChangePct,
		CumulativePct:       ev.CumulativePct,
		GapTriggered:        ev.GapTriggered,
		ChangeTriggered:     ev.ChangeTriggered,
		CumulativeTriggered: ev.CumulativeTriggered,
	}
}

// eventNewsWindow is how far from the event's own trading date a news item
// may be published and still be offered to ExplainPriceEvent as a possible
// cause. One day either side, not a tighter window: a US event dated in
// Taiwan-yesterday terms has its session news spread across two CST
// calendar days (the US open is 21:30 CST, the close 04:00 CST the next
// morning), so anything narrower would throw away news from the very
// session being explained.
const eventNewsWindow = 24 * time.Hour

// filterNewsNearDate keeps only the news published within eventNewsWindow of
// date (a CST "2006-01-02" trading date), dropping anything older — Phase 20
// 後續 PR2's fix for the summaries' main quality problem. The feed is
// correctly ordered newest-first, but a ticker's top-5 routinely contained
// 4–7-day-old stories, and the prompt asks the model to infer a cause from
// whatever news it's given, so it dutifully blamed last week's headline for
// today's gap. Filtering to nothing is the intended outcome for a move with
// no news behind it: the prompt already has a "no related news, cause
// unknown" branch that until now was almost never reached.
//
// Items with no publish timestamp are dropped rather than kept — an undated
// item can't be shown to be recent, and this is the one place where being
// wrong about recency is the whole failure mode. An unparseable date leaves
// news untouched (a filter bug must not silently starve the prompt).
func filterNewsNearDate(news []data.NewsItem, date string) []data.NewsItem {
	day, err := time.ParseInLocation("2006-01-02", date, cst)
	if err != nil {
		return news
	}
	var out []data.NewsItem
	for _, n := range news {
		if n.PublishedAt.IsZero() {
			continue
		}
		p := n.PublishedAt.In(cst)
		published := time.Date(p.Year(), p.Month(), p.Day(), 0, 0, 0, 0, cst)
		if diff := published.Sub(day); diff <= eventNewsWindow && diff >= -eventNewsWindow {
			out = append(out, n)
		}
	}
	return out
}
