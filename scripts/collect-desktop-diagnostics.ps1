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
    [int]$AgentRequestTimeoutSeconds = 10,
    [ValidateRange(1, 100000)]
    [int]$DesktopLogLines = 10000
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
    $redacted = $redacted -replace '(?i)("(?:api[_-]?key|access[_-]?key(?:[_-]?(?:id|secret))?|access[_-]?token|refresh[_-]?token|session[_-]?token|token|password|secret|authorization|cookie)"\s*:\s*)"(?:\\.|[^"\\])*"', '${1}"[REDACTED]"'
    $redacted = $redacted -replace '(?i)((?:api[_-]?key|access[_-]?key(?:[_-]?(?:id|secret))?|access[_-]?token|refresh[_-]?token|session[_-]?token|token|password|secret|authorization|cookie)\s*[=:]\s*)(?:"[^"]*"|''[^'']*''|[^\s,;]+)', '${1}[REDACTED]'
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

    if ($PropertyName -match '(?i)^(instructions|headers|env|environment|runtime_credentials)$') {
        return "[OMITTED FROM DIAGNOSTICS]"
    }

    $sensitiveProperty = $PropertyName -match '(?i)(api[_-]?key|access[_-]?key|access[_-]?token|refresh[_-]?token|session[_-]?token|authorization|password|secret|credential|cookie|private[_-]?key|^token$)'
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
            $result[$property.Name] = ConvertTo-RedactedDiagnosticValue -Value $property.Value -PropertyName $property.Name
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
    if ([string]::IsNullOrWhiteSpace($safeName) -or $safeName -in @(".", "..")) {
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
        $raw = Get-Content -LiteralPath $Source -Raw -Encoding UTF8 -ErrorAction Stop
        $parsed = $raw | ConvertFrom-Json -ErrorAction Stop
        $safe = ConvertTo-RedactedDiagnosticValue -Value $parsed
        ConvertTo-Json -InputObject $safe -Depth 32 |
            Set-Content -LiteralPath $Destination -Encoding UTF8
    }
    catch {
        Add-CollectionError -Area $Area -Failure $_
        "JSON could not be parsed; the raw file was omitted to avoid exposing credentials." |
            Set-Content -LiteralPath "$Destination.error.txt" -Encoding UTF8
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
        Get-Content -LiteralPath $Source -Tail $Lines -Encoding UTF8 -ErrorAction Stop |
            ForEach-Object { Protect-DiagnosticText -Text ([string]$_) } |
            Set-Content -LiteralPath $Destination -Encoding UTF8
    }
    catch {
        Add-CollectionError -Area $Area -Failure $_
        "Failed to read log: $(Protect-DiagnosticText -Text $_.Exception.Message)" |
            Set-Content -LiteralPath "$Destination.error.txt" -Encoding UTF8
    }
}

function Collect-HostRuntimeFiles {
    param(
        [string]$AgentHome,
        [string]$Destination,
        [ValidateSet("codex", "dsh")]
        [string]$Kind
    )

    $runtimeDirectory = Join-Path $AgentHome ".$Kind"
    $statusPath = Join-Path $Destination "$Kind-file-status.txt"
    if (-not (Test-Path -LiteralPath $runtimeDirectory -PathType Container)) {
        "Runtime state directory was not found: $runtimeDirectory" |
            Set-Content -LiteralPath $statusPath -Encoding UTF8
        return
    }

    try {
        $inventory = @(
            Get-ChildItem -LiteralPath $runtimeDirectory -Force -ErrorAction Stop
            $runtimeHome = Join-Path $runtimeDirectory "home"
            if (Test-Path -LiteralPath $runtimeHome -PathType Container) {
                Get-ChildItem -LiteralPath $runtimeHome -Force -ErrorAction Stop
            }
        )
        $inventory |
            Select-Object Name, FullName, Length, CreationTime, LastWriteTime, Attributes |
            Format-List |
            Out-File -LiteralPath $statusPath -Encoding UTF8
    }
    catch {
        Add-CollectionError -Area "agent $AgentHome $Kind inventory" -Failure $_
    }

    foreach ($metadataName in @("runtime.json", "session.json")) {
        $metadataPath = Join-Path $runtimeDirectory $metadataName
        if (Test-Path -LiteralPath $metadataPath -PathType Leaf) {
            Copy-RedactedJsonFile -Source $metadataPath `
                -Destination (Join-Path $Destination "$Kind-$metadataName") `
                -Area "agent $AgentHome $Kind $metadataName"
            $script:collectedPaths.Add($metadataPath)
        }
    }

    $stderrPath = if ($Kind -eq "dsh") {
        Join-Path $runtimeDirectory "stderr.log"
    }
    else {
        Join-Path (Join-Path $runtimeDirectory "home") "stderr.log"
    }
    if (Test-Path -LiteralPath $stderrPath -PathType Leaf) {
        Write-RedactedLogTail -Source $stderrPath `
            -Destination (Join-Path $Destination "$Kind-stderr-tail.log") `
            -Lines $AgentLogLines -Area "agent $AgentHome $Kind stderr"
        $script:collectedPaths.Add($stderrPath)
    }
}

function Write-DiagnosticEvents {
    param(
        [string]$LogName,
        [string]$Destination,
        [string]$MessagePattern = "",
        [string[]]$ProviderNames = @()
    )

    try {
        $filter = @{
            LogName = $LogName
            StartTime = (Get-Date).AddMinutes(-$EventLookbackMinutes)
        }
        if ($ProviderNames.Count -gt 0) {
            $filter.ProviderName = $ProviderNames
        }
        $events = @(Get-WinEvent -FilterHashtable $filter -MaxEvents 2000 -ErrorAction Stop |
            Where-Object { $MessagePattern -eq "" -or $_.Message -match $MessagePattern })
        if ($events.Count -eq 0) {
            "No matching $LogName event was found in the last $EventLookbackMinutes minutes." |
                Set-Content -LiteralPath $Destination -Encoding UTF8
        }
        else {
            $events |
                Select-Object TimeCreated, Id, ProviderName, LevelDisplayName, Message |
                Format-List | Out-String -Width 240 |
                ForEach-Object { Protect-DiagnosticText -Text $_ } |
                Set-Content -LiteralPath $Destination -Encoding UTF8
        }
    }
    catch {
        if ($_.FullyQualifiedErrorId -like "NoMatchingEventsFound*") {
            "No matching $LogName event was found in the last $EventLookbackMinutes minutes." |
                Set-Content -LiteralPath $Destination -Encoding UTF8
        }
        else {
            Add-CollectionError -Area "$LogName events" -Failure $_
            "Failed to read events: $(Protect-DiagnosticText -Text $_.Exception.Message)" |
                Set-Content -LiteralPath $Destination -Encoding UTF8
        }
    }
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw "collect-desktop-diagnostics.ps1 is only supported on Windows"
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$stagingDirectory = Join-Path ([IO.Path]::GetTempPath()) "csgclaw-diagnostics-$timestamp-$PID"
$archivePath = Join-Path $OutputDirectory "csgclaw-diagnostics.zip"
$temporaryArchivePath = Join-Path $OutputDirectory ".csgclaw-diagnostics-$timestamp-$PID.zip"
$collectedPaths = [Collections.Generic.List[string]]::new()
$collectionErrors = [Collections.Generic.List[string]]::new()
$installationDirectory = Join-Path $env:LOCALAPPDATA "csgclaw_desktop"
$agentDiagnosticsDirectory = Join-Path $stagingDirectory "agent-data"

New-Item -ItemType Directory -Path $stagingDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $agentDiagnosticsDirectory -Force | Out-Null

Write-Host "Collecting CSGClaw desktop diagnostics..."

try {
    if (-not (Test-Path -LiteralPath $UserDataDirectory -PathType Container)) {
        Add-CollectionError -Area "desktop user data" -Failure "Directory was not found: $UserDataDirectory"
    }
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
        Write-RedactedLogTail -Source $source.FullName `
            -Destination (Join-Path $stagingDirectory $name) -Lines $DesktopLogLines `
            -Area "desktop $name"
        $collectedPaths.Add($source.FullName)
    }

    $nativeReadyMarker = Get-ChildItem -LiteralPath $UserDataDirectory -Recurse -File -Filter "channel-installer-*.ready" -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
    if ($null -ne $nativeReadyMarker) {
        Write-RedactedLogTail -Source $nativeReadyMarker.FullName `
            -Destination (Join-Path $stagingDirectory $nativeReadyMarker.Name) `
            -Lines $DesktopLogLines -Area "desktop installer ready marker"
        $collectedPaths.Add($nativeReadyMarker.FullName)
    }

    $squirrelLog = Join-Path $env:LOCALAPPDATA "SquirrelTemp\SquirrelSetup.log"
    if (Test-Path -LiteralPath $squirrelLog -PathType Leaf) {
        Write-RedactedLogTail -Source $squirrelLog `
            -Destination (Join-Path $stagingDirectory "squirrel-setup.log") `
            -Lines $DesktopLogLines -Area "Squirrel setup log"
        $collectedPaths.Add($squirrelLog)
    }

    if (Test-Path -LiteralPath $installationDirectory -PathType Container) {
        Get-ChildItem -LiteralPath $installationDirectory -File -Filter "Squirrel-*.log" -ErrorAction SilentlyContinue |
            ForEach-Object {
                Write-RedactedLogTail -Source $_.FullName `
                    -Destination (Join-Path $stagingDirectory $_.Name) `
                    -Lines $DesktopLogLines -Area "Squirrel installation log"
                $collectedPaths.Add($_.FullName)
            }
    }

    Get-ChildItem -LiteralPath $UserDataDirectory -Recurse -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Extension -in @(".dmp", ".meta") } |
        ForEach-Object {
            try {
                Copy-Item -LiteralPath $_.FullName `
                    -Destination (Join-Path $stagingDirectory "crash-$($_.Name)") `
                    -Force
                $collectedPaths.Add($_.FullName)
            }
            catch {
                Add-CollectionError -Area "Crashpad dump $($_.TargetObject)" -Failure $_
            }
        }

    $runtimeProcesses = @()
    try {
        $allProcesses = @(Get-CimInstance Win32_Process -ErrorAction Stop)
        $selectedIDs = @{}
        foreach ($process in $allProcesses) {
            if ($process.Name -ieq "CSGClaw.exe" -or
                $process.Name -ieq "codex.exe" -or
                $process.Name -like "csgclaw-update-helper-*" -or
                $process.Name -ieq "dsh.exe" -or
                $process.CommandLine -match '(?i)(deepseek-harness|[\\/]@deepseek-ai[\\/]dsh[\\/]|[\\/]\.dsh[\\/]|[\\/]dsh\.cmd(?:"|\s|$))') {
                $selectedIDs[[uint32]$process.ProcessId] = $true
            }
        }
        # Follow descendants so the DSH cmd.exe -> node.exe process chain is included.
        do {
            $addedProcess = $false
            foreach ($process in $allProcesses) {
                $processID = [uint32]$process.ProcessId
                if (-not $selectedIDs.ContainsKey($processID) -and
                    $selectedIDs.ContainsKey([uint32]$process.ParentProcessId)) {
                    $selectedIDs[$processID] = $true
                    $addedProcess = $true
                }
            }
        } while ($addedProcess)
        $runtimeProcesses = @($allProcesses | Where-Object {
            $selectedIDs.ContainsKey([uint32]$_.ProcessId)
        })

        if ($runtimeProcesses.Count -eq 0) {
            "No CSGClaw, Codex, DSH, or related process was found." |
                Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "process-details.txt") -Encoding UTF8
        }
        else {
            $runtimeProcesses |
                Select-Object Name, ProcessId, ParentProcessId, CreationDate, ExecutablePath,
                    @{Name = "CommandLine"; Expression = { Protect-DiagnosticText -Text ([string]$_.CommandLine) }} |
                Format-List | Out-String -Width 240 |
                Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "process-details.txt") -Encoding UTF8

            $processIDs = @($runtimeProcesses | ForEach-Object { [uint32]$_.ProcessId })
            if ($null -ne (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue)) {
                try {
                    Get-NetTCPConnection -ErrorAction Stop |
                        Where-Object { $processIDs -contains [uint32]$_.OwningProcess } |
                        Select-Object State, LocalAddress, LocalPort, RemoteAddress, RemotePort, OwningProcess |
                        Sort-Object OwningProcess, LocalPort, RemotePort |
                        Format-Table -AutoSize | Out-String -Width 240 |
                        Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "network-connections.txt") -Encoding UTF8
                }
                catch {
                    Add-CollectionError -Area "agent network connections" -Failure $_
                }
            }
        }
    }
    catch {
        Add-CollectionError -Area "agent process details" -Failure $_
        "Failed to collect process details: $(Protect-DiagnosticText -Text $_.Exception.Message)" |
            Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "process-details.txt") -Encoding UTF8
    }

    try {
        $processIDs = @($runtimeProcesses | ForEach-Object { [uint32]$_.ProcessId })
        $processes = @(Get-Process -ErrorAction SilentlyContinue | Where-Object {
            $processIDs -contains [uint32]$_.Id -or $_.ProcessName -ieq "CSGClaw" -or
            $_.ProcessName -ieq "codex" -or $_.ProcessName -like "csgclaw-update-helper-*"
        })
        if ($processes.Count -eq 0) {
            "No running CSGClaw or related runtime process was found." |
                Set-Content -LiteralPath (Join-Path $stagingDirectory "process-status.txt") -Encoding UTF8
        }
        else {
            $processes |
                Select-Object Id, ProcessName, StartTime, CPU, WorkingSet64, Handles,
                    @{Name = "ThreadCount"; Expression = { $_.Threads.Count }}, Path,
                    @{Name = "FileVersion"; Expression = { $_.FileVersionInfo.FileVersion }} |
                Format-List |
                Out-File -LiteralPath (Join-Path $stagingDirectory "process-status.txt") -Encoding UTF8
        }
    }
    catch {
        Add-CollectionError -Area "process status" -Failure $_
    }

    try {
        Get-CimInstance Win32_OperatingSystem -ErrorAction Stop |
            Select-Object Caption, Version, BuildNumber, OSArchitecture, LastBootUpTime,
                TotalVisibleMemorySize, FreePhysicalMemory |
            Format-List |
            Out-File -LiteralPath (Join-Path $stagingDirectory "windows-system-info.txt") -Encoding UTF8
    }
    catch {
        Add-CollectionError -Area "Windows system information" -Failure $_
    }

    # Read only the agents section, never copy the complete state (rooms/users/messages).
    $agentStatePath = Join-Path (Split-Path -Parent $AgentsDirectory.TrimEnd('\', '/')) "state.json"
    if (Test-Path -LiteralPath $agentStatePath -PathType Leaf) {
        try {
            $state = Get-Content -LiteralPath $agentStatePath -Raw -Encoding UTF8 -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
            $savedAgents = Get-DiagnosticProperty -InputObject $state -Name "agents"
            if ($null -ne $savedAgents) {
                $safe = ConvertTo-RedactedDiagnosticValue -Value $savedAgents
                ConvertTo-Json -InputObject $safe -Depth 32 |
                    Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "persisted-agents.json") -Encoding UTF8
                $collectedPaths.Add($agentStatePath)
            }
        }
        catch {
            Add-CollectionError -Area "persisted agent state" -Failure $_
        }
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

                foreach ($kind in @("codex", "dsh")) {
                    Collect-HostRuntimeFiles -AgentHome $agentHome.FullName -Destination $agentDirectory -Kind $kind
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
        "Not deliberately enumerated: room message storage, workspace files, config.toml, auth files, or full Codex/DSH home contents."
        "Runtime and backend logs may still contain text emitted by a runtime. Review the archive before sharing it outside your support channel."
        "Common token, password, API key, authorization, cookie, credential, header, and env values are best-effort redacted from collected text and Agent metadata."
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

    $runtimeEventPattern = '(?i)(CSGClaw|deepseek-harness|\bdsh(?:\.cmd|\.exe)?\b|\bnode\.exe\b|\bcodex\.exe\b)'
    Write-DiagnosticEvents -LogName "Application" -Destination (Join-Path $stagingDirectory "windows-events.txt") `
        -MessagePattern $runtimeEventPattern
    Write-DiagnosticEvents -LogName "System" -Destination (Join-Path $stagingDirectory "windows-power-events.txt") `
        -ProviderNames @("Microsoft-Windows-Kernel-Power", "Microsoft-Windows-Power-Troubleshooter", "Microsoft-Windows-Kernel-General")
    Write-DiagnosticEvents -LogName "Microsoft-Windows-Windows Defender/Operational" `
        -Destination (Join-Path $stagingDirectory "windows-defender-events.txt") -MessagePattern $runtimeEventPattern

    @(
        "collected_at=$(Get-Date -Format o)"
        "user_data_directory=$UserDataDirectory"
        "event_lookback_minutes=$EventLookbackMinutes"
        "agent_api_base_url=$agentAPIBase"
        "agents_directory=$AgentsDirectory"
        "agent_log_lines=$AgentLogLines"
        "desktop_log_lines=$DesktopLogLines"
        "agent_request_timeout_seconds=$AgentRequestTimeoutSeconds"
        "powershell_version=$($PSVersionTable.PSVersion)"
        "windows_version=$([Environment]::OSVersion.VersionString)"
        "timezone=$([TimeZoneInfo]::Local.Id)"
        "utc_offset=$([TimeZoneInfo]::Local.GetUtcOffset([DateTime]::Now))"
        "is_64bit_os=$([Environment]::Is64BitOperatingSystem)"
        "is_64bit_process=$([Environment]::Is64BitProcess)"
        "user_data_directory_exists=$(Test-Path -LiteralPath $UserDataDirectory -PathType Container)"
    ) | Set-Content -LiteralPath (Join-Path $stagingDirectory "diagnostics-info.txt") -Encoding UTF8

    if ($collectedPaths.Count -eq 0) {
        "No desktop log, Crashpad dump, or Agent state/runtime file was found." |
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

    @(
        "CSGClaw diagnostics: send only csgclaw-diagnostics.zip to the person investigating the issue."
        "Collected at: $(Get-Date -Format o)"
        "All desktop, Codex, DSH, process, network, and Windows event diagnostics are in this single archive."
        "The next successful collection replaces this archive. No separate Agent or crash archive is generated."
        "Check collection-errors.txt for unavailable data; partial collection does not mean all diagnostics are missing."
        "Desktop logs contain at most $DesktopLogLines lines per file; Agent logs contain at most $AgentLogLines lines per source."
        "Common credentials are best-effort redacted from text. Crashpad dumps are binary and cannot be redacted."
        "Room messages, workspace documents, full runtime homes, and auth files are not intentionally collected."
    ) | Set-Content -LiteralPath (Join-Path $stagingDirectory "README.txt") -Encoding UTF8

    # Build beside the destination, then replace it only after compression succeeds.
    Compress-Archive -Path (Join-Path $stagingDirectory "*") -DestinationPath $temporaryArchivePath -Force
    if (Test-Path -LiteralPath $archivePath -PathType Leaf) {
        [IO.File]::Replace($temporaryArchivePath, $archivePath, $null)
    }
    else {
        [IO.File]::Move($temporaryArchivePath, $archivePath)
    }
}
finally {
    Remove-Item -LiteralPath $stagingDirectory -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $temporaryArchivePath -Force -ErrorAction SilentlyContinue
}

Write-Host "CSGClaw diagnostics archive created: $archivePath"
Write-Host "Send this single ZIP file to the person investigating the issue."
if ($collectionErrors.Count -gt 0) {
    Write-Host "Some data was unavailable; see collection-errors.txt inside the archive."
}
