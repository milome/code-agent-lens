param(
  [switch]$ValidateOnly,
  [switch]$StartServer,
  [int]$Port = 3000,
  [string]$Fixture,
  [string]$Endpoint = "http://127.0.0.1:3010/v1/responses",
  [string]$DumpDir = "./.tmp/observability-smoke",
  [switch]$ValidateDump,
  [object]$CaptureSecrets = $false,
  [switch]$RunClaudeCode,
  [switch]$RunCodexCli,
  [string]$Prompt = "",
  [string]$BaseURL = "http://127.0.0.1:3010",
  [string]$WireAPI = "responses",
  [string]$OtlpEndpoint = "http://127.0.0.1:4318",
  [string]$TraceIDOut = "./.tmp/observability-smoke/trace-id.txt",
  [switch]$ValidateTrace,
  [string]$TraceID = "",
  [string]$TraceIDFile = "",
  [string]$SpanName = "",
  [string]$CollectorEvidence = "./.tmp/observability-smoke/collector-evidence.jsonl",
  [string]$JaegerURL = "http://127.0.0.1:16686"
)

$ErrorActionPreference = "Stop"

if (!$PSBoundParameters.ContainsKey("Endpoint")) {
  $Endpoint = "http://127.0.0.1:$Port/v1/responses"
}
if (!$PSBoundParameters.ContainsKey("BaseURL")) {
  $BaseURL = "http://127.0.0.1:$Port"
}

function Resolve-RepoPath([string]$PathValue) {
  if ([System.IO.Path]::IsPathRooted($PathValue)) {
    return $PathValue
  }
  return [System.IO.Path]::GetFullPath((Join-Path (Get-Location) $PathValue))
}

function ConvertTo-SmokeBool([object]$Value) {
  if ($Value -is [bool]) {
    return $Value
  }
  $text = ([string]$Value).Trim().ToLowerInvariant()
  return $text -in @("1", "true", "yes", "on")
}

function Set-ObservabilityEnv {
  param([string]$Root, [object]$Secrets)
  $secretsEnabled = ConvertTo-SmokeBool $Secrets
  $env:CODE_AGENT_LENS_OTEL_ENABLED = "true"
  $env:CODE_AGENT_LENS_OBS_LOCAL_DEBUG = "true"
  $env:CODE_AGENT_LENS_OBS_DUMP_ENABLED = "true"
  $env:CODE_AGENT_LENS_OBS_DUMP_DIR = (Resolve-RepoPath $Root)
  $env:CODE_AGENT_LENS_OBS_VIEWER_ENABLED = "true"
  $env:CODE_AGENT_LENS_OBS_VIEWER_PUBLIC_URL = "http://127.0.0.1:$Port/debug/obs"
  $env:CODE_AGENT_LENS_OBS_CAPTURE_HEADERS = "all"
  $env:CODE_AGENT_LENS_OBS_CAPTURE_BODIES = "all"
  $env:CODE_AGENT_LENS_OBS_CAPTURE_STREAM_EVENTS = "all"
  $env:CODE_AGENT_LENS_OBS_CAPTURE_SECRETS = if ($secretsEnabled) { "true" } else { "false" }
  $env:CODE_AGENT_LENS_OBS_PROMPT_EXTRACT = "true"
  $env:CODE_AGENT_LENS_OBS_MAX_BODY_BYTES = "0"
  $env:CODE_AGENT_LENS_OBS_OTEL_PROMPT_MODE = "preview"
  $env:CODE_AGENT_LENS_OBS_OTEL_PROMPT_PREVIEW_BYTES = "256"
  $env:OTEL_EXPORTER_OTLP_ENDPOINT = $OtlpEndpoint
}

function Assert-SmokeFixtures {
  $fixtures = @(
    "internal/observability/testdata/claude_request.json",
    "internal/observability/testdata/openai_chat_request.json",
    "internal/observability/testdata/openai_responses_request.json"
  )
  foreach ($item in $fixtures) {
    if (!(Test-Path -LiteralPath $item)) {
      throw "missing fixture: $item"
    }
    $text = Get-Content -Raw -LiteralPath $item
    foreach ($literal in @("system prompt", "developer prompt", "user prompt")) {
      if (!$text.Contains($literal)) {
        throw "$item missing $literal"
      }
    }
  }
}

function Test-Dump {
  param([string]$Root)
  $rootPath = Resolve-RepoPath $Root
  $requests = Get-ChildItem -LiteralPath $rootPath -Recurse -Filter "requests.jsonl" -ErrorAction SilentlyContinue
  $indexes = Get-ChildItem -LiteralPath $rootPath -Recurse -Filter "prompt.index.json" -ErrorAction SilentlyContinue
  if ($requests.Count -lt 1) {
    throw "requests.jsonl not found under $rootPath"
  }
  if ($indexes.Count -lt 1) {
    throw "prompt.index.json not found under $rootPath"
  }
  $required = @("ingress.request.body.raw", "upstream.request.body.raw", "upstream.response.body.raw", "usage.json")
  foreach ($name in $required) {
    if ((Get-ChildItem -LiteralPath $rootPath -Recurse -Filter $name -ErrorAction SilentlyContinue).Count -lt 1) {
      throw "$name not found under $rootPath"
    }
  }
  Write-Host "dump validation passed root=$rootPath"
}

function Invoke-FixtureCurl {
  param([string]$FixturePath, [string]$Url)
  if (!(Test-Path -LiteralPath $FixturePath)) {
    throw "fixture missing: $FixturePath"
  }
  Write-Host "curl.exe -fsS -X POST $Url -H 'Content-Type: application/json' --data-binary '@$FixturePath'"
  curl.exe -fsS -X POST $Url -H "Content-Type: application/json" --data-binary "@$FixturePath"
}

function Get-LatestRequestAfter {
  param([string]$Root, [datetime]$StartedAtUtc, [string]$Scenario)
  $rootPath = Resolve-RepoPath $Root
  $latest = Get-ChildItem -LiteralPath $rootPath -Recurse -Filter "requests.jsonl" -ErrorAction SilentlyContinue |
    Where-Object { $_.LastWriteTimeUtc -ge $StartedAtUtc.AddSeconds(-1) } |
    Sort-Object LastWriteTimeUtc -Descending |
    Select-Object -First 1
  if ($null -eq $latest) {
    throw "no new requests.jsonl found after $Scenario"
  }
  $line = Get-Content -LiteralPath $latest.FullName | Select-Object -Last 1
  if (!$line) {
    throw "empty requests.jsonl found after $Scenario"
  }
  return ($line | ConvertFrom-Json)
}

function Start-SmokeServer {
  param([string]$Root)
  $pidPath = Resolve-RepoPath (Join-Path $Root "code-agent-lens-server.pid")
  New-Item -ItemType Directory -Force (Split-Path -Parent $pidPath) | Out-Null
  $listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($listener) {
    $existing = Get-Process -Id $listener.OwningProcess -ErrorAction SilentlyContinue
    if ($existing -and $existing.ProcessName -ne "code-agent-lens-server") {
      throw "port $Port is already owned by pid=$($existing.Id) process=$($existing.ProcessName); stop that process before running M001"
    }
  }
  if (Test-Path -LiteralPath $pidPath) {
    $oldPid = (Get-Content -Raw -LiteralPath $pidPath).Trim()
    if ($oldPid) {
      $oldProcess = Get-Process -Id ([int]$oldPid) -ErrorAction SilentlyContinue
      if ($oldProcess) {
        Write-Host "smoke server already running pid=$oldPid"
        return
      }
    }
  }
  $exe = Resolve-RepoPath ".tmp/observability-smoke/code-agent-lens-server.exe"
  New-Item -ItemType Directory -Force (Split-Path -Parent $exe) | Out-Null
  go build -o $exe ./cmd/server
  $env:CODE_AGENT_LENS_DATA_DIR = Resolve-RepoPath (Join-Path $Root "data")
  $env:CODE_AGENT_LENS_DB_PATH = Resolve-RepoPath (Join-Path $Root "data/code-agent-lens.db")
  $env:CODE_AGENT_LENS_PORT = "$Port"
  $env:CODE_AGENT_LENS_BASIC_AUTH_ENABLED = "false"
  $env:CODE_AGENT_LENS_OBS_SMOKE_UPSTREAM = "true"
  $process = Start-Process -FilePath $exe -PassThru -WindowStyle Hidden
  $process.Id | Set-Content -LiteralPath $pidPath -Encoding utf8
  for ($i = 0; $i -lt 50; $i++) {
    try {
      $health = curl.exe -fsS "http://127.0.0.1:$Port/health"
      if ($LASTEXITCODE -eq 0) {
        Write-Host "smoke server started pid=$($process.Id)"
        return
      }
    } catch {
      Start-Sleep -Milliseconds 200
    }
  }
  throw "smoke server did not become healthy"
}

function Invoke-ClaudeCodeSmoke {
  Set-ObservabilityEnv -Root $DumpDir -Secrets:$CaptureSecrets
  $env:CLAUDE_CODE_ENABLE_TELEMETRY = "1"
  $env:CLAUDE_CODE_ENHANCED_TELEMETRY_BETA = "1"
  $env:CLAUDE_CODE_PROPAGATE_TRACEPARENT = "1"
  $env:ANTHROPIC_BASE_URL = $BaseURL
  $env:OTEL_EXPORTER_OTLP_ENDPOINT = $OtlpEndpoint
  $env:OTEL_LOG_RAW_API_BODIES = "file:D:/code-agent-lens-debug/claude-otel-api-bodies"
  $outPath = Resolve-RepoPath $TraceIDOut
  New-Item -ItemType Directory -Force (Split-Path -Parent $outPath) | Out-Null
  if (!(Get-Command claude -ErrorAction SilentlyContinue)) {
    throw "claude CLI not found"
  }
  $startedAtUtc = [DateTime]::UtcNow
  claude --print $Prompt
  if ($LASTEXITCODE -ne 0) {
    throw "claude CLI exited with code $LASTEXITCODE"
  }
  $last = Get-LatestRequestAfter -Root $DumpDir -StartedAtUtc $startedAtUtc -Scenario "Claude Code smoke"
  $last.trace_id | Set-Content -LiteralPath $outPath -Encoding utf8
  Write-Host "trace id written to $outPath"
}

function Invoke-CodexSmoke {
  Set-ObservabilityEnv -Root $DumpDir -Secrets:$CaptureSecrets
  $codexDir = Join-Path $HOME ".codex"
  $configPath = Join-Path $codexDir "config.toml"
  $backupPath = Join-Path $codexDir "config.toml.code-agent-lens-observability-bak"
  New-Item -ItemType Directory -Force $codexDir | Out-Null
  $hadConfig = Test-Path -LiteralPath $configPath
  if ($hadConfig) {
    Copy-Item -LiteralPath $configPath -Destination $backupPath -Force
  }
  try {
    $toml = @"
model_provider = "CodeAgentLens"
model = "gpt-5"

[model_providers.CodeAgentLens]
name = "CodeAgentLens"
base_url = "$BaseURL"
wire_api = "$WireAPI"
env_key = "CODE_AGENT_LENS_SMOKE_API_KEY"
"@
    $toml | Set-Content -LiteralPath $configPath -Encoding utf8
    $env:CODE_AGENT_LENS_SMOKE_API_KEY = "local-smoke-placeholder"
    if (!(Get-Command codex -ErrorAction SilentlyContinue)) {
      throw "codex CLI not found"
    }
    $startedAtUtc = [DateTime]::UtcNow
    codex exec $Prompt
    if ($LASTEXITCODE -ne 0) {
      throw "codex CLI exited with code $LASTEXITCODE"
    }
    $last = Get-LatestRequestAfter -Root $DumpDir -StartedAtUtc $startedAtUtc -Scenario "Codex CLI smoke"
    Write-Host "codex smoke request trace_id=$($last.trace_id) request_id=$($last.request_id)"
  }
  finally {
    if ($hadConfig) {
      Move-Item -LiteralPath $backupPath -Destination $configPath -Force
    } else {
      Remove-Item -LiteralPath $configPath -Force -ErrorAction SilentlyContinue
      Remove-Item -LiteralPath $backupPath -Force -ErrorAction SilentlyContinue
    }
  }
}

function Test-TraceEvidence {
  $id = $TraceID
  if ($TraceIDFile) {
    $id = (Get-Content -Raw -LiteralPath (Resolve-RepoPath $TraceIDFile)).Trim()
  }
  if (!$id) {
    throw "TraceID or TraceIDFile is required"
  }
  if (!$SpanName) {
    throw "SpanName is required"
  }
  $evidencePath = Resolve-RepoPath $CollectorEvidence
  if (Test-Path -LiteralPath $evidencePath) {
    $text = Get-Content -Raw -LiteralPath $evidencePath
    if (!$text.Contains($id) -or !$text.Contains($SpanName)) {
      throw "trace evidence missing trace_id=$id span=$SpanName"
    }
    Write-Host "trace validation passed trace_id=$id span=$SpanName source=$evidencePath"
    return
  }

  $jaegerEndpoint = "$($JaegerURL.TrimEnd('/'))/api/traces/$id"
  for ($i = 0; $i -lt 10; $i++) {
    $response = ""
    try {
      $response = curl.exe -fsS $jaegerEndpoint
    } catch {
      $response = ""
    }
    if ($LASTEXITCODE -eq 0 -and $response.Contains($id) -and $response.Contains($SpanName)) {
      Write-Host "trace validation passed trace_id=$id span=$SpanName source=$jaegerEndpoint"
      return
    }
    Start-Sleep -Milliseconds 500
  }
  throw "trace evidence missing trace_id=$id span=$SpanName in collector evidence $evidencePath and Jaeger $jaegerEndpoint"
}

if ($ValidateOnly) {
  Assert-SmokeFixtures
  $scriptText = Get-Content -Raw -LiteralPath $PSCommandPath
  foreach ($literal in @("-RunClaudeCode", "-RunCodexCli", "-ValidateTrace", "code-agent-lens-otel-smoke-claude", "code-agent-lens-otel-smoke-codex", "CLAUDE_CODE_PROPAGATE_TRACEPARENT", "JaegerURL")) {
    if (!$scriptText.Contains($literal)) {
      throw "smoke script missing $literal"
    }
  }
  Write-Host "observability smoke validate-only passed"
  exit 0
}

Set-ObservabilityEnv -Root $DumpDir -Secrets:$CaptureSecrets

if ($StartServer) {
  Start-SmokeServer -Root $DumpDir
}
if ($Fixture) {
  Invoke-FixtureCurl -FixturePath $Fixture -Url $Endpoint
}
if ($ValidateDump) {
  Test-Dump -Root $DumpDir
}
if ($RunClaudeCode) {
  if (!$Prompt) { $Prompt = "Return the string code-agent-lens-otel-smoke-claude" }
  Invoke-ClaudeCodeSmoke
}
if ($RunCodexCli) {
  if (!$Prompt) { $Prompt = "Return the string code-agent-lens-otel-smoke-codex" }
  Invoke-CodexSmoke
}
if ($ValidateTrace) {
  Test-TraceEvidence
}
