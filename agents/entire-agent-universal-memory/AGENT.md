# Universal Agent Memory — External Agent Research

## Verdict: COMPATIBLE

Universal Agent Memory is a controlled, file-backed developer workflow rather than an adapter for an existing third-party IDE. Its CLI captures a coding handoff and exposes the resulting session through Entire's external-agent protocol. This makes the cross-agent handoff deterministic, locally testable, and usable without an API key.

## Static Checks

| Check | Result | Notes |
| --- | --- | --- |
| Adapter binary | PENDING | Built as `entire-agent-universal-memory`. |
| Protocol documentation | PASS | Entire external-agent protocol v1 was reviewed. |
| Entire CLI | PASS | Available on `PATH`. |
| Native third-party hook source | NOT APPLICABLE | The workflow owns its capture command; it does not claim unverified Copilot or Codex hooks. |

## Binary

- Name: `entire-agent-universal-memory`
- Protocol version: `1`
- Install: build the module, put the executable on an absolute `PATH` entry, and enable external agents in Entire's local settings.

## Workflow and Hook Mechanism

The companion workflow writes structured coding handoffs to a repository-local runtime directory. `capture` is the coding-side boundary; `handoff` is the deployment-side boundary. A future hook wrapper can call `capture` on a supported native agent event, but native hooks are not declared until they are verified on this machine.

| Workflow action | Entire concept | Status |
| --- | --- | --- |
| `capture` | Session/turn data is persisted | Planned |
| `handoff` | Deployment agent reads the latest bounded context | Planned |
| Native IDE hook | `parse-hook`/`install-hooks` | Not declared; requires real payload capture |

## Session Management

- Session directory: `<repo>/.entire/universal-memory/sessions/`
- Session ID source: explicit `--session-id` supplied by the workflow; a generated ID is allowed for local demos.
- Session format: JSONL with a bounded, redacted handoff packet.
- Runtime state: never committed. The adapter creates it only below the repository supplied by `ENTIRE_REPO_ROOT` or `--repo-path`.

## Transcript

- Location: `<session directory>/<session-id>.jsonl`
- Format: JSONL with `role`, `content`, and timestamp fields.
- User prompt field: `content` where `role` is `user`.
- Modified files: optional string array in session metadata.
- Token usage: not available in deterministic mode.

## Protocol Mapping

| Subcommand | Native concept | Implementation notes | Feasibility |
| --- | --- | --- | --- |
| `info`, `detect` | Static workflow metadata | Declares protocol v1 and only implemented capabilities | Required |
| Session helpers | File-backed session store | Resolve safe paths below the runtime session directory | Required |
| `read-session`, `write-session` | Handoff envelope | Preserve opaque transcript bytes and metadata | Required |
| Transcript chunk/reassemble | Raw JSONL bytes | Base64 chunks, deterministic round trip | Required |
| Resume command | `universal-memory handoff` | Names the session explicitly | Required |
| Hooks | Controlled workflow events | Deferred until native source payloads are verified | Not declared initially |
| Transcript analyzer | JSONL records | Extract prompts and summary after core compliance works | Planned |

## Selected Capabilities

| Capability | Declared initially | Justification |
| --- | --- | --- |
| hooks | false | No unverified native hook integration. |
| transcript_analyzer | false | Added only after core protocol compliance. |
| transcript_preparer | false | JSONL needs no conversion. |
| token_calculator | false | Deterministic mode has no provider token data. |
| text_generator | false | OpenAI is optional and must not be required for the workflow. |
| hook_response_writer | false | No native hook response transport. |
| subagent_aware_extractor | false | Not part of the MVP. |

## Data Safety

- Handoff data is length-bounded before persistence.
- Obvious bearer tokens, API keys, and credentials are redacted before write.
- Missing context produces an explicit missing-context result; it never fabricates a deployment or a prior decision.
- Raw prompts can contain sensitive data, so the runtime directory is ignored and no sample secret is committed.

## Captured Payloads

- Verification script: `scripts/verify-universal-memory.sh`
- Status: UNVERIFIED for native IDE hooks. The script validates the controlled CLI workflow and records the exact limitation instead of pretending a native hook was observed.

## E2E Test Prerequisites

- Entire CLI: `entire` on `PATH`.
- Adapter binary: built from this module and exposed on an absolute `PATH`.
- Native IDE runtime: not required for deterministic protocol/unit tests.
- Interactive lifecycle tests: blocked until a real agent CLI and `tmux` are available on this Windows host.
