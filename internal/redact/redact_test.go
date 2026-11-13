package redact

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain text is unchanged", "fatal: missing DB_URL", "fatal: missing DB_URL"},
		{"password=", "connecting with password=hunter2 now", "connecting with password=[REDACTED] now"},
		{"token: in YAML", "token: abc123", "token: [REDACTED]"},
		{"quoted secret", `secret="s3 cr3t"`, `secret=[REDACTED]`},
		{"bearer token", "Authorization: Bearer abcdefgh12345678", "Authorization: Bearer [REDACTED]"},
		{"JWT", "jwt eyJhbGciOi.eyJzdWIiOi.SflKxwRJSM", "jwt [REDACTED JWT]"},
		{"AWS key", "key AKIAABCDEFGHIJKLMNOP used", "key [REDACTED AWS KEY] used"},
		{"private key", "a\n-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----\nb", "a\n[REDACTED PRIVATE KEY]\nb"},
		{"field names that only contain the word stay", "automountServiceAccountToken: true", "automountServiceAccountToken: true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clean(tt.in, 0); got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanCapsSize(t *testing.T) {
	got := Clean(strings.Repeat("x", 100), 10)
	if !strings.HasPrefix(got, "xxxxxxxxxx\n[output cut: first 10 of 100 bytes shown]") {
		t.Errorf("Clean() = %q", got)
	}
	if got := Clean("short", 10); got != "short" {
		t.Errorf("Clean() = %q, want unchanged", got)
	}
}
