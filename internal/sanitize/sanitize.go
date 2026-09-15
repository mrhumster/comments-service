package sanitize

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

const MaxBodyLength = 4000

// policy is a UGC-style allowlist. It strips scripts, event handlers and
// dangerous URL schemes from any raw HTML embedded in markdown source, while
// keeping safe formatting tags. Markdown syntax itself is untouched.
var policy = bluemonday.UGCPolicy()

// Body trims and sanitizes a comment body (markdown source). Returns an
// empty string when the result contains no text.
func Body(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) > MaxBodyLength {
		raw = raw[:MaxBodyLength]
	}
	cleaned := policy.Sanitize(raw)
	return strings.TrimSpace(cleaned)
}

// Snippet returns a plain-ish preview of the sanitized body (markdown
// markers included), limited to maxRunes, for activity feed payloads.
func Snippet(body string, maxRunes int) string {
	runes := []rune(body)
	if len(runes) <= maxRunes {
		return body
	}
	return string(runes[:maxRunes])
}
