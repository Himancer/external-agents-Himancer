#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_BIN="${AGENT_BIN:-${SCRIPT_DIR}/../entire-agent-universal-memory}"

if [[ ! -x "${AGENT_BIN}" ]]; then
  echo "FAIL: build the adapter first or set AGENT_BIN to its executable path" >&2
  exit 1
fi

TEMP_ROOT="$(mktemp -d)"
trap 'rm -rf "${TEMP_ROOT}"' EXIT

FIXTURE="${TEMP_ROOT}/track3.jsonl"
RUNTIME_ROOT="${TEMP_ROOT}/repo"

printf '%s
'   '{"event":"session_started","session_id":"verify-track3","agent":{"name":"VerifyAgent"}}'   '{"event":"user_prompt","session_id":"verify-track3","text":"Add safe coupon validation."}'   '{"event":"file_changed","session_id":"verify-track3","path":"src/coupon.go","change":"modified"}'   '{"event":"checkpoint_created","session_id":"verify-track3","checkpoint_id":"verify-cp","summary":"Coupon validation tested.","intent":"Add safe coupon validation.","open_questions":["Confirm error ordering."]}'   > "${FIXTURE}"

"${AGENT_BIN}" info | grep -q '"transcript_analyzer":true'
"${AGENT_BIN}" detect | grep -q '"present":true'

CAPTURED="$(ENTIRE_REPO_ROOT="${RUNTIME_ROOT}" "${AGENT_BIN}" capture --transcript "${FIXTURE}")"
printf '%s' "${CAPTURED}" | grep -q '"handoff_id":"verify-track3"'
printf '%s' "${CAPTURED}" | grep -q '"context_status":"complete"'

HANDED_OFF="$(ENTIRE_REPO_ROOT="${RUNTIME_ROOT}" "${AGENT_BIN}" handoff --session-id verify-track3)"
printf '%s' "${HANDED_OFF}" | grep -q '"available":true'
printf '%s' "${HANDED_OFF}" | grep -q 'No deployment has been executed'

echo "PASS: legacy/event-capable controlled capture-to-handoff workflow verified."
echo "NOTE: native Codex/Copilot lifecycle hooks are intentionally not claimed by this adapter."
