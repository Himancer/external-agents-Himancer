[CmdletBinding()]
param(
    [string]$AgentBin
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($AgentBin)) {
    $AgentBin = Join-Path (Split-Path -Parent $PSScriptRoot) "entire-agent-universal-memory.exe"
}

if (-not (Test-Path -LiteralPath $AgentBin -PathType Leaf)) {
    throw "Build the adapter first or pass -AgentBin with its executable path."
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("uam-verify-" + [guid]::NewGuid().ToString("N"))
$fixture = Join-Path $tempRoot "track3.jsonl"
$hadRepoRoot = Test-Path Env:ENTIRE_REPO_ROOT
$previousRepoRoot = $env:ENTIRE_REPO_ROOT

try {
    New-Item -ItemType Directory -Path $tempRoot | Out-Null

    $fixtureContent = @'
{"event":"session_started","session_id":"verify-track3","agent":{"name":"VerifyAgent"}}
{"event":"user_prompt","session_id":"verify-track3","text":"Add safe coupon validation."}
{"event":"file_changed","session_id":"verify-track3","path":"src/coupon.go","change":"modified"}
{"event":"checkpoint_created","session_id":"verify-track3","checkpoint_id":"verify-cp","summary":"Coupon validation tested.","intent":"Add safe coupon validation.","open_questions":["Confirm error ordering."]}
'@
    [System.IO.File]::WriteAllText($fixture, $fixtureContent, [System.Text.UTF8Encoding]::new($false))

    $info = (& $AgentBin info | ConvertFrom-Json)
    if (-not $info.capabilities.transcript_analyzer) {
        throw "The built adapter did not declare transcript_analyzer support."
    }
    $detect = (& $AgentBin detect | ConvertFrom-Json)
    if (-not $detect.present) {
        throw "The built adapter did not report itself as present."
    }

    $env:ENTIRE_REPO_ROOT = (Join-Path $tempRoot "repo")
    $captured = (& $AgentBin capture --transcript $fixture | ConvertFrom-Json)
    if ($captured.handoff_id -ne "verify-track3" -or $captured.context_status -ne "complete") {
        throw "Capture output did not preserve the expected Track 3 context."
    }

    $handedOff = (& $AgentBin handoff --session-id verify-track3 | ConvertFrom-Json)
    if (-not $handedOff.available -or $handedOff.message -notmatch "No deployment has been executed") {
        throw "Handoff was not a safe review-only result."
    }

    Write-Host "PASS: controlled capture-to-handoff workflow verified."
    Write-Host "NOTE: native Codex/Copilot lifecycle hooks are intentionally not claimed by this adapter."
}
finally {
    if ($hadRepoRoot) {
        $env:ENTIRE_REPO_ROOT = $previousRepoRoot
    }
    else {
        Remove-Item Env:ENTIRE_REPO_ROOT -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}
