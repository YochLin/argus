// Package webfetch fetches a web page server-side and extracts its readable
// text, for the chat "article digestion" mode (Phase 3): the user pastes a
// URL into chat, and the extracted text gets fed to the LLM as plain text.
// This lives outside internal/bot so ExtractURL/Fetch stay independently
// testable, and outside internal/data since it isn't a stock-quote data
// source. Fetching happens here rather than via an ACP tool call because
// every chat session disables the agent's built-in tools (see
// internal/llm's acp_provider.go) — the agent never gets network access of
// its own.
package webfetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// urlRe matches a bare http(s) URL inside a larger message. It stops at
// whitespace or the quoting characters someone might wrap a link in.
var urlRe = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// trailingPunct is stripped off a matched URL — punctuation that closes a
// surrounding sentence (Chinese and Western) rather than being part of the
// URL itself, e.g. "看看這篇 https://x.com/a。" or "(https://x.com/a)".
const trailingPunct = ".,;:!?)]}、。，！？」』"

// maxBodyBytes caps how much of the response body Fetch reads, so a huge or
// misbehaving page can't blow up memory or take forever to download.
const maxBodyBytes = 5 << 20 // 5MB

// maxArticleRunes caps the extracted text handed to the LLM. Claude via ACP
// has no metered per-token cost (see internal/llm), but an unbounded page
// still means an unbounded prompt and a slower reply.
const maxArticleRunes = 20000

// skipTags holds elements whose entire subtree carries no readable article
// text (scripts, styles, nav chrome, embedded media) and should be skipped
// during extraction.
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true,
	"nav": true, "header": true, "footer": true, "aside": true,
	"form": true, "button": true, "select": true, "svg": true, "iframe": true,
}

// blockTags holds elements that end a block of text — a paragraph break is
// inserted after each one closes, so extracted text keeps some structure
// instead of running every sentence together.
var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "br": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true,
}

// Article is a fetched page's title and extracted body text.
type Article struct {
	Title string
	Text  string
}

// ExtractURL returns the first http(s) URL found in text, or ok=false if
// there isn't one. Pure — no network access.
func ExtractURL(text string) (rawURL string, ok bool) {
	m := urlRe.FindString(text)
	if m == "" {
		return "", false
	}
	m = strings.TrimRight(m, trailingPunct)
	if m == "" {
		return "", false
	}
	return m, true
}

// Fetch downloads rawURL and extracts its title and readable text. It
// returns an error for anything that keeps the page from being digestible —
// a non-2xx status, a non-HTML content type, or a page whose extracted text
// is too short to be a real article (the common paywall/JS-rendered-page
// signature, since the initial HTML in those cases is mostly chrome with no
// body copy) — callers should treat any error as "couldn't read this page"
// and degrade gracefully rather than surfacing it raw.
func Fetch(ctx context.Context, rawURL string) (Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Article{}, fmt.Errorf("webfetch: bad url: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Article{}, fmt.Errorf("webfetch: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Article{}, fmt.Errorf("webfetch: http status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
		return Article{}, fmt.Errorf("webfetch: unsupported content type %q", ct)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Article{}, fmt.Errorf("webfetch: read body: %w", err)
	}

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return Article{}, fmt.Errorf("webfetch: parse html: %w", err)
	}

	title, text := extractText(doc)
	text = strings.TrimSpace(text)
	if len([]rune(text)) < 200 {
		return Article{}, fmt.Errorf("webfetch: extracted content too short (%d chars) — likely a paywall or JS-rendered page", len([]rune(text)))
	}

	if r := []rune(text); len(r) > maxArticleRunes {
		text = string(r[:maxArticleRunes]) + "...[truncated]"
	}

	return Article{Title: strings.TrimSpace(title), Text: text}, nil
}

// extractText walks the parsed document collecting title text and readable
// body text, skipping the subtree of any skipTags element.
func extractText(doc *html.Node) (title, text string) {
	var titleBuf, textBuf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[strings.ToLower(n.Data)] {
			return
		}
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "title" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					titleBuf.WriteString(c.Data)
				}
			}
		}
		if n.Type == html.TextNode {
			s := strings.TrimSpace(n.Data)
			if s != "" {
				textBuf.WriteString(s)
				textBuf.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[strings.ToLower(n.Data)] {
			textBuf.WriteString("\n")
		}
	}
	walk(doc)
	return titleBuf.String(), textBuf.String()
}
