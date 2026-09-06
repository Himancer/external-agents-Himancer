# Universal Agent Memory

An Entire external-agent adapter for a deterministic coding-to-deployment
handoff workflow. It stores bounded, redacted session context locally under
`.entire/universal-memory/`; no model API key is required.

## Development

```powershell
..\\..\\.tools\\mise.exe exec -- go build -o entire-agent-universal-memory ./cmd/entire-agent-universal-memory
.\\entire-agent-universal-memory info
..\\..\\.tools\\mise.exe exec -- go test ./...
```

`AGENT.md` documents the protocol mapping and known native-hook limitation.
