package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/external-agents/agents/entire-agent-universal-memory/internal/memory"
)

func TestCaptureThenHandoffChangesDeploymentPlan(t *testing.T) {
	t.Setenv("ENTIRE_REPO_ROOT", t.TempDir())
	var captured bytes.Buffer
	err := captureCommand([]string{
		"--session-id", "redis-demo", "--source-agent", "vscode-copilot",
		"--intent", "Add Redis cache", "--changed", "internal/cache.go,compose.yml adds redis",
	}, &captured)
	if err != nil {
		t.Fatalf("captureCommand: %v", err)
	}
	var context memory.HandoffContext
	if err := json.Unmarshal(captured.Bytes(), &context); err != nil {
		t.Fatal(err)
	}
	if context.HandoffID != "redis-demo" {
		t.Fatalf("handoff id = %q", context.HandoffID)
	}

	var handedOff bytes.Buffer
	if err := handoffCommand([]string{"--session-id", "redis-demo"}, &handedOff); err != nil {
		t.Fatalf("handoffCommand: %v", err)
	}
	var result memory.HandoffResult
	if err := json.Unmarshal(handedOff.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Available || !strings.Contains(strings.Join(result.DeploymentPlan, " "), "Redis") {
		t.Fatalf("unexpected handoff result: %#v", result)
	}
	if !strings.Contains(result.Message, "No deployment has been executed") {
		t.Fatalf("unsafe handoff message: %q", result.Message)
	}
}

func TestOpaqueProtocolSessionRoundTrip(t *testing.T) {
	t.Setenv("ENTIRE_REPO_ROOT", t.TempDir())
	native := []byte(`{"tool":"write","value":"safe"}`)
	input, err := json.Marshal(agentSession{SessionID: "protocol-1", AgentName: "test-agent", NativeData: native})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSession(bytes.NewReader(input)); err != nil {
		t.Fatalf("writeSession: %v", err)
	}
	hook, _ := json.Marshal(hookInput{SessionID: "protocol-1"})
	var output bytes.Buffer
	if err := readSession(bytes.NewReader(hook), &output); err != nil {
		t.Fatalf("readSession: %v", err)
	}
	var result agentSession
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.NativeData, native) || result.ModifiedFiles == nil || result.NewFiles == nil || result.DeletedFiles == nil {
		t.Fatalf("protocol session did not round trip: %#v", result)
	}
}

func TestProtocolSessionDoesNotOverwriteCapturedHandoff(t *testing.T) {
	t.Setenv("ENTIRE_REPO_ROOT", t.TempDir())
	var captured bytes.Buffer
	if err := captureCommand([]string{
		"--session-id", "shared-id", "--source-agent", "vscode-copilot",
		"--intent", "Add Redis cache", "--changed", "compose.yml adds redis",
	}, &captured); err != nil {
		t.Fatalf("captureCommand: %v", err)
	}

	native := []byte(`{"tool":"write","value":"safe"}`)
	input, err := json.Marshal(agentSession{SessionID: "shared-id", AgentName: "test-agent", NativeData: native})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSession(bytes.NewReader(input)); err != nil {
		t.Fatalf("writeSession: %v", err)
	}

	var output bytes.Buffer
	if err := handoffCommand([]string{"--session-id", "shared-id"}, &output); err != nil {
		t.Fatalf("handoffCommand: %v", err)
	}
	var result memory.HandoffResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Available || result.Context == nil || result.Context.DeveloperIntent != "Add Redis cache" {
		t.Fatalf("handoff was overwritten by protocol session: %#v", result)
	}
}

func TestHandoffReportsMissingContext(t *testing.T) {
	t.Setenv("ENTIRE_REPO_ROOT", t.TempDir())
	var output bytes.Buffer
	if err := handoffCommand(nil, &output); err != nil {
		t.Fatal(err)
	}
	var result memory.HandoffResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Available || !strings.Contains(result.Message, "No handoff context") {
		t.Fatalf("unexpected missing handoff response: %#v", result)
	}
}

func TestCaptureDerivesHandoffFromEventTranscript(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", repo)
	path := writeTranscriptFixture(t, repo, strings.Join([]string{
		`{"event":"session_started","session_id":"event-session","agent":{"name":"AcmeCode"}}`,
		`{"event":"user_prompt","session_id":"event-session","text":"Add Redis cache."}`,
		`{"event":"file_changed","session_id":"event-session","path":"internal/cache.go","change":"modified"}`,
		`{"event":"checkpoint_created","session_id":"event-session","checkpoint_id":"cp-42","summary":"Redis cache complete.","intent":"Add Redis cache.","open_questions":["Confirm TTL."]}`,
	}, "\n")+"\n")

	var output bytes.Buffer
	if err := captureCommand([]string{"--transcript", path}, &output); err != nil {
		t.Fatalf("captureCommand: %v", err)
	}
	var context memory.HandoffContext
	if err := json.Unmarshal(output.Bytes(), &context); err != nil {
		t.Fatal(err)
	}
	if context.HandoffID != "event-session" || context.SourceAgent != "AcmeCode" || context.DeveloperIntent != "Add Redis cache." {
		t.Fatalf("transcript metadata was not captured: %#v", context)
	}
	if context.ContextStatus != memory.TranscriptStatusComplete || context.TranscriptFormat != memory.TranscriptFormatEvent || context.CheckpointRef != "cp-42" {
		t.Fatalf("transcript evidence was not captured: %#v", context)
	}
	if got := strings.Join(context.StructuralChanges, " "); !strings.Contains(got, "internal/cache.go") {
		t.Fatalf("changed file missing: %#v", context.StructuralChanges)
	}
}

func TestCaptureMarksTruncatedTranscriptAsPartial(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", repo)
	path := writeTranscriptFixture(t, repo, strings.Join([]string{
		`{"event":"session_started","session_id":"partial-session","agent":{"name":"AcmeCode"}}`,
		`{"event":"user_prompt","session_id":"partial-session","text":"Add Redis cache."}`,
		`{"event":"file_changed","session_id":"partial-session","path":"internal/cache.go","change":"modified"}`,
		`{"event":"agent_response","session_id":"partial-session","text":"truncated`,
	}, "\n"))

	var captured bytes.Buffer
	if err := captureCommand([]string{"--transcript", path}, &captured); err != nil {
		t.Fatalf("captureCommand: %v", err)
	}
	var context memory.HandoffContext
	if err := json.Unmarshal(captured.Bytes(), &context); err != nil {
		t.Fatal(err)
	}
	if context.ContextStatus != memory.TranscriptStatusPartial || len(context.ContextWarnings) == 0 {
		t.Fatalf("partial status was not stored: %#v", context)
	}

	var handedOff bytes.Buffer
	if err := handoffCommand([]string{"--session-id", "partial-session"}, &handedOff); err != nil {
		t.Fatalf("handoffCommand: %v", err)
	}
	var result memory.HandoffResult
	if err := json.Unmarshal(handedOff.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Available || !strings.Contains(result.Message, "PARTIAL") {
		t.Fatalf("partial context was not clearly disclosed: %#v", result)
	}
}

func TestCaptureSupportsLegacyTranscriptWithoutAgentMetadata(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ENTIRE_REPO_ROOT", repo)
	path := writeTranscriptFixture(t, repo, strings.Join([]string{
		`{"session_id":"legacy-session","role":"user","content":"Add cache.","modified_files":["internal/cache.go"]}`,
		`{"session_id":"legacy-session","role":"assistant","content":"Cache complete.","checkpoint_ref":"legacy-cp"}`,
	}, "\n")+"\n")

	var output bytes.Buffer
	if err := captureCommand([]string{"--transcript", path}, &output); err != nil {
		t.Fatalf("captureCommand: %v", err)
	}
	var context memory.HandoffContext
	if err := json.Unmarshal(output.Bytes(), &context); err != nil {
		t.Fatal(err)
	}
	if context.HandoffID != "legacy-session" || context.SourceAgent != "legacy-workflow" || context.TranscriptFormat != memory.TranscriptFormatLegacy {
		t.Fatalf("legacy transcript was not captured safely: %#v", context)
	}
	if len(context.ContextWarnings) == 0 || !strings.Contains(strings.Join(context.ContextWarnings, " "), "did not identify") {
		t.Fatalf("legacy source fallback was not disclosed: %#v", context.ContextWarnings)
	}
}

func TestTranscriptAnalyzerCommands(t *testing.T) {
	path := writeTranscriptFixture(t, t.TempDir(), strings.Join([]string{
		`{"event":"session_started","session_id":"event-session","agent":{"name":"AcmeCode"}}`,
		`{"event":"user_prompt","session_id":"event-session","text":"Add Redis cache."}`,
		`{"event":"file_changed","session_id":"event-session","path":"internal/cache.go","change":"modified"}`,
		`{"event":"agent_response","session_id":"event-session","text":"Redis cache complete."}`,
	}, "\n")+"\n")

	var positionOutput bytes.Buffer
	if err := getTranscriptPosition([]string{"--path", path}, &positionOutput); err != nil {
		t.Fatalf("getTranscriptPosition: %v", err)
	}
	var position struct {
		Position int `json:"position"`
	}
	if err := json.Unmarshal(positionOutput.Bytes(), &position); err != nil {
		t.Fatal(err)
	}
	if position.Position != 4 {
		t.Fatalf("position = %d, want 4", position.Position)
	}

	var filesOutput bytes.Buffer
	if err := extractModifiedFiles([]string{"--path", path, "--offset", "2"}, &filesOutput); err != nil {
		t.Fatalf("extractModifiedFiles: %v", err)
	}
	var files struct {
		Files           []string `json:"files"`
		CurrentPosition int      `json:"current_position"`
	}
	if err := json.Unmarshal(filesOutput.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files.Files) != 1 || files.Files[0] != "internal/cache.go" || files.CurrentPosition != 4 {
		t.Fatalf("unexpected file response: %#v", files)
	}

	var promptsOutput bytes.Buffer
	if err := extractPrompts([]string{"--session-ref", path, "--offset", "0"}, &promptsOutput); err != nil {
		t.Fatalf("extractPrompts: %v", err)
	}
	var prompts struct {
		Prompts []string `json:"prompts"`
	}
	if err := json.Unmarshal(promptsOutput.Bytes(), &prompts); err != nil {
		t.Fatal(err)
	}
	if len(prompts.Prompts) != 1 || prompts.Prompts[0] != "Add Redis cache." {
		t.Fatalf("unexpected prompts response: %#v", prompts)
	}

	var summaryOutput bytes.Buffer
	if err := extractSummary([]string{"--session-ref", path}, &summaryOutput); err != nil {
		t.Fatalf("extractSummary: %v", err)
	}
	var summary struct {
		Summary    string `json:"summary"`
		HasSummary bool   `json:"has_summary"`
	}
	if err := json.Unmarshal(summaryOutput.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if !summary.HasSummary || summary.Summary != "Redis cache complete." {
		t.Fatalf("unexpected summary response: %#v", summary)
	}
}

func TestGetTranscriptPositionDoesNotAdvancePastTerminalPartialRecord(t *testing.T) {
	path := writeTranscriptFixture(t, t.TempDir(), strings.Join([]string{
		`{"event":"session_started","session_id":"event-session"}`,
		`{"event":"user_prompt","session_id":"event-session","text":"Add Redis cache."}`,
		`{"event":"agent_response","session_id":"event-session","text":"unfinished`,
	}, "\n")+"\n")

	var output bytes.Buffer
	if err := getTranscriptPosition([]string{"--path", path}, &output); err != nil {
		t.Fatalf("getTranscriptPosition: %v", err)
	}
	var result struct {
		Position int `json:"position"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Position != 2 {
		t.Fatalf("position = %d, want 2 completed records", result.Position)
	}
}

func TestTranscriptAnalyzerTreatsMissingTranscriptAsEmpty(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-created.jsonl")

	var positionOutput bytes.Buffer
	if err := getTranscriptPosition([]string{"--path", missing}, &positionOutput); err != nil {
		t.Fatalf("getTranscriptPosition: %v", err)
	}
	var position struct {
		Position int `json:"position"`
	}
	if err := json.Unmarshal(positionOutput.Bytes(), &position); err != nil {
		t.Fatal(err)
	}
	if position.Position != 0 {
		t.Fatalf("missing transcript position = %d, want 0", position.Position)
	}

	var filesOutput bytes.Buffer
	if err := extractModifiedFiles([]string{"--path", missing, "--offset", "0"}, &filesOutput); err != nil {
		t.Fatalf("extractModifiedFiles: %v", err)
	}
	var files struct {
		Files           []string `json:"files"`
		CurrentPosition int      `json:"current_position"`
	}
	if err := json.Unmarshal(filesOutput.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files.Files) != 0 || files.CurrentPosition != 0 {
		t.Fatalf("missing transcript files = %#v", files)
	}
}

func writeTranscriptFixture(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
