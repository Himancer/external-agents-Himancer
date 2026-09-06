#!/usr/bin/env bash
set -euo pipefail

AGENT_BIN="${AGENT_BIN:-../entire-agent-universal-memory}"

echo "Universal Agent Memory verification"
echo "Binary: ${AGENT_BIN}"

if ! command -v "${AGENT_BIN}" >/dev/null 2>&1 && [[ ! -x "${AGENT_BIN}" ]]; then
  echo "FAIL: build the adapter first or set AGENT_BIN" >&2
  exit 1
fi

"${AGENT_BIN}" info
"${AGENT_BIN}" detect

echo "WARN: Native Copilot/Codex hooks are not probed by this script."
echo "PASS: Controlled workflow verification is available once capture/handoff is implemented."
