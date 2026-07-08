package llm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AntigravityProvider drives Google's Antigravity CLI (`agy`) as a fallback
// LLM backend, authenticated via the operator's Google OAuth login
// (Antigravity subscription) rather than a metered Gemini API key — same
// non-billing principle as acpProvider. Wired in via Client.AddFallback,
// constructed by main.go only when ANTIGRAVITY_ENABLED is set — it's opt-in
// rather than always-on because of the tool-safety tradeoff below.
//
// Every call runs with --sandbox. agy's non-interactive `-p` mode
// auto-approves every tool call it decides to make — including write_file —
// with no working read-only/plan-mode equivalent (confirmed against
// google-antigravity/antigravity-cli issue #45; even the CLI's own `strict`
// permission preset isn't reliably honored in `-p` runs). --sandbox does not
// stop the model from calling tools, it only contains *where* those calls
// execute, so a stray write/exec lands in a throwaway container instead of
// the real VPS filesystem — see PLAN.md's architecture-debt entry for why
// this is a deliberate, accepted risk rather than an oversight. This also
// means the VPS needs a working sandbox/container runtime available to
// `agy`, a new operational dependency the bare-metal systemd deployment
// doesn't otherwise have.
//
// Two more things are unverified against a real `agy` install (there's no
// safe way to test them without one) and may need adjusting once this runs
// against the live CLI:
//   - `-p` has a reported bug where stdout goes missing under a non-TTY pipe
//     (antigravity-cli#76) — exactly how os/exec invokes it. Left as-is
//     rather than pre-emptively wrapped in a pty-faking workaround (e.g.
//     `script`), since that workaround is itself unverified and would need
//     shell-quoting the prompt text to avoid command injection; if this bug
//     bites in practice, ANTIGRAVITY_CLI_COMMAND can point at a wrapper
//     script instead of changing this code.
//   - `-p` has no session id it reliably surfaces to resume a conversation
//     (antigravity-cli#7), which is why antigravityChatSession replays the
//     whole transcript every turn instead of trying to resume a backing
//     session the way acpChatSession does.
type AntigravityProvider struct{}

func (AntigravityProvider) Prompt(ctx context.Context, systemPrompt, model, text string) (string, error) {
	prompt := text
	if systemPrompt != "" {
		prompt = systemPrompt + "\n\n" + text
	}
	return runAntigravity(ctx, model, prompt)
}

func (AntigravityProvider) NewChatSession(ctx context.Context, systemPrompt, model string) (ChatSession, error) {
	return &antigravityChatSession{systemPrompt: systemPrompt, model: model}, nil
}

// antigravityChatSession replays the whole conversation transcript on every
// turn instead of resuming a backing session (see AntigravityProvider's doc
// comment for why) — conversation memory lives here in Go, not in an agy
// process the way it does for acpChatSession.
type antigravityChatSession struct {
	systemPrompt string
	model        string
	turns        []string // alternating "User: "/"Assistant: " lines, oldest first
}

func (s *antigravityChatSession) Send(ctx context.Context, text string) (string, error) {
	var sb strings.Builder
	if s.systemPrompt != "" {
		sb.WriteString(s.systemPrompt)
		sb.WriteString("\n\n")
	}
	for _, t := range s.turns {
		sb.WriteString(t)
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "User: %s\n", text)
	sb.WriteString("Assistant:")

	reply, err := runAntigravity(ctx, s.model, sb.String())
	if err != nil {
		return "", err
	}
	s.turns = append(s.turns, "User: "+text, "Assistant: "+reply)
	return reply, nil
}

// Close is a no-op: there's no backing process or remote session to tear
// down — history lives in the antigravityChatSession value itself.
func (s *antigravityChatSession) Close() error {
	return nil
}

// runAntigravity shells out to `agy -p` for a single non-interactive turn.
func runAntigravity(ctx context.Context, model, prompt string) (string, error) {
	args := []string{"-p", prompt, "--sandbox"}
	if model != "" {
		args = append(args, "--model", model)
	}

	cmd := exec.CommandContext(ctx, agyCommand(), args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("agy: %w", err)
	}
	reply := strings.TrimSpace(string(out))
	if reply == "" {
		// Could be a genuinely empty reply, or the known non-TTY stdout bug
		// (see AntigravityProvider's doc comment) — treat both as an error
		// rather than silently returning blank text to the user.
		return "", fmt.Errorf("agy: empty response")
	}
	return reply, nil
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
