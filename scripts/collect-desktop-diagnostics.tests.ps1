# Run with Windows PowerShell 5.1 or pwsh; no Pester/server/runtime dependency.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$sourcePath = Join-Path $PSScriptRoot 'collect-desktop-diagnostics.ps1'
$source = Get-Content -LiteralPath $sourcePath -Raw -Encoding UTF8
$tokens = $null; $parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
foreach ($function in $ast.FindAll({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] }, $false)) {
    . ([scriptblock]::Create($function.Extent.Text))
}
function Assert-True { param([bool]$Condition, [string]$Message) if (-not $Condition) { throw $Message } }
function Put-Fixture { param([string]$Path, [string]$Text)
    New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null
    [IO.File]::WriteAllText($Path, $Text, [Text.UTF8Encoding]::new($false))
}
$fixtureTemp = [IO.Path]::GetTempPath()
if (Test-Path -LiteralPath '/private/tmp' -PathType Container) { $fixtureTemp = '/private/tmp' }
$fixtureRoot = Join-Path $fixtureTemp ('csgclaw-diagnostics-test-' + [Guid]::NewGuid().ToString('N'))
$MaxFileMB = 1; $MaxWorkDataMB = 16
$workBytes = 0L; $workManifest = [Collections.Generic.List[object]]::new()
$collectedPaths = [Collections.Generic.List[string]]::new(); $collectionErrors = [Collections.Generic.List[string]]::new()
$activeIdentities = @{}; $activeRooms = @{}; $workBlobs = @{}
$windowEnd = [DateTimeOffset]::Now; $windowStart = $windowEnd.AddHours(-6)
$recentTime = $windowEnd.AddMinutes(-5).ToString('o'); $oldTime = $windowEnd.AddHours(-8).ToString('o')
$oldLocalAppData = $env:LOCALAPPDATA
try {
    $inputPath = Join-Path $fixtureRoot 'sessions/rollout.jsonl'
    Put-Fixture $inputPath (@(
        (@{type='session_meta'; timestamp=$oldTime; payload=@{base_instructions='saved base policy'}} | ConvertTo-Json -Compress)
        (@{type='turn_context'; timestamp=$oldTime; payload=@{developer_instructions='saved turn policy'}} | ConvertTo-Json -Compress)
        (@{type='response_item'; timestamp=$oldTime; payload=@{content='old unrelated turn'}} | ConvertTo-Json -Compress)
        (@{type='response_item'; timestamp=$recentTime; payload=@{content='recent work'; arguments='{"api_key":"nested-secret"}'; env=@{CUSTOM_AUTH='env-secret'}}} | ConvertTo-Json -Depth 10 -Compress)
        '{"incomplete":'
    ) -join "`n")
    $outputPath = Join-Path $fixtureRoot 'output/rollout.jsonl'
    Export-WorkLog $inputPath $outputPath -JsonLines -RuntimeSession
    $text = Get-Content -LiteralPath $outputPath -Raw
    Assert-True ($text.Contains('saved base policy') -and $text.Contains('saved turn policy') -and $text.Contains('recent work')) 'Missing saved runtime context'
    Assert-True (-not $text.Contains('old unrelated turn')) 'Window included old work'
    Assert-True (-not $text.Contains('nested-secret') -and -not $text.Contains('env-secret')) 'Nested JSON/env secret leaked'
    Assert-True ($workManifest[0].note -match 'invalid_json_lines=1') 'Partial live JSON line was not reported'
    Assert-True ($null -ne (Get-WorkTime '2026-10-09T14:22:28.527805999+08:00')) 'Go nanosecond timestamps unsupported'
    $emptyHeaders = ConvertTo-RedactedDiagnosticValue (ConvertFrom-Json -InputObject '{"headers":{}}')
    Assert-True ($null -ne $emptyHeaders) 'Empty header map failed redaction'

    $dshPath = Join-Path $fixtureRoot 'sessions/dsh.jsonl'
    Put-Fixture $dshPath (@(
        (@{type='session';createdAt=$windowEnd.AddHours(-8).ToUnixTimeMilliseconds();id='dsh-session'} | ConvertTo-Json -Compress)
        '{"type":"message","content":"DSH untimed work"}'
    ) -join "`n")
    Export-WorkLog $dshPath (Join-Path $fixtureRoot 'output/dsh.jsonl') -JsonLines -RuntimeSession
    $text = Get-Content -LiteralPath (Join-Path $fixtureRoot 'output/dsh.jsonl') -Raw
    Assert-True ($text.Contains('DSH untimed work') -and $text.Contains('dsh-session')) 'DSH events incorrectly inherited old header time'

    $configPath = Join-Path $fixtureRoot 'config.toml'
    Put-Fixture $configPath "model = 'fixture-model'`napi_key = 'config-secret'`n[mcp_servers.fixture.http_headers]`nX-Custom = 'header-secret'`n"
    Export-WorkLog $configPath (Join-Path $fixtureRoot 'output/config.toml') -Snapshot
    $text = Get-Content -LiteralPath (Join-Path $fixtureRoot 'output/config.toml') -Raw
    Assert-True ($text.Contains('fixture-model') -and -not $text.Contains('config-secret') -and -not $text.Contains('header-secret')) 'Config redaction lost model or exposed credentials'

    Put-Fixture (Join-Path $fixtureRoot 'big.log') ('x' * (1MB + 1))
    Export-WorkLog (Join-Path $fixtureRoot 'big.log') (Join-Path $fixtureRoot 'output/big.log')
    Assert-True ($workManifest[$workManifest.Count - 1].status -eq 'truncated') 'Missing truncation status'

    # Exercise the complete archive flow with Windows/HTTP probes replaced by local fixtures.
    $dataRoot = Join-Path $fixtureRoot 'data'
    $agentsRoot = Join-Path $dataRoot 'agents'
    $agentHome = Join-Path $agentsRoot 'agent-dev'
    Put-Fixture (Join-Path $dataRoot 'im/diagnostics/turn.json') (@{id='turn'; room_id='room-a'; agent_id='agent-dev'; started_at=$recentTime; total_ms=1000} | ConvertTo-Json)
    Put-Fixture (Join-Path $dataRoot 'im/sessions/room-a.jsonl') (@{id='msg'; sender_id='user-dev'; content='room message'; created_at=$recentTime; blob_ref='blobs/room-a/msg.json'} | ConvertTo-Json -Compress)
    Put-Fixture (Join-Path $dataRoot 'im/sessions/blobs/room-a/msg.json') '{"content":"saved long message"}'
    Put-Fixture (Join-Path $dataRoot 'im/state.json') '{"rooms":[{"id":"room-a","members":["user-dev"],"type":"on_demand"},{"id":"room-unrelated","members":[]}],"users":[{"id":"user-dev","name":"dev"}]}'
    Put-Fixture (Join-Path $dataRoot 'tasks/task-1/tasks.json') '{"tasks":[{"id":"task-1","room_id":"room-a","assigned_to":"pt-dev","body":"do work"}]}'
    Put-Fixture (Join-Path $dataRoot 'tasks/task-1/events.jsonl') (@{created_at=$recentTime; task_id='task-1'; actor_id='pt-manager'; type='dispatched'} | ConvertTo-Json -Compress)
    Put-Fixture (Join-Path $dataRoot 'state.json') '{"agents":{"items":[{"id":"agent-dev","instructions":"active instructions","env":{"CUSTOM":"state-secret"}},{"id":"agent-idle","instructions":"unrelated instructions"}]}}'
    Put-Fixture (Join-Path $agentHome '.codex/home/sessions/rollout.jsonl') (Get-Content -LiteralPath $inputPath -Raw)
    Put-Fixture (Join-Path $agentHome '.codex/home/AGENTS.md') 'current instructions snapshot'
    Put-Fixture (Join-Path $agentHome '.codex/home/config.toml') "model = 'fixture-model'"
    Put-Fixture (Join-Path $agentHome '.codex/home/auth.json') '{"credential":"must-not-copy"}'
    Put-Fixture (Join-Path $agentHome '.codex/runtime.json') '{"agent_id":"agent-dev","api_key":"runtime-secret"}'
    Put-Fixture (Join-Path $agentHome '.dsh/csgclaw-context.patch.yml') 'system_prompt: saved DSH context'
    Put-Fixture (Join-Path $dataRoot 'session-bindings/agent-dev.jsonl') '{"agent_id":"agent-dev","external_session_id":"native-session","conversation_key":"room-a"}'
    Put-Fixture (Join-Path $agentsRoot 'agent-idle/.codex/home/AGENTS.md') 'idle instructions'
    $desktopRoot = Join-Path $fixtureRoot 'desktop'
    Put-Fixture (Join-Path $desktopRoot 'backend.log') "$oldTime old log`n$recentTime current log api_key=log-secret"
    $env:LOCALAPPDATA = Join-Path $fixtureRoot 'localappdata'
    New-Item -ItemType Directory -Path $env:LOCALAPPDATA -Force | Out-Null
    function Get-CimInstance { param($ClassName, $ErrorAction) return }
    function Get-Process { param($ErrorAction) return }
    function Get-WinEvent { param($FilterHashtable, $MaxEvents, $ErrorAction) return }
    function Invoke-WebRequest {
        param($Uri, $Method, [switch]$UseBasicParsing, $TimeoutSec, $ErrorAction)
        if ($Uri -like '*/api/v1/agents') { return [pscustomobject]@{Content='[{"id":"agent-dev","instructions":"API instructions"},{"id":"agent-idle"}]'} }
        return [pscustomobject]@{Content='{"logs":"api_key=api-log-secret"}'}
    }
    $portableSource = $source -replace '(?s)if \(\[Environment\]::OSVersion.Platform -ne \[PlatformID\]::Win32NT\) \{\s*throw "collect-desktop-diagnostics.ps1 is only supported on Windows"\s*\}', ''
    $archiveOutput = Join-Path $fixtureRoot 'archives'
    . ([scriptblock]::Create($portableSource)) -UserDataDirectory $desktopRoot -OutputDirectory $archiveOutput -AgentsDirectory $agentsRoot -MaxFileMB 1 -MaxWorkDataMB 16
    $archives = @(Get-ChildItem -LiteralPath $archiveOutput -Filter '*.zip')
    Assert-True ($archives.Count -eq 1) 'Expected one diagnostics archive'
    $unpacked = Join-Path $fixtureRoot 'unpacked'
    Expand-Archive -LiteralPath $archives[0].FullName -DestinationPath $unpacked
    foreach ($relative in @('recent-work/turns/turn.json', 'recent-work/rooms/room-a.jsonl', 'recent-work/rooms/blobs/room-a/msg.json', 'recent-work/tasks/task-1/tasks.json', 'agent-data/agent-dev/codex-context/home/sessions/rollout.jsonl', 'agent-data/agent-dev/codex-context/home/AGENTS.md', 'agent-data/agent-dev/dsh-context/csgclaw-context.patch.yml', 'agent-data/agent-dev/session-bindings.jsonl', 'work-manifest.json')) {
        Assert-True (Test-Path -LiteralPath (Join-Path $unpacked $relative)) "Archive missing $relative"
    }
    $allText = (Get-ChildItem -LiteralPath $unpacked -File -Recurse | ForEach-Object { Get-Content -LiteralPath $_.FullName -Raw }) -join "`n"
    foreach ($secret in @('state-secret', 'runtime-secret', 'log-secret', 'api-log-secret', 'nested-secret', 'env-secret', 'must-not-copy', 'unrelated instructions')) { Assert-True (-not $allText.Contains($secret)) "Archive exposed $secret" }
    Assert-True (-not (Test-Path (Join-Path $unpacked 'agent-data/agent-idle'))) 'Idle agent work was collected'
    $roster = ConvertFrom-Json -InputObject (Get-Content -LiteralPath (Join-Path $unpacked 'agent-data/agents.json') -Raw)
    Assert-True (@($roster).Count -eq 1 -and $roster[0].id -eq 'agent-dev') 'Agent array shape is nested or incorrect'
    Assert-True ($allText.Contains('active instructions') -and $allText.Contains('saved base policy')) 'Instructions missing from archive'
    Write-Host 'PASS: time window, persisted prompt context, active Agents, archive contents, redaction and truncation'
} finally {
    $env:LOCALAPPDATA = $oldLocalAppData
    Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
}
