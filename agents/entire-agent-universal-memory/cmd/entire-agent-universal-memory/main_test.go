package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestInfoReturnsProtocolV1(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"info"}, &stdout); err != nil {
		t.Fatalf("run(info): %v", err)
	}

	var response infoResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode info: %v", err)
	}
	if response.ProtocolVersion != protocolVersion || response.Name != "universal-memory" {
		t.Fatalf("unexpected info response: %#v", response)
	}
}

func TestUnknownCommandFails(t *testing.T) {
	err := run([]string{"unknown"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("run(unknown) error = %v", err)
	}
}
