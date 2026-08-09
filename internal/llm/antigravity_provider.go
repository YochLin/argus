package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// AntigravityProvider drives Google's Antigravity CLI (`agy`) as a fallback
// LLM backend, authenticated via the operator's Google OAuth login
// (Antigravity subscription) rather than a metered Gemini API key — same
// non-billing principle as acpProvider. Wired in via Client.AddFallback,
// constructed by main.go only when ANTIGRAVITY_ENABLED is set — it's opt-in
// rather than always-on because of the tool-safety tradeoff below.
//
// Calls do NOT run with --sandbox, and deliberately not with
// --dangerously-skip-permissions either. Verified against a live agy install:
// headless `-p` mode auto-*denies* any tool call needing permission rather
// than auto-approving it ("a tool required the \"write_file\" permission that
// headless mode cannot prompt for, so it was auto-denied"), with or without
// --mode plan. An earlier version of this file assumed the opposite and paid
// for --sandbox's container-runtime dependency on the VPS to contain writes
// that headless mode was never going to allow in the first place.
//
// Every call uses --output-format json: it's the only way to get the
// conversation id back (needed by runAntigravity's chunked feed and by
// antigravityChatSession), and it separates the reply from any stray
// diagnostic lines agy writes alongside it.
type AntigravityProvider struct{}

func (AntigravityProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	prompt := text
	if systemPrompt != "" {
		prompt = systemPrompt + "\n\n" + text
	}
	reply, _, err := runAntigravity(ctx, model, "", prompt)
	return reply, err
}

func (AntigravityProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (ChatSession, error) {
	return &antigravityChatSession{systemPrompt: systemPrompt, model: model}, nil
}

// antigravityChatSession keeps its history in an agy-side conversation
// (resumed by id via --conversation) rather than replaying the whole
// transcript from Go on every turn. convID is empty until the first turn
// returns one; a turn that fails leaves it untouched, so the next Send
// retries against the same conversation rather than silently starting a new
// one mid-chat.
type antigravityChatSession struct {
	systemPrompt string
	model        string
	convID       string
}

func (s *antigravityChatSession) Send(ctx context.Context, text string) (string, error) {
	// The system prompt rides along with the first turn only — later turns
	// resume a conversation that already has it.
	if s.convID == "" && s.systemPrompt != "" {
		text = s.systemPrompt + "\n\n" + text
	}
	reply, convID, err := runAntigravity(ctx, s.model, s.convID, text)
	if err != nil {
		return "", err
	}
	s.convID = convID
	return reply, nil
}

// Close is a no-op: there's no local process to tear down, and agy's
// conversations persist on its own side with no explicit end call.
func (s *antigravityChatSession) Close() error {
	return nil
}

// agyMaxPromptArg caps how much prompt text goes into a single `agy -p <text>`
// argument. Linux caps one argv entry at MAX_ARG_STRLEN = 128KiB — a kernel
// constant, unrelated to (and far below) ARG_MAX — so an oversized prompt
// fails at exec time with "argument list too long", never reaching agy at
// all. A realistic /recommend prompt already runs ~107KB at 25 watchlist +
// 15 candidate tickers, so this is a live wall on the VPS, not a theoretical
// one. macOS has no per-argument cap, which is why this only bites in
// production.
//
// The remaining ~48KiB of headroom covers the per-chunk framing text below
// plus the other args.
const agyMaxPromptArg = 80 * 1024

// runAntigravity runs prompt through agy and returns the reply plus the
// conversation id it ran in. resumeID continues an existing conversation
// (empty starts a new one).
//
// A prompt too big for one argv entry is fed as several turns of one
// conversation instead: each turn's text lands in agy's context in full, and
// the last turn answers with everything before it still in context. The
// alternative — dumping the prompt to a file and letting agy read it — was
// measured and rejected: agy greps/skims a large file rather than ingesting
// it (a 287KB file came back at 23K input tokens), which for a multi-ticker
// analysis silently drops most of the input instead of erroring. Chunked
// turns keep the full text in context, confirmed by input_tokens climbing to
// the whole prompt's worth by the final turn.
func runAntigravity(ctx context.Context, model, resumeID, prompt string) (reply string, convID string, err error) {
	chunks := splitPrompt(prompt, agyMaxPromptArg)
	convID = resumeID
	for i, chunk := range chunks {
		text := chunk
		last := i == len(chunks)-1
		if len(chunks) > 1 {
			// Framing text is intentionally not in internal/i18n: it's
			// transport plumbing that never reaches the user (only the final
			// turn's reply is returned), and the model follows it regardless
			// of the language the surrounding prompt is written in.
			if last {
				text = fmt.Sprintf("[input part %d/%d — final part]\n%s\n\n[end of input] All parts have now been sent. Answer the instructions given at the very beginning of part 1, using every part.", i+1, len(chunks), chunk)
			} else {
				text = fmt.Sprintf("[input part %d/%d] This is a partial input. Do not answer yet, do not analyze, reply with exactly ACK.\n%s", i+1, len(chunks), chunk)
			}
		}
		res, err := runAgyOnce(ctx, model, convID, text)
		if err != nil {
			return "", "", err
		}
		convID = res.ConversationID
		if last {
			if res.Response == "" {
				// An empty final reply means the run produced nothing usable
				// (e.g. a tool call was auto-denied, see AntigravityProvider)
				// — an error, not blank text handed to the user.
				return "", "", fmt.Errorf("agy: empty response (status %q)", res.Status)
			}
			return strings.TrimSpace(res.Response), convID, nil
		}
	}
	return "", "", fmt.Errorf("agy: empty prompt")
}

// agyResult is the subset of `agy --output-format json`'s result object this
// package uses.
type agyResult struct {
	ConversationID string `json:"conversation_id"`
	Status         string `json:"status"`
	Response       string `json:"response"`
}

// runAgyOnce shells out to `agy -p` for a single non-interactive turn.
func runAgyOnce(ctx context.Context, model, resumeID, text string) (agyResult, error) {
	args := []string{"-p", text, "--output-format", "json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if resumeID != "" {
		args = append(args, "--conversation", resumeID)
	}

	out, err := exec.CommandContext(ctx, agyCommand(), args...).Output()
	if err != nil {
		return agyResult{}, fmt.Errorf("agy: %w", err)
	}
	// agy can precede the result object with plain diagnostic lines, so take
	// the last line that parses as JSON rather than the whole of stdout.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var res agyResult
		if json.Unmarshal([]byte(lines[i]), &res) == nil && res.ConversationID != "" {
			return res, nil
		}
	}
	return agyResult{}, fmt.Errorf("agy: no JSON result in output")
}

// splitPrompt breaks s into chunks of at most limit bytes, cutting at line
// boundaries so a chunk never ends mid-row of a data table. A single line
// longer than limit is hard-split at the nearest rune boundary below the
// limit — content is never dropped, and never cut through a multi-byte rune.
func splitPrompt(s string, limit int) []string {
	if len(s) <= limit {
		return []string{s}
	}
	var chunks []string
	for len(s) > limit {
		cut := strings.LastIndexByte(s[:limit], '\n') + 1 // keep the newline
		if cut <= 0 {
			cut = limit
			for cut > 0 && !utf8.RuneStart(s[cut]) {
				cut--
			}
		}
		chunks = append(chunks, s[:cut])
		s = s[cut:]
	}
	if s != "" {
		chunks = append(chunks, s)
	}
	return chunks
}

// agyCommand resolves how to launch the Antigravity CLI. Defaults to `agy`
// on PATH; set ANTIGRAVITY_CLI_COMMAND to point at a wrapper (e.g. one that
// works around a CLI quirk found in practice) without touching this file —
// same escape hatch CLAUDE_ACP_COMMAND provides for the acp path.
func agyCommand() string {
	if custom := os.Getenv("ANTIGRAVITY_CLI_COMMAND"); custom != "" {
		return custom
	}
	return "agy"
}
