# Universal Agent Memory

A preview Entire external-agent adapter for a local coding-to-handoff workflow.
It converts either a legacy role/content JSONL transcript or the Track 3
lifecycle-event JSONL format into bounded, redacted context for a later
developer or deployment review. It never executes a deployment.

For the architectural rationale, protocol mapping, format guarantees, and
known limits, read AGENT.md. The Buildathon evidence is in the repository-root
BUILDATHON.md.

## Requirements

- Go 1.26 or newer
- Optional: mise, which reads this directory's mise.toml
- No model API key or external service is required

## Build and test

~~~powershell
cd agents/entire-agent-universal-memory
mise run test
mise run build
.entire-agent-universal-memory.exe info
~~~

Without mise:

~~~powershell
go test ./...
go vet ./...
go build -o entire-agent-universal-memory.exe ./cmd/entire-agent-universal-memory
~~~

## Demonstrate the critical path

On Windows:

~~~powershell
.scriptserify-universal-memory.ps1
~~~

On Bash-compatible systems:

~~~bash
AGENT_BIN=./entire-agent-universal-memory ./scripts/verify-universal-memory.sh
~~~

Both scripts create only temporary runtime data. They parse the committed
Track 3 fixture, capture a handoff, read it back, and assert that the result is
a review-only recommendation.

## Manual use

~~~powershell
$env:ENTIRE_REPO_ROOT = (Get-Location).Path
.entire-agent-universal-memory.exe capture --transcript .internalmemory	estdata	rack-3-event-format.jsonl
.entire-agent-universal-memory.exe handoff --session-id btw-track3-demo-001
~~~

The adapter advertises transcript_analyzer support. It does not advertise
native hooks because real Codex/Copilot hook payloads and lifecycle behaviour
have not been verified.
