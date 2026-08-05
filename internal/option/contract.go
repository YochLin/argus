// Package option holds pure functions for US equity options: OCC symbol
// parsing/formatting, Black-Scholes greeks, and mark-price selection. It has
// no dependency on internal/db or internal/bot — see docs/phase-12-options.md
// §4 PR1 for the full rationale (OCC symbols must never enter positions.ticker,
// see CLAUDE.md's internal/data section).
package option

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Right is a contract's option type.
type Right string

const (
	Call Right = "C"
	Put  Right = "P"
)

// Contract is an OCC symbol's decoded fields — the single source of truth
// derivation for option_positions' underlying/right/strike/expiry columns
// (see docs/phase-12-options.md §3.1: those columns are always derived by
// Parse at write time, never caller-supplied, same convention as the
// existing market column).
type Contract struct {
	Underlying string
	Right      Right
	Expiry     time.Time
	Strike     float64
}

// Parse decodes an OCC/OSI option symbol, e.g. "AAPL260805C00310000":
// underlying (1-6 letters) + expiry YYMMDD + right C|P + strike*1000
// zero-padded to 8 digits. Parsing anchors from the right (date+right+strike
// is always exactly 15 characters) rather than assuming a fixed-width,
// space-padded underlying — live data (Yahoo's contractSymbol, what /obuy
// takes verbatim) has no such padding, so anchoring from the right handles
// both the padded textbook OCC spec and the compact real-world form.
func Parse(occ string) (Contract, error) {
	s := strings.ToUpper(strings.TrimSpace(occ))
	const suffixLen = 6 /* date */ + 1 /* right */ + 8 /* strike */
	if len(s) <= suffixLen {
		return Contract{}, fmt.Errorf("option: invalid OCC symbol %q", occ)
	}

	underlying := strings.TrimSpace(s[:len(s)-suffixLen])
	datePart := s[len(s)-suffixLen : len(s)-suffixLen+6]
	rightPart := Right(s[len(s)-9 : len(s)-8])
	strikePart := s[len(s)-8:]

	if underlying == "" || len(underlying) > 6 || !isAlpha(underlying) {
		return Contract{}, fmt.Errorf("option: invalid underlying in %q", occ)
	}
	if rightPart != Call && rightPart != Put {
		return Contract{}, fmt.Errorf("option: invalid right in %q", occ)
	}
	expiry, err := time.Parse("060102", datePart)
	if err != nil {
		return Contract{}, fmt.Errorf("option: invalid expiry in %q: %w", occ, err)
	}
	strikeMilli, err := strconv.ParseInt(strikePart, 10, 64)
	if err != nil {
		return Contract{}, fmt.Errorf("option: invalid strike in %q: %w", occ, err)
	}

	return Contract{
		Underlying: underlying,
		Right:      rightPart,
		Expiry:     expiry,
		Strike:     float64(strikeMilli) / 1000,
	}, nil
}

// Format builds the compact OCC/OSI symbol Parse round-trips — no space
// padding on the underlying, matching Yahoo's actual contractSymbol (the
// textbook OCC spec pads the root to 6 characters, but no live data source
// this project talks to does).
func Format(underlying string, right Right, expiry time.Time, strike float64) string {
	strikeMilli := int64(math.Round(strike * 1000))
	return fmt.Sprintf("%s%s%s%08d", strings.ToUpper(underlying), expiry.Format("060102"), right, strikeMilli)
}

// IsOCC reports whether s parses as an OCC option symbol, letting callers
// tell an option contract apart from a plain stock ticker in one line.
func IsOCC(s string) bool {
	_, err := Parse(s)
	return err == nil
}

func isAlpha(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
