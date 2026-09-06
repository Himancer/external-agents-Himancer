package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeTranscriptSupportsLegacyFormat(t *testing.T) {
	data := readTranscriptFixture(t, "legacy-format.jsonl")
	analysis, err := AnalyzeTranscript(data)
	if err != nil {
		t.Fatalf("AnalyzeTranscript: %v", err)
	}
	if analysis.Format != TranscriptFormatLegacy || analysis.Status != TranscriptStatusComplete {
		t.Fatalf("unexpected legacy analysis: %#v", analysis)
	}
	if analysis.SessionID != "legacy-session-001" {
		t.Fatalf("session id = %q", analysis.SessionID)
	}
	if got := strings.Join(analysis.Prompts, " "); !strings.Contains(got, "Redis") {
		t.Fatalf("prompt was not preserved: %#v", analysis.Prompts)
	}
	if got := strings.Join(analysis.ModifiedFiles, " "); !strings.Contains(got, "internal/cache.go") || !strings.Contains(got, "tests/cache_test.go") {
		t.Fatalf("modified files were not preserved: %#v", analysis.ModifiedFiles)
	}
	if analysis.CheckpointRef != "legacy-cp-001" || !strings.Contains(analysis.Summary, "complete") {
		t.Fatalf("legacy summary/checkpoint not preserved: %#v", analysis)
	}
}

func TestAnalyzeTranscriptSupportsTrackThreeEventFormat(t *testing.T) {
	data := readTranscriptFixture(t, "track-3-event-format.jsonl")
	analysis, err := AnalyzeTranscript(data)
	if err != nil {
		t.Fatalf("AnalyzeTranscript: %v", err)
	}
	if analysis.Format != TranscriptFormatEvent || analysis.Status != TranscriptStatusComplete {
		t.Fatalf("unexpected event analysis: %#v", analysis)
	}
	if analysis.SessionID != "btw-track3-demo-001" || analysis.SourceAgent != "AcmeCode" {
		t.Fatalf("session metadata missing: %#v", analysis)
	}
	if len(analysis.Prompts) != 1 || !strings.Contains(analysis.Prompts[0], "coupon validation") {
		t.Fatalf("prompt was not normalized: %#v", analysis.Prompts)
	}
	if got := strings.Join(analysis.ModifiedFiles, " "); !strings.Contains(got, "src/checkout/apply_coupon.ts") || !strings.Contains(got, "tests/checkout/apply_coupon.test.ts") {
		t.Fatalf("file changes were not normalized: %#v", analysis.ModifiedFiles)
	}
	if analysis.CheckpointRef != "cp-001" || !strings.Contains(analysis.Summary, "Coupon validation") || !strings.Contains(analysis.DeveloperIntent, "Reject expired") {
		t.Fatalf("checkpoint data was not normalized: %#v", analysis)
	}
	if len(analysis.OpenQuestions) != 1 || analysis.OpenQuestions[0] == "" {
		t.Fatalf("open questions missing: %#v", analysis.OpenQuestions)
	}
}

func TestAnalyzeTranscriptIgnoresUnknownEvents(t *testing.T) {
	data := []byte("{\"event\":\"session_started\",\"session_id\":\"s-1\"}\n" +
		"{\"event\":\"future_event\",\"session_id\":\"s-1\",\"value\":\"ignored\"}\n" +
		"{\"event\":\"user_prompt\",\"session_id\":\"s-1\",\"text\":\"Keep known context.\"}\n")
	analysis, err := AnalyzeTranscript(data)
	if err != nil {
		t.Fatalf("AnalyzeTranscript: %v", err)
	}
	if analysis.Status != TranscriptStatusComplete || analysis.UnknownEventCount != 1 {
		t.Fatalf("unknown event was not safely recorded: %#v", analysis)
	}
	if len(analysis.Prompts) != 1 || analysis.Prompts[0] != "Keep known context." {
		t.Fatalf("known event was lost: %#v", analysis.Prompts)
	}
}

func TestAnalyzeTranscriptReturnsPartialResultForTruncatedFinalRecord(t *testing.T) {
	data := []byte("{\"event\":\"session_started\",\"session_id\":\"s-1\"}\n" +
		"{\"event\":\"user_prompt\",\"session_id\":\"s-1\",\"text\":\"Keep this prompt.\"}\n" +
		"{\"event\":\"agent_response\",\"session_id\":\"s-1\",\"text\":\"truncated")
	analysis, err := AnalyzeTranscript(data)
	if err != nil {
		t.Fatalf("AnalyzeTranscript: %v", err)
	}
	if analysis.Status != TranscriptStatusPartial || len(analysis.Prompts) != 1 || !strings.Contains(strings.Join(analysis.Warnings, " "), "truncated") {
		t.Fatalf("truncated final record was not retained as partial: %#v", analysis)
	}
}

func TestAnalyzeTranscriptLeavesTerminalPartialRecordUnconsumed(t *testing.T) {
	prefix := "{\"event\":\"session_started\",\"session_id\":\"s-1\"}\n" +
		"{\"event\":\"user_prompt\",\"session_id\":\"s-1\",\"text\":\"Keep this prompt.\"}\n"
	partial := []byte(prefix + "{\"event\":\"agent_response\",\"session_id\":\"s-1\",\"text\":\"unfinished\n")
	analysis, position, err := AnalyzeTranscriptAfterOffset(partial, 0)
	if err != nil {
		t.Fatalf("AnalyzeTranscriptAfterOffset(partial): %v", err)
	}
	if analysis.Status != TranscriptStatusPartial || position != 2 {
		t.Fatalf("partial transcript consumed an incomplete record: status=%q position=%d", analysis.Status, position)
	}

	completed := []byte(prefix + "{\"event\":\"agent_response\",\"session_id\":\"s-1\",\"text\":\"finished\"}\n")
	resumed, finalPosition, err := AnalyzeTranscriptAfterOffset(completed, position)
	if err != nil {
		t.Fatalf("AnalyzeTranscriptAfterOffset(completed): %v", err)
	}
	if finalPosition != 3 || resumed.Summary != "finished" {
		t.Fatalf("completed record was skipped after resume: position=%d summary=%q", finalPosition, resumed.Summary)
	}
}

func TestAnalyzeTranscriptUsesLatestCheckpoint(t *testing.T) {
	data := []byte("{\"event\":\"session_started\",\"session_id\":\"s-1\"}\n" +
		"{\"event\":\"checkpoint_created\",\"session_id\":\"s-1\",\"checkpoint_id\":\"cp-early\",\"intent\":\"Add cache.\"}\n" +
		"{\"event\":\"checkpoint_created\",\"session_id\":\"s-1\",\"checkpoint_id\":\"cp-final\",\"intent\":\"Add cache safely.\"}\n")
	analysis, err := AnalyzeTranscript(data)
	if err != nil {
		t.Fatalf("AnalyzeTranscript: %v", err)
	}
	if analysis.CheckpointRef != "cp-final" || analysis.DeveloperIntent != "Add cache safely." {
		t.Fatalf("latest checkpoint was not selected: %#v", analysis)
	}
}

func TestAnalyzeTranscriptRedactsSecretShapedPathAndUnknownEvent(t *testing.T) {
	data := []byte("{\"event\":\"session_started\",\"session_id\":\"s-1\"}\n" +
		"{\"event\":\"file_changed\",\"session_id\":\"s-1\",\"path\":\"config/AKIAIOSFODNN7EXAMPLE.txt\",\"change\":\"modified\"}\n" +
		"{\"event\":\"ghp_abcdefghijklmnopqrstuvwxyz1234567890ABCD\",\"session_id\":\"s-1\"}\n")
	analysis, err := AnalyzeTranscript(data)
	if err != nil {
		t.Fatalf("AnalyzeTranscript: %v", err)
	}
	combined := strings.Join(append(analysis.ModifiedFiles, analysis.Warnings...), " ")
	if strings.Contains(combined, "AKIAIOSFODNN7EXAMPLE") || strings.Contains(combined, "ghp_abcdefghijklmnopqrstuvwxyz1234567890ABCD") {
		t.Fatalf("analyzer output exposed secret-shaped input: %#v", analysis)
	}
}

func TestAnalyzeTranscriptRejectsMalformedMiddleRecord(t *testing.T) {
	data := []byte("{\"event\":\"session_started\",\"session_id\":\"s-1\"}\n" +
		"{not-json}\n" +
		"{\"event\":\"user_prompt\",\"session_id\":\"s-1\",\"text\":\"later\"}\n")
	if _, err := AnalyzeTranscript(data); err == nil {
		t.Fatal("AnalyzeTranscript accepted a malformed middle record")
	}
}

func readTranscriptFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
