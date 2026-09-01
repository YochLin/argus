package bot

import (
	"context"
	"strings"

	"argus/internal/db"
	"argus/internal/i18n"
	"argus/internal/logger"
	"argus/internal/webfetch"
)

// handlePodcast backs /podcast <url>: the user pastes a link to a stock-
// market podcast/video transcript (e.g. a vocus.cc 股癌 episode), the page
// is fetched and its text extracted via internal/webfetch (same fetch path
// as the chat "article digestion" mode — see handleChatArticle), then run
// through a dedicated one-shot LLM call (llm.ExtractPodcastInsights) that
// pulls out per-stock and macro market views while ignoring sponsor reads
// and unrelated chatter. Each extracted view is persisted via
// db.SavePodcastInsight — a separate, structured-output command rather than
// reusing handleChatArticle's free-form chat reply, since the point here is
// building up a queryable log of past views (see the /podcast design
// discussion), not a one-off conversational summary. Re-submitting the same
// URL overwrites rather than accumulates: CountPodcastInsightsByURL warns
// the user up front, and once a fresh, non-empty extraction is in hand,
// DeletePodcastInsightsByURL clears the URL's old rows before the new ones
// are saved — a run that comes back empty leaves prior insights untouched,
// since an empty reply is more likely extraction flakiness than "there's
// genuinely nothing here anymore".
func (b *Bot) handlePodcast(ctx context.Context, args string) {
	url, ok := webfetch.ExtractURL(args)
	if !ok {
		b.Send(i18n.T(b.lang, i18n.KeyPodcastUsage))
		return
	}

	existing, err := b.db.CountPodcastInsightsByURL(url)
	if err != nil {
		logger.Errorf("podcast: count existing insights for %s: %v", url, err)
	} else if existing > 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPodcastDuplicateWarning, existing))
	}

	b.Send(i18n.T(b.lang, i18n.KeyPodcastFetching))
	article, err := webfetch.Fetch(ctx, url)
	if err != nil {
		logger.Errorf("podcast: fetch %s: %v", url, err)
		b.Send(i18n.T(b.lang, i18n.KeyPodcastFetchFailed, err))
		return
	}

	b.Send(i18n.T(b.lang, i18n.KeyPodcastAnalyzing))
	insights, err := b.llm.ExtractPodcastInsights(ctx, article.Title, url, article.Text)
	if err != nil {
		logger.Errorf("podcast: extract %s: %v", url, err)
		b.Send(i18n.T(b.lang, i18n.KeyPodcastAnalyzeFailed, err))
		return
	}
	if len(insights) == 0 {
		b.Send(i18n.T(b.lang, i18n.KeyPodcastNoInsights))
		return
	}

	if existing > 0 {
		if err := b.db.DeletePodcastInsightsByURL(url); err != nil {
			logger.Errorf("podcast: delete existing insights for %s: %v", url, err)
			b.Send(i18n.T(b.lang, i18n.KeyPodcastAnalyzeFailed, err))
			return
		}
	}

	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, i18n.KeyPodcastSavedHeader, len(insights)))
	for _, ins := range insights {
		if err := b.db.SavePodcastInsight(db.PodcastInsight{
			SourceURL:   url,
			SourceTitle: article.Title,
			Ticker:      ins.Ticker,
			Market:      ins.Market,
			Stance:      ins.Stance,
			Thesis:      ins.Thesis,
			DerivedFrom: ins.DerivedFrom,
		}); err != nil {
			logger.Errorf("podcast: save insight (%s/%s) for %s: %v", ins.Ticker, ins.Stance, url, err)
			continue
		}
		label := ins.Ticker
		if label == "" {
			label = i18n.T(b.lang, i18n.KeyPodcastMacroLabel)
		}
		sb.WriteString(i18n.T(b.lang, i18n.KeyPodcastSavedLine, label, ins.Stance, ins.Thesis))
		if ins.DerivedFrom != "" {
			sb.WriteString(i18n.T(b.lang, i18n.KeyPodcastDerivedLine, ins.DerivedFrom))
		}
	}
	b.Send(sb.String())
}
