package main

import (
	"bytes"
	"encoding/json"
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
