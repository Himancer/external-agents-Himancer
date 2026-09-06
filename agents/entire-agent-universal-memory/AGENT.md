# Universal Agent Memory - External Agent Design

## Verdict

Universal Agent Memory is a **preview-compatible** Entire external-agent
adapter for a controlled coding-to-handoff workflow. It deliberately supports
the protocol features it can verify locally and does not claim unverified
native IDE lifecycle hooks.

## What it does

The binary captures bounded, redacted developer context and makes it available
to a later review or deployment workflow:

1. A caller supplies handoff fields directly, or runs capture with a transcript.
2. One normalizer reads either the documented legacy role/content JSONL shape
   or the Track 3 lifecycle-event JSONL shape.
3. The result is stored as a HandoffContext under the selected repository.
4. A later handoff command reads that context and returns recommendations only;
   it never executes a deployment.

## Runtime layout

All runtime data remains below the repository selected by ENTIRE_REPO_ROOT
(or the current working directory).

| Purpose | Location |
| --- | --- |
| Opaque Entire protocol session envelope | .entire/universal-memory/sessions/<session-id>.json |
| Validated coding-to-deployment handoff | .entire/universal-memory/handoffs/<handoff-id>.json |

The namespaces are intentionally separate. A protocol session and a handoff
can use the same identifier without overwriting each other. Runtime state is
ignored by Git.

## Transcript formats

### Legacy format

The original documented format is JSONL records with role, content, session_id,
and optional modified_files, new_files, deleted_files, checkpoint_ref, or
summary fields. This is the compatibility baseline for existing users.

### Event format

The Curveball format uses JSONL lifecycle events including session_started,
user_prompt, agent_response, file_changed, checkpoint_created, and
session_ended. A new event is normalized into the same internal
TranscriptAnalysis as a legacy record.

### Format safety

- Unknown named events do not crash parsing. They are counted and returned in
  warnings while known context stays available.
- A malformed non-final record is an explicit error because it may indicate
  hidden corruption.
- A malformed final record yields partial context containing every valid prior
  record. It is not counted in the incremental position, so a later read can
  process it after the writer completes it.
- Text, paths, and warnings are redacted and bounded before they leave the
  parser.
- The most recent checkpoint reference is paired with its most recent intent
  and summary.

## Protocol mapping

| Command | Behaviour |
| --- | --- |
| info, detect | Declares protocol v1, preview metadata, and transcript_analyzer support. |
| get-session-id, get-session-dir, resolve-session-file | Implements required session helpers. |
| read-session, write-session | Round-trips opaque protocol session envelopes. |
| read-transcript, chunk-transcript, reassemble-transcript | Safely reads repo-local session data and handles byte-preserving transcript chunks. |
| get-transcript-position | Returns completed JSONL record count. |
| extract-modified-files | Returns ordered, deduplicated changed files after an offset. |
| extract-prompts | Returns user prompts after an offset. |
| extract-summary | Returns the latest summary and whether one exists. |
| capture | Persists a bounded handoff, optionally derived from --transcript. |
| handoff | Returns stored context and deployment review recommendations; it does not deploy. |
| format-resume-command | Formats the explicit handoff resume command. |

## Selected capabilities

| Capability | Value | Reason |
| --- | --- | --- |
| hooks | false | Native Codex/Copilot payloads and lifecycle behaviour have not been verified. |
| transcript_analyzer | true | The four analyzer commands support the legacy and supplied event JSONL formats. |
| transcript_preparer | false | No conversion is required before normalization. |
| token_calculator | false | The workflow has no trustworthy provider token count. |
| text_generator | false | The workflow does not require a model API key. |
| hook_response_writer | false | There is no verified native hook response transport. |
| subagent_aware_extractor | false | Not in this focused MVP. |

## Capture and handoff example

~~~powershell
$env:ENTIRE_REPO_ROOT = (Get-Location).Path
./entire-agent-universal-memory.exe capture --transcript ./internal/memory/testdata/track-3-event-format.jsonl
./entire-agent-universal-memory.exe handoff --session-id btw-track3-demo-001
~~~

A legacy transcript without source-agent metadata is labelled
legacy-workflow, and that fallback is placed in context_warnings rather than
silently presented as provenance.

## Data safety

- Handoff fields have fixed size and list bounds.
- Obvious provider credentials, bearer tokens, AWS-style keys, and sensitive
  environment assignments are redacted.
- Unsafe PEM-like content and NUL bytes are rejected before persistence.
- Missing handoff context returns an explicit unavailable result.
- A partial transcript produces a partial handoff message and requires review;
  it is never called authoritative.

## Verification

~~~powershell
go test ./...
go build -o entire-agent-universal-memory.exe ./cmd/entire-agent-universal-memory
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify-universal-memory.ps1 -AgentBin ./entire-agent-universal-memory.exe
~~~

If mise is installed and on PATH, `mise run test` and `mise run build` are
equivalent convenience commands.

The unit tests cover redaction, safe storage, protocol-session/handoff namespace
separation, legacy parsing, new event parsing, unknown events, incomplete
terminal input, incremental recovery, latest checkpoint selection, missing
analyzer input, and capture/handoff behaviour.

Native IDE hooks remain an explicit known limitation. An end-to-end lifecycle
test will be added only after real hook payloads and supported agent CLIs are
available.
