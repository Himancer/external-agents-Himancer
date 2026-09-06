package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

const protocolVersion = 1

type capabilities struct {
	Hooks                  bool `json:"hooks"`
	TranscriptAnalyzer     bool `json:"transcript_analyzer"`
	TranscriptPreparer     bool `json:"transcript_preparer"`
	TokenCalculator        bool `json:"token_calculator"`
	TextGenerator          bool `json:"text_generator"`
	HookResponseWriter     bool `json:"hook_response_writer"`
	SubagentAwareExtractor bool `json:"subagent_aware_extractor"`
}

type infoResponse struct {
	ProtocolVersion int          `json:"protocol_version"`
	Name            string       `json:"name"`
	Type            string       `json:"type"`
	Description     string       `json:"description"`
	IsPreview       bool         `json:"is_preview"`
	ProtectedDirs   []string     `json:"protected_dirs"`
	HookNames       []string     `json:"hook_names"`
	Capabilities    capabilities `json:"capabilities"`
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: entire-agent-universal-memory <subcommand>")
	}

	var response any
	switch args[0] {
	case "info":
		response = infoResponse{
			ProtocolVersion: protocolVersion,
			Name:            "universal-memory",
			Type:            "Universal Agent Memory",
			Description:     "Bounded, redacted context handoff between coding and deployment workflows",
			IsPreview:       true,
			ProtectedDirs:   []string{".entire/universal-memory"},
			HookNames:       []string{},
			Capabilities: capabilities{
				TranscriptAnalyzer: true,
			},
		}
	case "detect":
		response = struct {
			Present bool `json:"present"`
		}{Present: true}
	case "capture":
		return captureCommand(args[1:], stdout)
	case "handoff":
		return handoffCommand(args[1:], stdout)
	case "get-session-id":
		return getSessionID(os.Stdin, stdout)
	case "get-session-dir":
		return getSessionDir(args[1:], stdout)
	case "resolve-session-file":
		return resolveSessionFile(args[1:], stdout)
	case "read-session":
		return readSession(os.Stdin, stdout)
	case "write-session":
		return writeSession(os.Stdin)
	case "read-transcript":
		return readTranscript(args[1:], stdout)
	case "get-transcript-position":
		return getTranscriptPosition(args[1:], stdout)
	case "extract-modified-files":
		return extractModifiedFiles(args[1:], stdout)
	case "extract-prompts":
		return extractPrompts(args[1:], stdout)
	case "extract-summary":
		return extractSummary(args[1:], stdout)
	case "chunk-transcript":
		return chunkTranscript(args[1:], os.Stdin, stdout)
	case "reassemble-transcript":
		return reassembleTranscript(os.Stdin, stdout)
	case "format-resume-command":
		fs := flag.NewFlagSet("format-resume-command", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		id := fs.String("session-id", "", "session id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *id == "" {
			return fmt.Errorf("session-id is required")
		}
		response = struct {
			Command string `json:"command"`
		}{"entire-agent-universal-memory handoff --session-id " + *id}
	default:
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}

	return json.NewEncoder(stdout).Encode(response)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
