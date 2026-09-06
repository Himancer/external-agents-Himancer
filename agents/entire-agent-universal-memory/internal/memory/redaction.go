// Package memory contains the bounded, safe handoff store.
package memory

import (
	"regexp"
	"strings"
)

type redactionRule struct {
	pattern     *regexp.Regexp
	replacement string
}

var redactionRules = []redactionRule{
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), "[REDACTED_AWS_ACCESS_KEY]"},
	{regexp.MustCompile(`\bsk-(?:proj-)?[a-zA-Z0-9_-]{20,}\b`), "[REDACTED_OPENAI_KEY]"},
	{regexp.MustCompile(`\bghp_[a-zA-Z0-9]{30,}\b`), "[REDACTED_GITHUB_TOKEN]"},
	{regexp.MustCompile(`\bgithub_pat_[a-zA-Z0-9_]{20,}\b`), "[REDACTED_GITHUB_TOKEN]"},
	{regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._-]{12,}`), "Bearer [REDACTED_TOKEN]"},
	{regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}\b`), "[REDACTED_JWT]"},
	{regexp.MustCompile(`\bsk_(?:live|test)_[a-zA-Z0-9]{12,}\b`), "[REDACTED_STRIPE_KEY]"},
	{regexp.MustCompile(`(?i)\b(API_KEY|OPENAI_API_KEY|DATABASE_URL|PASSWORD|SECRET|TOKEN)\s*=\s*[^\s'\"]+`), "$1=[REDACTED]"},
}

// Redact removes credential-shaped text from a handoff before it is persisted
// or displayed to a receiving workflow. It is deliberately conservative: it
// preserves the surrounding explanation while replacing the sensitive value.
func Redact(input string) string {
	output := input
	for _, rule := range redactionRules {
		output = rule.pattern.ReplaceAllString(output, rule.replacement)
	}
	return output
}

// HasSensitiveMaterial detects the credential forms that Redact can safely
// identify. Protocol-native blobs are rejected rather than silently rewritten
// because Entire requires opaque native bytes to round-trip unchanged.
func HasSensitiveMaterial(input string) bool {
	return Redact(input) != input || strings.Contains(input, "-----BEGIN") || strings.Contains(input, "\x00")
}
