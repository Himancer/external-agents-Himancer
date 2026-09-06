package memory

import "testing"

func TestRedact(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"keeps ordinary context", "Enable Redis and run migrations.", "Enable Redis and run migrations."},
		{"redacts AWS key", "AWS key AKIA1234567890ABCDEF", "AWS key [REDACTED_AWS_ACCESS_KEY]"},
		{"redacts OpenAI standard key", "OPENAI_API_KEY=sk-1234567890abcdef1234567890", "OPENAI_API_KEY=[REDACTED]"},
		{"redacts OpenAI project key", "key sk-proj-A1b2C3d4E5f6G7h8I9j0K1l2", "key [REDACTED_OPENAI_KEY]"},
		{"redacts GitHub classic token", "token ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ1234567890", "token [REDACTED_GITHUB_TOKEN]"},
		{"redacts GitHub fine-grained token", "token github_pat_11AABBCC01234567890abcdefghijklmnopqrstuvwxyz", "token [REDACTED_GITHUB_TOKEN]"},
		{"redacts bearer token", "Authorization: Bearer secret-token-123456", "Authorization: Bearer [REDACTED_TOKEN]"},
		{"redacts JWT", "token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signaturevalue", "token [REDACTED_JWT]"},
		{"redacts Stripe key", "use sk_live_1234567890abcdef", "use [REDACTED_STRIPE_KEY]"},
		{"redacts inline environment variable", "DATABASE_URL=postgres://user:pass@host/db deploy", "DATABASE_URL=[REDACTED] deploy"},
		{"redacts multiple secrets", "sk-1234567890abcdef1234567890 and ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ1234567890", "[REDACTED_OPENAI_KEY] and [REDACTED_GITHUB_TOKEN]"},
		{"keeps UUID", "handoff_id 123e4567-e89b-12d3-a456-426614174000", "handoff_id 123e4567-e89b-12d3-a456-426614174000"},
		{"keeps short sk prefix", "The skill is sk-level-2.", "The skill is sk-level-2."},
		{"redacts mixed case environment variable", "openai_api_key = sk-example-value", "openai_api_key=[REDACTED]"},
		{"redacts multiple secrets in one string", "AWS AKIA1234567890ABCDEF; Bearer deploy-token-123456; PASSWORD=long-secret-value", "AWS [REDACTED_AWS_ACCESS_KEY]; Bearer [REDACTED_TOKEN]; PASSWORD=[REDACTED]"},
		{"redacts long environment value", "SECRET=abcdefghijklmnopqrstuvwxyz0123456789-._~", "SECRET=[REDACTED]"},
		{"keeps near match for AWS key", "example AKIA1234567890ABCDE", "example AKIA1234567890ABCDE"},
		{"keeps near match for bearer token", "Authorization: Bearer short", "Authorization: Bearer short"},
		{"keeps environment name without assignment", "set API_KEY in the deployment environment", "set API_KEY in the deployment environment"},
		{"keeps colon separated environment text", "PASSWORD: do not print this value", "PASSWORD: do not print this value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Redact(tt.input); got != tt.want {
				t.Fatalf("Redact() = %q, want %q", got, tt.want)
			}
		})
	}
}
