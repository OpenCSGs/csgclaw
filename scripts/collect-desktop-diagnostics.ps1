param(
    [string]$UserDataDirectory = (Join-Path $env:APPDATA "CSGClaw"),
    [string]$OutputDirectory = [Environment]::GetFolderPath([Environment+SpecialFolder]::Desktop),
    [ValidateRange(1, 1440)]
    [int]$EventLookbackMinutes = 180,
    [string]$AgentAPIBaseURL = "http://127.0.0.1:18080",
    [string]$AgentsDirectory = (Join-Path (Join-Path $env:USERPROFILE ".csgclaw") "agents"),
    [ValidateRange(1, 5000)]
    [int]$AgentLogLines = 500,
    [ValidateRange(1, 120)]
    [int]$AgentRequestTimeoutSeconds = 10
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Protect-DiagnosticText {
    param(
        [AllowNull()]
        [AllowEmptyString()]
        [string]$Text
    )

    if ($null -eq $Text) {
        return $null
    }

    $redacted = $Text
    $redacted = $redacted -replace '(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]+', '${1}[REDACTED]'
    $redacted = $redacted -replace '(?i)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|session[_-]?token|password|secret|authorization)\s*[=:]\s*)(?:"[^"]*"|''[^'']*''|[^\s,;]+)', '${1}[REDACTED]'
    $redacted = $redacted -replace '(?i)(--(?:api-key|token|access-token|password)(?:=|\s+))(?:"[^"]*"|[^\s]+)', '${1}[REDACTED]'
    return $redacted
}

function ConvertTo-RedactedDiagnosticValue {
    param(
        [AllowNull()]
        [object]$Value,
        [string]$PropertyName = ""
    )

    if ($null -eq $Value) {
        return $null
    }

    if ($PropertyName -ieq "instructions") {
        return "[OMITTED FROM DIAGNOSTICS]"
    }

    $sensitiveProperty = $PropertyName -match '(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|session[_-]?token|authorization|password|secret|credential|cookie|private[_-]?key)'
    if ($sensitiveProperty -and $PropertyName -notmatch '(?i)_set$') {
        return "[REDACTED]"
    }

    if ($Value -is [string]) {
        return Protect-DiagnosticText -Text $Value
    }

    if ($Value -is [System.Collections.IDictionary]) {
        $result = [ordered]@{}
        foreach ($key in $Value.Keys) {
            $name = [string]$key
            $result[$name] = ConvertTo-RedactedDiagnosticValue -Value $Value[$key] -PropertyName $name
        }
        return $result
    }

    if ($Value -is [System.Management.Automation.PSCustomObject]) {
        $result = [ordered]@{}
        foreach ($property in $Value.PSObject.Properties) {
            if ($PropertyName -match '(?i)^(headers|env)$') {
                $result[$property.Name] = "[REDACTED]"
            }
            else {
                $result[$property.Name] = ConvertTo-RedactedDiagnosticValue -Value $property.Value -PropertyName $property.Name
            }
        }
        return [pscustomobject]$result
    }

    if ($Value -is [System.Collections.IEnumerable] -and $Value -isnot [string]) {
        $items = [Collections.Generic.List[object]]::new()
        foreach ($item in $Value) {
            $items.Add((ConvertTo-RedactedDiagnosticValue -Value $item))
        }
        return ,$items.ToArray()
    }

    return $Value
}

function Get-DiagnosticProperty {
    param(
        [AllowNull()]
        [object]$InputObject,
        [string]$Name
    )

    if ($null -eq $InputObject) {
        return $null
    }
    $property = $InputObject.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }
    return $property.Value
}

function Get-SafeDiagnosticName {
    param([string]$Name)

    $safeName = ([string]$Name) -replace '[^A-Za-z0-9._-]', '_'
    if ([string]::IsNullOrWhiteSpace($safeName)) {
        return "unknown-agent"
    }
    return $safeName
}

function Add-CollectionError {
    param(
        [string]$Area,
        [object]$Failure
    )

    $message = if ($Failure -is [System.Management.Automation.ErrorRecord]) {
        $Failure.Exception.Message
    }
    else {
        [string]$Failure
    }
    $script:collectionErrors.Add("${Area}: $(Protect-DiagnosticText -Text $message)")
}

function Copy-RedactedJsonFile {
    param(
        [string]$Source,
        [string]$Destination,
        [string]$Area
    )

    try {
        $raw = Get-Content -LiteralPath $Source -Raw -ErrorAction Stop
        $parsed = $raw | ConvertFrom-Json -ErrorAction Stop
        $safe = ConvertTo-RedactedDiagnosticValue -Value $parsed
        ConvertTo-Json -InputObject $safe -Depth 32 |
            Set-Content -LiteralPath $Destination -Encoding UTF8
    }
    catch {
        Add-CollectionError -Area $Area -Failure $_
        try {
            $raw = Get-Content -LiteralPath $Source -Raw -ErrorAction Stop
            Protect-DiagnosticText -Text $raw |
                Set-Content -LiteralPath "$Destination.unparsed.txt" -Encoding UTF8
        }
        catch {
            Add-CollectionError -Area "$Area fallback" -Failure $_
        }
    }
}

function Write-RedactedLogTail {
    param(
        [string]$Source,
        [string]$Destination,
        [int]$Lines,
        [string]$Area
    )

    try {
        Get-Content -LiteralPath $Source -Tail $Lines -ErrorAction Stop |
            ForEach-Object { Protect-DiagnosticText -Text ([string]$_) } |
            Set-Content -LiteralPath $Destination -Encoding UTF8
    }
    catch {
        Add-CollectionError -Area $Area -Failure $_
        "Failed to read log: $(Protect-DiagnosticText -Text $_.Exception.Message)" |
            Set-Content -LiteralPath "$Destination.error.txt" -Encoding UTF8
    }
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw "collect-desktop-diagnostics.ps1 is only supported on Windows"
}
if (-not (Test-Path -LiteralPath $UserDataDirectory -PathType Container)) {
    throw "CSGClaw user data directory was not found: $UserDataDirectory"
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$stagingDirectory = Join-Path ([IO.Path]::GetTempPath()) "csgclaw-diagnostics-$timestamp-$PID"
$archivePath = Join-Path $OutputDirectory "csgclaw-diagnostics-$timestamp.zip"
$collectedPaths = [Collections.Generic.List[string]]::new()
$collectionErrors = [Collections.Generic.List[string]]::new()
$installationDirectory = Join-Path $env:LOCALAPPDATA "csgclaw_desktop"
$agentDiagnosticsDirectory = Join-Path $stagingDirectory "agent-data"

New-Item -ItemType Directory -Path $stagingDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $agentDiagnosticsDirectory -Force | Out-Null

Write-Host "Collecting CSGClaw desktop diagnostics..."

try {
    foreach ($name in @(
        "main.log",
        "main.previous.log",
        "backend.log",
        "channel-installer.log",
        "channel-installer.cmd",
        "channel-installer.ready"
    )) {
        $source = Get-ChildItem -LiteralPath $UserDataDirectory -Recurse -File -Filter $name -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending |
            Select-Object -First 1
        if ($null -eq $source) {
            continue
        }
        Copy-Item -LiteralPath $source.FullName -Destination (Join-Path $stagingDirectory $name) -Force
        $collectedPaths.Add($source.FullName)
    }

    $nativeReadyMarker = Get-ChildItem -LiteralPath $UserDataDirectory -Recurse -File -Filter "channel-installer-*.ready" -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
    if ($null -ne $nativeReadyMarker) {
        Copy-Item -LiteralPath $nativeReadyMarker.FullName `
            -Destination (Join-Path $stagingDirectory $nativeReadyMarker.Name) `
            -Force
        $collectedPaths.Add($nativeReadyMarker.FullName)
    }

    $squirrelLog = Join-Path $env:LOCALAPPDATA "SquirrelTemp\SquirrelSetup.log"
    if (Test-Path -LiteralPath $squirrelLog -PathType Leaf) {
        Copy-Item -LiteralPath $squirrelLog `
            -Destination (Join-Path $stagingDirectory "squirrel-setup.log") `
            -Force
        $collectedPaths.Add($squirrelLog)
    }

    if (Test-Path -LiteralPath $installationDirectory -PathType Container) {
        Get-ChildItem -LiteralPath $installationDirectory -File -Filter "Squirrel-*.log" -ErrorAction SilentlyContinue |
            ForEach-Object {
                Copy-Item -LiteralPath $_.FullName `
                    -Destination (Join-Path $stagingDirectory $_.Name) `
                    -Force
                $collectedPaths.Add($_.FullName)
            }
    }

    Get-ChildItem -LiteralPath $UserDataDirectory -Recurse -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Extension -in @(".dmp", ".meta") } |
        ForEach-Object {
            Copy-Item -LiteralPath $_.FullName `
                -Destination (Join-Path $stagingDirectory "crash-$($_.Name)") `
                -Force
            $collectedPaths.Add($_.FullName)
        }

    $processes = @(
        Get-Process -ErrorAction SilentlyContinue |
            Where-Object {
                $_.ProcessName -eq "CSGClaw" -or
                $_.ProcessName -eq "codex" -or
                $_.ProcessName -like "csgclaw-update-helper-*"
            }
    )
    if ($processes.Count -eq 0) {
        "No running CSGClaw, Codex, or desktop update helper process was found." |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "process-status.txt") -Encoding UTF8
    }
    else {
        $processes |
            Select-Object Id, ProcessName, StartTime, CPU, WorkingSet64, Handles, `
                @{Name = "ThreadCount"; Expression = { $_.Threads.Count }}, Path |
            Format-List |
            Out-File -LiteralPath (Join-Path $stagingDirectory "process-status.txt") -Encoding UTF8
    }

    try {
        $runtimeProcesses = @(
            Get-CimInstance Win32_Process -ErrorAction Stop |
                Where-Object {
                    $_.Name -ieq "CSGClaw.exe" -or
                    $_.Name -ieq "csgclaw.exe" -or
                    $_.Name -ieq "codex.exe"
                }
        )
        if ($runtimeProcesses.Count -eq 0) {
            "No CSGClaw or Codex process details were found." |
                Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "process-details.txt") -Encoding UTF8
        }
        else {
            $processDetailLines = [Collections.Generic.List[string]]::new()
            foreach ($process in $runtimeProcesses) {
                $processDetailLines.Add("Name=$($process.Name)")
                $processDetailLines.Add("ProcessId=$($process.ProcessId)")
                $processDetailLines.Add("ParentProcessId=$($process.ParentProcessId)")
                $processDetailLines.Add("CreationDate=$($process.CreationDate)")
                $processDetailLines.Add("ExecutablePath=$($process.ExecutablePath)")
                $processDetailLines.Add("CommandLine=$(Protect-DiagnosticText -Text ([string]$process.CommandLine))")
                $processDetailLines.Add("")
            }
            $processDetailLines |
                Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "process-details.txt") -Encoding UTF8

            $processIDs = @($runtimeProcesses | ForEach-Object { [uint32]$_.ProcessId })
            if ($null -ne (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue)) {
                try {
                    Get-NetTCPConnection -ErrorAction Stop |
                        Where-Object { $processIDs -contains [uint32]$_.OwningProcess } |
                        Select-Object State, LocalAddress, LocalPort, RemoteAddress, RemotePort, OwningProcess |
                        Sort-Object OwningProcess, LocalPort, RemotePort |
                        Format-Table -AutoSize |
                        Out-File -LiteralPath (Join-Path $agentDiagnosticsDirectory "network-connections.txt") -Encoding UTF8
                }
                catch {
                    Add-CollectionError -Area "agent network connections" -Failure $_
                    "Failed to collect network connections: $(Protect-DiagnosticText -Text $_.Exception.Message)" |
                        Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "network-connections.error.txt") -Encoding UTF8
                }
            }
        }
    }
    catch {
        Add-CollectionError -Area "agent process details" -Failure $_
        "Failed to collect process details: $(Protect-DiagnosticText -Text $_.Exception.Message)" |
            Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "process-details.error.txt") -Encoding UTF8
    }

    $agents = @()
    $agentAPIBase = $AgentAPIBaseURL.TrimEnd('/')
    Write-Host "Collecting Agent roster and runtime log tails..."
    try {
        $response = Invoke-WebRequest `
            -Uri "$agentAPIBase/api/v1/agents" `
            -Method Get `
            -UseBasicParsing `
            -TimeoutSec $AgentRequestTimeoutSeconds `
            -ErrorAction Stop
        $agents = @($response.Content | ConvertFrom-Json -ErrorAction Stop)
        $safeAgents = ConvertTo-RedactedDiagnosticValue -Value $agents
        ConvertTo-Json -InputObject $safeAgents -Depth 32 |
            Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "agents.json") -Encoding UTF8
        "Agent API collection succeeded. agent_count=$($agents.Count)" |
            Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "api-status.txt") -Encoding UTF8

        foreach ($agentItem in $agents) {
            $agentID = [string](Get-DiagnosticProperty -InputObject $agentItem -Name "id")
            if ([string]::IsNullOrWhiteSpace($agentID)) {
                Add-CollectionError -Area "agent API logs" -Failure "Agent response did not contain an id"
                continue
            }
            $safeAgentID = Get-SafeDiagnosticName -Name $agentID
            $agentDirectory = Join-Path $agentDiagnosticsDirectory $safeAgentID
            New-Item -ItemType Directory -Path $agentDirectory -Force | Out-Null
            Write-Host "  Agent $agentID"

            try {
                $encodedAgentID = [Uri]::EscapeDataString($agentID)
                $logResponse = Invoke-WebRequest `
                    -Uri "$agentAPIBase/api/v1/agents/$encodedAgentID/logs?lines=$AgentLogLines" `
                    -Method Get `
                    -UseBasicParsing `
                    -TimeoutSec $AgentRequestTimeoutSeconds `
                    -ErrorAction Stop
                Protect-DiagnosticText -Text ([string]$logResponse.Content) |
                    Set-Content -LiteralPath (Join-Path $agentDirectory "runtime-log-tail.txt") -Encoding UTF8
            }
            catch {
                Add-CollectionError -Area "agent $agentID API logs" -Failure $_
                "Failed to collect runtime logs from the local API: $(Protect-DiagnosticText -Text $_.Exception.Message)" |
                    Set-Content -LiteralPath (Join-Path $agentDirectory "runtime-log-tail.error.txt") -Encoding UTF8
            }
        }
    }
    catch {
        Add-CollectionError -Area "agent API" -Failure $_
        "Agent API collection failed. The desktop app may not be running or the local API may be unresponsive.`r`n$(Protect-DiagnosticText -Text $_.Exception.Message)" |
            Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "api-status.txt") -Encoding UTF8
    }

    if (Test-Path -LiteralPath $AgentsDirectory -PathType Container) {
        Get-ChildItem -LiteralPath $AgentsDirectory -Directory -Force -ErrorAction SilentlyContinue |
            ForEach-Object {
                $agentHome = $_
                $safeAgentID = Get-SafeDiagnosticName -Name $agentHome.Name
                $agentDirectory = Join-Path $agentDiagnosticsDirectory $safeAgentID
                New-Item -ItemType Directory -Path $agentDirectory -Force | Out-Null

                @(
                    "agent_home=$($agentHome.FullName)"
                    "created_at=$($agentHome.CreationTime.ToString('o'))"
                    "last_write_at=$($agentHome.LastWriteTime.ToString('o'))"
                ) | Set-Content -LiteralPath (Join-Path $agentDirectory "host-agent-home.txt") -Encoding UTF8

                $codexDirectory = Join-Path $agentHome.FullName ".codex"
                if (-not (Test-Path -LiteralPath $codexDirectory -PathType Container)) {
                    "Codex host state directory was not found: $codexDirectory" |
                        Set-Content -LiteralPath (Join-Path $agentDirectory "codex-file-status.txt") -Encoding UTF8
                }
                else {
                    try {
                        $inventory = [Collections.Generic.List[object]]::new()
                        Get-ChildItem -LiteralPath $codexDirectory -Force -ErrorAction Stop |
                            ForEach-Object { $inventory.Add($_) }
                        $codexHomeDirectory = Join-Path $codexDirectory "home"
                        if (Test-Path -LiteralPath $codexHomeDirectory -PathType Container) {
                            Get-ChildItem -LiteralPath $codexHomeDirectory -Force -ErrorAction Stop |
                                ForEach-Object { $inventory.Add($_) }
                        }
                        $inventory |
                            Select-Object Name, FullName, Length, CreationTime, LastWriteTime, Attributes |
                            Format-List |
                            Out-File -LiteralPath (Join-Path $agentDirectory "codex-file-status.txt") -Encoding UTF8
                    }
                    catch {
                        Add-CollectionError -Area "agent $($agentHome.Name) Codex file inventory" -Failure $_
                    }

                    foreach ($metadataName in @("runtime.json", "session.json")) {
                        $metadataPath = Join-Path $codexDirectory $metadataName
                        if (Test-Path -LiteralPath $metadataPath -PathType Leaf) {
                            Copy-RedactedJsonFile `
                                -Source $metadataPath `
                                -Destination (Join-Path $agentDirectory "codex-$metadataName") `
                                -Area "agent $($agentHome.Name) $metadataName"
                            $collectedPaths.Add($metadataPath)
                        }
                    }

                    $stderrPath = Join-Path (Join-Path $codexDirectory "home") "stderr.log"
                    if (Test-Path -LiteralPath $stderrPath -PathType Leaf) {
                        Write-RedactedLogTail `
                            -Source $stderrPath `
                            -Destination (Join-Path $agentDirectory "codex-stderr-tail.log") `
                            -Lines $AgentLogLines `
                            -Area "agent $($agentHome.Name) Codex stderr"
                        $collectedPaths.Add($stderrPath)
                    }
                }
            }
    }
    else {
        "Agent data directory was not found: $AgentsDirectory" |
            Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "host-agent-data-status.txt") -Encoding UTF8
    }

    @(
        "This directory contains bounded Agent diagnostics for troubleshooting."
        "Agent API base URL: $agentAPIBase"
        "Agent host directory: $AgentsDirectory"
        "Log tail lines: $AgentLogLines"
        "API timeout per request: $AgentRequestTimeoutSeconds seconds"
        ""
        "Not deliberately enumerated: room message storage, workspace files, config.toml, auth files, or full Codex home contents."
        "Runtime and backend logs may still contain text emitted by a runtime. Review the archive before sharing it outside your support channel."
        "Common token, password, API key, authorization, cookie, credential, header, and env values are best-effort redacted from newly collected Agent data."
    ) | Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "README.txt") -Encoding UTF8

    $desktopUpdatesDirectory = Join-Path $UserDataDirectory "desktop-updates"
    if (Test-Path -LiteralPath $desktopUpdatesDirectory -PathType Container) {
        Get-ChildItem -LiteralPath $desktopUpdatesDirectory -Force -ErrorAction SilentlyContinue |
            Select-Object Name, FullName, Length, LastWriteTime, Attributes |
            Format-List |
            Out-File -LiteralPath (Join-Path $stagingDirectory "update-coordinator-status.txt") -Encoding UTF8
    }

    if (Test-Path -LiteralPath $installationDirectory -PathType Container) {
        Get-ChildItem -LiteralPath $installationDirectory -Force -ErrorAction SilentlyContinue |
            Select-Object Name, FullName, Length, LastWriteTime, Attributes |
            Format-List |
            Out-File -LiteralPath (Join-Path $stagingDirectory "installation-status.txt") -Encoding UTF8
    }
    else {
        "CSGClaw Squirrel installation directory was not found: $installationDirectory" |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "installation-status.txt") -Encoding UTF8
    }

    try {
        $events = @(Get-WinEvent -FilterHashtable @{
            LogName = "Application"
            StartTime = (Get-Date).AddMinutes(-$EventLookbackMinutes)
        } -ErrorAction SilentlyContinue | Where-Object { $_.Message -match "CSGClaw" })
        if ($events.Count -eq 0) {
            "No CSGClaw application event was found in the last $EventLookbackMinutes minutes." |
                Set-Content -LiteralPath (Join-Path $stagingDirectory "windows-events.txt") -Encoding UTF8
        }
        else {
            $events |
                Select-Object TimeCreated, Id, ProviderName, LevelDisplayName, Message |
                Format-List |
                Out-File -LiteralPath (Join-Path $stagingDirectory "windows-events.txt") -Encoding UTF8
        }
    }
    catch {
        "Failed to read the Windows Application event log: $($_.Exception.Message)" |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "windows-events.txt") -Encoding UTF8
    }

    @(
        "collected_at=$(Get-Date -Format o)"
        "user_data_directory=$UserDataDirectory"
        "event_lookback_minutes=$EventLookbackMinutes"
        "agent_api_base_url=$agentAPIBase"
        "agents_directory=$AgentsDirectory"
        "agent_log_lines=$AgentLogLines"
        "agent_request_timeout_seconds=$AgentRequestTimeoutSeconds"
        "powershell_version=$($PSVersionTable.PSVersion)"
        "windows_version=$([Environment]::OSVersion.VersionString)"
    ) | Set-Content -LiteralPath (Join-Path $stagingDirectory "diagnostics-info.txt") -Encoding UTF8

    if ($collectedPaths.Count -eq 0) {
        "No desktop log, Crashpad dump, or Agent runtime file was found." |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "collected-paths.txt") -Encoding UTF8
    }
    else {
        $collectedPaths |
            Sort-Object -Unique |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "collected-paths.txt") -Encoding UTF8
    }

    if ($collectionErrors.Count -eq 0) {
        "No collection errors." |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "collection-errors.txt") -Encoding UTF8
    }
    else {
        $collectionErrors |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "collection-errors.txt") -Encoding UTF8
    }

    Compress-Archive -Path (Join-Path $stagingDirectory "*") -DestinationPath $archivePath -Force
}
finally {
    Remove-Item -LiteralPath $stagingDirectory -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "CSGClaw diagnostics archive created: $archivePath"
