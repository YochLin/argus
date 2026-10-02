package bot

import (
	"context"
	"time"

	"argus/internal/data"
	"argus/internal/db"
	"argus/internal/logger"
	"argus/internal/service"
)

// fillNewsTimeout bounds the LLM half of a capture; the fetch and the save
// are quick, and nothing waits on this goroutine.
const fillNewsTimeout = 5 * time.Minute

// captureFillNews is Phase 27 P7's news snapshot: when a trade is recorded, it
// keeps the headlines from around the fill date (title, source, link, time)
// and has the LLM label each one (sentiment / kind / major event). GetNews has
// no date parameter, so a past day's news can't be fetched later — this is the
// only chance to keep it, and why it runs on every recorded fill rather than
// on demand. A fill dated days ago (CSV import, a late broker sync) finds
// nothing inside the ±EventNewsWindow date filter and stores nothing, which
// is the intended "no news snapshot" outcome.
//
// It follows the price-event path (fetch → FilterNewsNearDate → NewsPicker),
// and is best-effort like it: a news or LLM failure is logged, never surfaced
// to the trade. Headlines are saved before the LLM runs, so an LLM failure
// still leaves them readable, and the next fill of that ticker/day labels
// whatever is still unlabelled.
func (b *Bot) captureFillNews(ticker, date string) {
	defer b.recoverJobPanic("fill news")

	if !b.storeFillNews(ticker, date) {
		return
	}

	stored, err := b.db.FillNewsFor(ticker, date)
	if err != nil {
		logger.Errorf("fill news %s: reload: %v", ticker, err)
		return
	}
	var todo []db.FillNews
	for _, r := range stored {
		if r.Sentiment == "" {
			todo = append(todo, r)
		}
	}
	if len(todo) == 0 || b.llm == nil {
		return
	}

	items := make([]data.NewsItem, len(todo))
	for i, r := range todo {
		items[i] = data.NewsItem{Headline: r.Headline, Source: r.Source, PublishedAt: r.PublishedAt}
	}
	ctx, cancel := context.WithTimeout(context.Background(), fillNewsTimeout)
	defer cancel()
	labels, err := b.llm.ClassifyNews(ctx, ticker, items)
	if err != nil {
		logger.Errorf("fill news %s: classify: %v", ticker, err)
		return
	}
	for i, l := range labels {
		if l.Sentiment == "" {
			continue
		}
		if err := b.db.SetFillNewsLabel(todo[i].ID, l.Sentiment, l.Tag, l.Major); err != nil {
			logger.Errorf("fill news %s: label: %v", ticker, err)
		}
	}
}

// storeFillNews is captureFillNews' first half: fetch the headlines around date
// and keep them, unlabelled. It is idempotent (a headline already stored for
// that ticker/day is skipped), so a trade review can call it to be sure a
// same-day fill's news is in before reading it, without waiting for the
// capture goroutine that is still busy with the LLM. ok is false when there is
// nothing to do or the save failed.
func (b *Bot) storeFillNews(ticker, date string) (ok bool) {
	// FilterNewsNearDate passes everything through on an unparseable date, which
	// would file the latest headlines under a fill they have nothing to do with.
	if _, err := time.Parse("2006-01-02", date); err != nil || b.provider == nil {
		return false
	}

	news, err := b.provider.GetNews(ticker, service.EventNewsFetch)
	if err != nil {
		logger.Errorf("fill news %s: fetch: %v", ticker, err)
	} else if len(news) == 0 {
		// Yahoo's search sometimes answers an empty list with no error; without
		// this line a fill with no stored headlines can't be told from one that
		// had none to find.
		logger.Warnf("fill news %s %s: provider returned no headlines", ticker, date)
	}
	fetched := len(news)
	news = (&service.NewsPicker{}).Pick(service.FilterNewsNearDate(news, date), service.EventNewsSlots)
	if fetched > 0 && len(news) == 0 {
		logger.Infof("fill news %s %s: %d headlines fetched, none within the date window", ticker, date, fetched)
	}

	rows := make([]db.FillNews, len(news))
	for i, n := range news {
		rows[i] = db.FillNews{Headline: n.Headline, Source: n.Source, URL: n.URL, PublishedAt: n.PublishedAt}
	}
	if err := b.db.SaveFillNews(ticker, date, rows); err != nil {
		logger.Errorf("fill news %s: save: %v", ticker, err)
		return false
	}
	return true
}
