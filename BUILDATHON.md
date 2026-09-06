# Universal Agent Memory

## One-sentence summary

Universal Agent Memory is a local Entire external-agent adapter that turns a
coding agent's legacy or lifecycle-event JSONL transcript into a bounded,
redacted, reviewable handoff for the next developer or deployment workflow.

## Problem, intended user, and why it matters

Developers frequently switch from a coding agent to another developer, a
reviewer, or a deployment workflow. A Git diff shows what changed but not the
user intent, test result, open question, or checkpoint that explains why the
change is safe. The intended user is a developer who needs to hand work off
without pasting an unbounded or secret-bearing chat transcript into a second
tool.

## Selected Entire track and why Entire is essential

**Track 3 - Bring Entire to a New Agent or Workflow.**

This is an Entire external-agent binary, not a wrapper that merely invokes an
Entire command. It implements protocol v1 session helpers, transcript
chunking/reassembly, transcript-analyzer commands, and a repository-scoped
handoff store. Entire can discover the binary and use its analyzer contract;
the adapter then keeps handoff state below the repository's .entire runtime
directory. The product's key value - preserving development context across a
workflow boundary - is therefore supplied through Entire's external agent
protocol and local checkpoint-oriented workflow.

## Submission identity

- GitHub fork: [Himancer/external-agents-Himancer](https://github.com/Himancer/external-agents-Himancer)
- Submission branch: [buildathon/universal-agent-memory](https://github.com/Himancer/external-agents-Himancer/tree/buildathon/universal-agent-memory)
- Entire India mirror: `entire://aws-ap-south-1.entire.io/gh/himancer/external-agents-himancer`
- Code implementation commit: [6750384](https://github.com/Himancer/external-agents-Himancer/commit/6750384dfa5d35fe3bb8421e25f7a9f0b83ca47b)
- The submission form must use the final branch-head SHA printed by `git rev-parse HEAD`; documentation-only commits made after the implementation can change that SHA.

## Architecture and main workflow

~~~text
legacy role/content JSONL OR new lifecycle-event JSONL
                         |
                         v
             one normalizer / TranscriptAnalysis
                         |
                         v
 capture --transcript -> bounded, redacted HandoffContext
                         |
                         v
 .entire/universal-memory/handoffs/<id>.json
                         |
                         v
 handoff --session-id -> review-only deployment recommendations
~~~

- Opaque Entire protocol session envelopes are kept separately at
  .entire/universal-memory/sessions/<id>.json.
- Handoff packets live at
  .entire/universal-memory/handoffs/<id>.json; the two namespaces prevent
  a protocol session from overwriting a same-named handoff.
- capture accepts explicit fields or --transcript <path>. It records a session
  ID, developer intent, changed files, checkpoint reference, open questions,
  transcript format, and whether the context is complete.
- handoff never deploys anything. It returns review recommendations and
  prominently labels a partial input as PARTIAL.
- Obvious bearer tokens, provider keys, AWS-style keys, and credential-like
  environment assignments are redacted before persistence or analyzer output.

## Noon Curveball: what changed and how we adapted

### Invalidated assumption

The baseline assumed one documented legacy JSONL shape with role and content.
The Track 3 Curveball supplied a new lifecycle-event JSONL format with records
such as session_started, user_prompt, file_changed, and checkpoint_created.

### Revised design

We did **not** duplicate capture or handoff implementations. A single parser
normalizes both formats into TranscriptAnalysis; the existing capture and
handoff paths consume that shared result. The event fixture supplied at noon
is represented in
agents/entire-agent-universal-memory/internal/memory/testdata/track-3-event-format.jsonl.
The legacy test fixture is an internal fixture derived from the format that
the pre-Curveball adapter documentation described; it is not claimed to be an
organizer-supplied legacy fixture.

### Safety behaviour

- A named event the adapter does not understand is ignored, counted, and
  returned as a warning; known records remain usable.
- A malformed non-final record fails clearly rather than silently hiding
  corruption.
- A malformed terminal record yields a partial result containing all prior
  valid records. Its incremental position does not advance, so a later read
  can consume the completed record rather than losing it.
- The latest checkpoint in a transcript is selected consistently with its
  latest summary and intent.
- Existing opaque protocol-session storage remains separate from handoff
  storage and has a regression test for same-ID collision safety.

## Entire Graph findings and verification

Graph was used before changing the parser and handoff path:

~~~powershell
entire graph search --repo . --profile full --query "transcript session handoff checkpoint capture"
entire graph impact --repo . --symbol HandoffContext --depth 2
~~~

The search located captureCommand, protocol-session helpers, and the two
runtime storage paths. The HandoffContext impact result identified its
sanitization, persistence (Save, Load, and Latest), and DeploymentPlan
consumers. Source review confirmed that a shared normalized representation
could be added at the parser boundary without changing the existing
protocol-session write/read contract.

The final semantic diff is run before submission with:

~~~powershell
entire graph diff --repo . --base a0cd69382e6ce20c30142deac935703b44f0a6ce --head HEAD --json
~~~

The final diff was run after the functional implementation and rerun after the
Windows setup documentation correction. It identified the changed capture and
handoff command paths plus the new shared TranscriptAnalysis parser and
analyzer handlers. The graph reported three bounded blind spots: the two JSONL
fixtures and PowerShell verification script have no supported semantic parser.
Those files were verified directly through unit tests and the Windows
verification script. Graph output is treated as evidence to verify, not as an
oracle.

## Checkpoint links and what each checkpoint proves

| Required milestone | Evidence | Honest status |
| --- | --- | --- |
| Initial understanding and architecture | No checkpoint was created before noon. | Not met; cannot be backdated. |
| Last stable state before the Noon Curveball | No 11:45 AM checkpoint or pre-noon stable commit exists. | Not met; cannot be backdated. |
| Fresh-session reconstruction | Local Entire record 74c103fc007c6aacd0d1fc591b2cb53b50466464, session 01a0758d-bff5-7f01-92cd-25fea00253d0, created at 12:40:52 IST. | Post-Curveball recovery evidence only; it is not a verified remote checkpoint link. |
| Curveball response and final verification | Local Entire checkpoint `c4f8c7ce08ed`, created by the normal Codex `manual-commit` strategy after the final Graph-verification documentation commit. Inspect locally with `entire checkpoint explain c4f8c7ce08ed --json`. | Final checkpoint exists locally; remote synchronization and a shareable checkpoint link are not verified. |

The baseline commit a0cd69382e6ce20c30142deac935703b44f0a6ce was created after noon.
It is a recovery baseline, not evidence that the pre-Curveball process
requirement was met.

The guide asks for four accessible checkpoint links. Those links are not
available for this submission: the initial and pre-noon checkpoints were not
created, and both the recovery record and final checkpoint are local-only with
remote synchronization unavailable at verification time. This is disclosed
rather than fabricated.

## Setup, run, and test instructions

Install Go 1.26 or newer and make `go` available on PATH. Then, from the
repository root on Windows PowerShell:

~~~powershell
cd agents/entire-agent-universal-memory
go test ./...
go vet ./...
go build -o entire-agent-universal-memory.exe ./cmd/entire-agent-universal-memory
./entire-agent-universal-memory.exe info
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./scripts/verify-universal-memory.ps1 -AgentBin ./entire-agent-universal-memory.exe
~~~

If `mise` is installed and on PATH, `mise run test` and `mise run build` are
equivalent convenience commands. They are optional; this host did not rely on
them for the verified path above.

To demonstrate the parser and handoff manually after building:

~~~powershell
./entire-agent-universal-memory.exe capture --transcript ./internal/memory/testdata/track-3-event-format.jsonl
./entire-agent-universal-memory.exe handoff --session-id btw-track3-demo-001
~~~

The critical-path verification script creates a temporary runtime directory,
captures the committed Track 3 event fixture, and reads the resulting handoff.
It does not deploy an application or contact an external model API.
Use this command for the live terminal demonstration; a short fallback screen
recording is recommended before judging.

### Verified locally

- go test ./... passes.
- go vet ./... passes.
- The shared external-agent protocol compliance harness passes against a
  freshly built adapter binary.
- A local capture -> handoff run using the supplied Track 3 event fixture
  preserves AcmeCode, the coupon-validation intent, two changed files,
  checkpoint cp-001, and the unresolved validation-order question.

## Databricks use, data sources, and limitations

Databricks is not used in this submission. The product is intentionally a
local, file-backed workflow so it can preserve context without uploading raw
transcripts to another service. The committed transcript fixtures are
synthetic demonstration data supplied for the Buildathon format exercise or
derived from the documented legacy shape; no customer data, credentials, or
model API keys are required.

## Known limitations and next steps

- Native Codex, Copilot, and other IDE lifecycle hooks are deliberately not
  advertised by this adapter (hooks: false). They need real source payloads
  and end-to-end lifecycle tests before being claimed as supported.
- The adapter supports the documented legacy shape and the supplied new event
  fixture, not every possible third-party transcript schema.
- The post-Curveball recovery record and final checkpoint `c4f8c7ce08ed` are
  local-only and cannot be presented as verified remote checkpoint links.
- The pre-noon checkpoint milestones were missed and cannot be repaired
  retroactively. The evidence above distinguishes the recovery work from
  compliant pre-noon work.
- A production version should add verified native hook adapters, a broader
  versioned schema registry, and real agent CLI lifecycle tests.
