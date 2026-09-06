package memory

import (
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStoreSaveLoadAndDeploymentPlan(t *testing.T) {
	store := Store{RepoRoot: t.TempDir()}
	saved, err := store.Save(HandoffContext{
		HandoffID: "demo-1", SourceAgent: "vscode-copilot", TargetAgent: "deployment-cli",
		DeveloperIntent:   "Add Redis caching using OPENAI_API_KEY=sk-1234567890abcdef1234567890",
		StructuralChanges: []string{"internal/cache.go", "compose.yml adds redis"},
		UnresolvedRisks:   []string{"Set REDIS_URL before rollout"}, CheckpointRef: "abc123",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if strings.Contains(saved.DeveloperIntent, "sk-123") {
		t.Fatalf("secret was not redacted: %q", saved.DeveloperIntent)
	}
	loaded, err := store.Load("demo-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.HandoffID != saved.HandoffID {
		t.Fatalf("loaded id = %q, want %q", loaded.HandoffID, saved.HandoffID)
	}
	if got := strings.Join(DeploymentPlan(loaded), " "); !strings.Contains(got, "Redis") {
		t.Fatalf("plan does not respond to Redis change: %q", got)
	}
}

func TestStoreRejectsUnsafeMaterialAndTraversal(t *testing.T) {
	store := Store{RepoRoot: t.TempDir()}
	if _, err := store.Save(HandoffContext{HandoffID: "../escape", SourceAgent: "a", DeveloperIntent: "safe"}); err == nil {
		t.Fatal("Save accepted traversal id")
	}
	if _, err := store.Save(HandoffContext{SourceAgent: "a", DeveloperIntent: "-----BEGIN PRIVATE KEY-----"}); err == nil {
		t.Fatal("Save accepted private key marker")
	}
	if _, err := store.Save(HandoffContext{SourceAgent: "a", DeveloperIntent: "safe"}); err == nil {
		t.Fatal("Save accepted missing structural changes")
	}
}

func TestLoadRejectsTamperedRecordAndKeepsUTF8Boundaries(t *testing.T) {
	store := Store{RepoRoot: t.TempDir()}
	path, err := store.HandoffPath("tampered")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.HandoffDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"handoff_id":"tampered","source_agent":"a","developer_intent":"safe"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("tampered"); err == nil {
		t.Fatal("Load accepted a record without structural changes")
	}
	value := limit(strings.Repeat("é", 300), 100)
	if !utf8.ValidString(value) || len(value) > 100 {
		t.Fatalf("limit produced invalid or oversized UTF-8: %q", value)
	}
}

func TestLatestReportsMissingContext(t *testing.T) {
	_, err := (Store{RepoRoot: t.TempDir()}).Latest()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Latest error = %v, want not exist", err)
	}
}
