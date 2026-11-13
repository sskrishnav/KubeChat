// Package redact removes secrets from tool output and caps its size before
// it reaches the LLM or Slack. Basic patterns only; not a guarantee.
package redact

import (
	"fmt"
	"regexp"
	"strings"
)

var rules = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), "[REDACTED PRIVATE KEY]"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`), "[REDACTED JWT]"},
	{regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`), "${1}[REDACTED]"},
	{regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{16}\b`), "[REDACTED AWS KEY]"},
	{regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api[_-]?key)(\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`), "${1}${2}[REDACTED]"},
}

// Clean redacts secrets, then cuts the text to at most max bytes (0 = no cap)
// and says so when it did.
func Clean(text string, max int) string {
	for _, r := range rules {
		text = r.re.ReplaceAllString(text, r.with)
	}
	if max > 0 && len(text) > max {
		total := len(text)
		text = strings.ToValidUTF8(text[:max], "") // don't leave half a character
		text += fmt.Sprintf("\n[output cut: first %d of %d bytes shown]", max, total)
	}
	return text
}
