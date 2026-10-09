# Collect recent work plus current configuration snapshots without changing the runtime.
# Default window: 6 hours. Override with -LookbackMinutes; inspect work-manifest.json for gaps.
param(
    [string]$UserDataDirectory = (Join-Path $env:APPDATA "CSGClaw"),
    [string]$OutputDirectory = [Environment]::GetFolderPath([Environment+SpecialFolder]::Desktop),
    [ValidateRange(1, 1440)]
    [Alias("LookbackMinutes")]
    [int]$EventLookbackMinutes = 360,
    [string]$AgentAPIBaseURL = "http://127.0.0.1:18080",
    [string]$AgentsDirectory = (Join-Path (Join-Path $env:USERPROFILE ".csgclaw") "agents"),
    [ValidateRange(1, 5000)]
    [int]$AgentLogLines = 500,
    [ValidateRange(1, 120)]
    [int]$AgentRequestTimeoutSeconds = 10,
    [ValidateRange(1, 100000)]
    [int]$DesktopLogLines = 10000,
    [ValidateRange(1, 256)]
    [int]$MaxFileMB = 32,
    [ValidateRange(1, 2048)]
    [int]$MaxWorkDataMB = 256
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

    if ($Text.TrimStart().StartsWith('{') -or $Text.TrimStart().StartsWith('[')) {
        try {
            $parsed = ConvertFrom-Json -InputObject $Text -ErrorAction Stop
            return ConvertTo-Json -InputObject (ConvertTo-RedactedDiagnosticValue $parsed) -Depth 100 -Compress
        } catch { }
    }

    $redacted = $Text
    $redacted = $redacted -replace '(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]+', '${1}[REDACTED]'
    $redacted = $redacted -replace '(?i)("(?:api[_-]?key|access[_-]?key(?:[_-]?(?:id|secret))?|access[_-]?token|refresh[_-]?token|session[_-]?token|token|password|secret|authorization|cookie)"\s*:\s*)"(?:\\.|[^"\\])*"', '${1}"[REDACTED]"'
    $redacted = $redacted -replace '(?i)((?:api[_-]?key|access[_-]?key(?:[_-]?(?:id|secret))?|access[_-]?token|refresh[_-]?token|session[_-]?token|token|password|secret|authorization|cookie)\s*[=:]\s*)(?:"[^"]*"|''[^'']*''|[^\s,;]+)', '${1}[REDACTED]'
    $redacted = $redacted -replace '(?i)(--(?:api-key|token|access-token|password)(?:=|\s+))(?:"[^"]*"|[^\s]+)', '${1}[REDACTED]'
    $redacted = $redacted -replace '(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----', '[REDACTED PRIVATE KEY]'
    $redacted = $redacted -replace '(?i)(https?://)[^/@\s]+:[^/@\s]+@', '${1}[REDACTED]@'
    $redacted = $redacted -replace '(?i)([?&](?:[^=&\s]*(?:token|secret|password|api[_-]?key)|key)=)[^&\s"'']+', '${1}[REDACTED]'
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

    if ($PropertyName -match '(?i)^(headers|http_headers|env|environment)$') {
        # Keep names useful for diagnosis, but never assume an unfamiliar value is public.
        $names = if ($Value -is [System.Collections.IDictionary]) { @($Value.Keys) } else { @($Value.PSObject.Properties | ForEach-Object { $_.Name }) }
        $masked = [ordered]@{}
        foreach ($name in $names) { $masked[$name] = '[REDACTED]' }
        return $masked
    }

    $sensitiveProperty = $PropertyName -match '(?i)(api[_-]?key|access[_-]?key|access[_-]?token|refresh[_-]?token|session[_-]?token|authorization|password|secret|credential|cookie|private[_-]?key|^token$)'
    if ($sensitiveProperty -and $PropertyName -notmatch '(?i)_set$') {
        return "[REDACTED]"
    }

    if ($Value -is [string]) {
        # Tool results and arguments can themselves contain serialized JSON.
        if ($Value.TrimStart().StartsWith('{') -or $Value.TrimStart().StartsWith('[')) {
            try {
                $nested = ConvertFrom-Json -InputObject $Value -ErrorAction Stop
                $safeNested = ConvertTo-RedactedDiagnosticValue -Value $nested
                return ConvertTo-Json -InputObject $safeNested -Depth 100 -Compress
            } catch { }
        }
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
    if ($InputObject -is [System.Collections.IDictionary]) { return $InputObject[$Name] }
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

    $parsed = Read-WorkJson $Source
    if ($null -ne $parsed) { Write-WorkValue $parsed $Source $Destination }

}

function Write-RedactedLogTail {
    param(
        [string]$Source,
        [string]$Destination,
        [int]$Lines,
        [string]$Area
    )

    # Kept as a compatibility wrapper; local logs now use the shared time window.
    Export-WorkLog -Source $Source -Destination $Destination

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

# Work files are read locally: no running server or extra Windows dependency is required.
function Add-WorkManifest {
    param([string]$Source, [string]$Output, [string]$Status, [string]$Note = '')
    $script:workManifest.Add([pscustomobject]@{source=$Source; output=$Output; status=$Status; note=$Note})
}

function Get-WorkFiles {
    param([string]$Directory)
    if (-not (Test-Path -LiteralPath $Directory -PathType Container)) { return }
    # Never follow junctions/symlinks into unrelated workspaces or credential directories.
    if (-not (Test-WorkPath $Directory)) { Add-WorkManifest $Directory '' 'skipped' 'Reparse point'; return }
    foreach ($item in @(Get-ChildItem -LiteralPath $Directory -Force -ErrorAction SilentlyContinue)) {
        if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }
        if ($item.PSIsContainer) { Get-WorkFiles -Directory $item.FullName } else { $item }
    }
}

function Test-WorkPath {
    param([string]$Path)
    # Check ancestors as well as the leaf; a blob/workspace path may cross a junction.
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    while ($null -ne $item) {
        if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { return $false }
        if ($item -is [IO.DirectoryInfo]) { $item = $item.Parent } else { $item = $item.Directory }
    }
    return $true
}

function Read-WorkJson {
    param([string]$Source)
    if (-not (Test-Path -LiteralPath $Source -PathType Leaf)) {
        Add-WorkManifest $Source '' 'missing'; return
    }
    if (-not (Test-WorkPath $Source)) { Add-WorkManifest $Source '' 'skipped' 'Reparse point'; return }
    if ((Get-Item -LiteralPath $Source -Force).Length -gt $MaxFileMB * 1MB) {
        Add-WorkManifest $Source '' 'skipped' 'JSON exceeds MaxFileMB; raw file omitted'; return
    }
    try { return ConvertFrom-Json -InputObject (Get-Content -LiteralPath $Source -Raw -Encoding UTF8) }
    catch { Add-WorkManifest $Source '' 'error' (Protect-DiagnosticText $_.Exception.Message) }
}

function Write-WorkValue {
    param([object]$Value, [string]$Source, [string]$Destination, [string]$Note = 'current snapshot')
    $safe = ConvertTo-RedactedDiagnosticValue -Value $Value
    $text = ConvertTo-Json -InputObject $safe -Depth 100
    $bytes = [Text.Encoding]::UTF8.GetByteCount($text)
    if ($bytes -gt $MaxFileMB * 1MB -or $script:workBytes + $bytes -gt $MaxWorkDataMB * 1MB) {
        Add-WorkManifest $Source $Destination 'skipped' 'Size budget exceeded'; return
    }
    New-Item -ItemType Directory -Path (Split-Path -Parent $Destination) -Force | Out-Null
    [IO.File]::WriteAllText($Destination, $text, [Text.UTF8Encoding]::new($false))
    $script:workBytes += $bytes
    Add-WorkManifest $Source $Destination 'collected' $Note
    $script:collectedPaths.Add($Source)
}

function Get-WorkTime {
    param([object]$Value)
    if ($null -eq $Value) { return $null }
    $parsedTime = [DateTimeOffset]::MinValue
    $raw = [string]$Value
    if ($raw -match '^\d{13}$') { return [DateTimeOffset]::FromUnixTimeMilliseconds([long]$raw) }
    # Go can write nanoseconds; Windows .NET parsing accepts at most seven digits.
    $raw = $raw -replace '(\.\d{7})\d+(?=Z|[+-]\d\d:\d\d)', '$1'
    if ([DateTimeOffset]::TryParse($raw, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::AssumeLocal, [ref]$parsedTime)) { return $parsedTime }
    return $null
}

function Get-RecordTime {
    param([object]$Record)
    foreach ($name in @('timestamp', 'created_at', 'started_at', 'createdAt', 'updated_at', 'time')) {
        $value = Get-WorkTime (Get-DiagnosticProperty $Record $name)
        if ($null -ne $value) { return $value }
    }
    return $null
}

function Register-WorkIdentity {
    param([object]$Record)
    foreach ($name in @('agent_id', 'sender_id', 'actor_id', 'assigned_to', 'claimed_by', 'created_by', 'assignment_id')) {
        $identity = [string](Get-DiagnosticProperty $Record $name)
        if ($identity) { $script:activeIdentities[$identity] = $true }
    }
    $room = [string](Get-DiagnosticProperty $Record 'room_id')
    if ($room) { $script:activeRooms[$room] = $true }
}

function Export-WorkLog {
    param([string]$Source, [string]$Destination, [switch]$JsonLines, [switch]$RuntimeSession, [switch]$Snapshot)
    if (-not (Test-Path -LiteralPath $Source -PathType Leaf)) { Add-WorkManifest $Source $Destination 'missing'; return }
    $file = Get-Item -LiteralPath $Source -Force
    if (-not (Test-WorkPath $Source)) { Add-WorkManifest $Source $Destination 'skipped' 'Reparse point'; return }
    if (-not $Snapshot -and $file.LastWriteTimeUtc -lt $script:windowStart.UtcDateTime) {
        Add-WorkManifest $Source $Destination 'outside_window'; return
    }
    $reader = $null; $writer = $null; $stream = $null
    $bytes = 0L; $count = 0; $invalid = 0; $truncated = $false; $hasTime = $false
    $include = [bool]$Snapshot; $header = $null; $contexts = [ordered]@{}; $maskedSection = $false; $privateKey = $false; $untimed = 0
    try {
        New-Item -ItemType Directory -Path (Split-Path -Parent $Destination) -Force | Out-Null
        $stream = [IO.File]::Open($Source, [IO.FileMode]::Open, [IO.FileAccess]::Read, ([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete))
        $reader = [IO.StreamReader]::new($stream, [Text.Encoding]::UTF8, $true)
        $writer = [IO.StreamWriter]::new($Destination, $false, [Text.UTF8Encoding]::new($false))
        while ($null -ne ($line = $reader.ReadLine())) {
            $record = $null; $recordTime = $null
            if ($JsonLines) {
                if (-not $line.Trim()) { continue }
                try { $record = ConvertFrom-Json -InputObject $line -ErrorAction Stop }
                catch { $invalid++; continue } # A live file may end with a partially written record.
                $recordTime = Get-RecordTime $record
                $kind = [string](Get-DiagnosticProperty $record 'type')
                if ($RuntimeSession -and $count -eq 0) {
                    if ($kind -in @('session_meta', 'header', 'session')) { $header = $record }
                    if ($kind -in @('turn_context', 'compacted')) { $contexts[$kind] = $record }
                }
            } elseif ($line -match '(\d{4}-\d\d-\d\d[T ]\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)?)') {
                $recordTime = Get-WorkTime $Matches[1]
            }
            if ($null -ne $recordTime) {
                $hasTime = $true
                $include = $Snapshot -or ($recordTime -ge $script:windowStart -and $recordTime -le $script:windowEnd)
            } elseif ($JsonLines -or -not $hasTime) {
                # JSONL event formats without timestamps cannot inherit the session creation time.
                $include = $true
                $untimed++
            }
            if (-not $include) { continue }
            $lines = [Collections.Generic.List[string]]::new()
            if ($JsonLines) {
                if ($RuntimeSession -and $count -eq 0) {
                    foreach ($prior in (@($header) + @($contexts.Values))) {
                        if ($null -ne $prior -and $prior -ne $record) {
                            $lines.Add((ConvertTo-Json -InputObject (ConvertTo-RedactedDiagnosticValue $prior) -Depth 100 -Compress))
                        }
                    }
                }
                $lines.Add((ConvertTo-Json -InputObject (ConvertTo-RedactedDiagnosticValue $record) -Depth 100 -Compress))
                Register-WorkIdentity $record
                $blobRef = [string](Get-DiagnosticProperty $record 'blob_ref')
                if ($blobRef) { $script:workBlobs[$blobRef] = $true }
            } else {
                if ($line -match '-----BEGIN [^-]*PRIVATE KEY-----') { $privateKey = $true }
                if ($privateKey) {
                    if ($line -match '-----END [^-]*PRIVATE KEY-----') { $privateKey = $false }
                    $line = '[REDACTED PRIVATE KEY]'
                }
                # TOML/YAML maps may contain arbitrary names, e.g. headers.X-Custom.
                if ($Snapshot -and $line -match '^\s*\[') { $maskedSection = $line -match '(?i)(env|environment|headers)\]$' }
                if ($Snapshot -and $line -match '(?i)^\s*(env|environment|headers|http_headers)\s*[:=]') { $maskedSection = $true }
                if ($Snapshot -and $maskedSection -and $line -match '^(\s*[^#\[=:\s][^=:]*[=:]).+$') {
                    $line = $Matches[1] + ' "[REDACTED]"'
                }
                $lines.Add((Protect-DiagnosticText $line))
            }
            foreach ($safeLine in $lines) {
                $size = [Text.Encoding]::UTF8.GetByteCount($safeLine) + 1
                if ($bytes + $size -gt $MaxFileMB * 1MB -or $script:workBytes + $size -gt $MaxWorkDataMB * 1MB) { $truncated = $true; break }
                $writer.WriteLine($safeLine); $bytes += $size; $script:workBytes += $size
            }
            if ($truncated) { break }
            $count++
        }
        $status = if ($truncated) { 'truncated' } elseif ($count -eq 0) { 'no_records' } else { 'collected' }
        $mode = if ($Snapshot) { 'current snapshot' } elseif ($hasTime) { 'time window; session header/latest preceding context retained' } else { 'no record timestamps: bounded file selected by mtime' }
        Add-WorkManifest $Source $Destination $status "$mode; records=$count; untimed_records=$untimed; invalid_json_lines=$invalid; bytes=$bytes"
        $script:collectedPaths.Add($Source)
    } catch { Add-WorkManifest $Source $Destination 'error' (Protect-DiagnosticText $_.Exception.Message) }
    finally {
        if ($null -ne $writer) { $writer.Dispose() }
        if ($null -ne $reader) { $reader.Dispose() } elseif ($null -ne $stream) { $stream.Dispose() }
    }
}

function Collect-RecentWork {
    $dataRoot = Split-Path -Parent $AgentsDirectory.TrimEnd('\', '/')
    $workRoot = Join-Path $script:stagingDirectory 'recent-work'
    foreach ($name in @('im/diagnostics', 'im/sessions', 'tasks', 'agents')) {
        $path = Join-Path $dataRoot $name
        if (-not (Test-Path -LiteralPath $path -PathType Container)) { Add-WorkManifest $path '' 'missing' }
    }
    $diagnosticsRoot = Join-Path $dataRoot 'im/diagnostics'
    foreach ($file in @(Get-WorkFiles $diagnosticsRoot | Where-Object { $_.Extension -eq '.json' -and $_.LastWriteTimeUtc -ge $script:windowStart.UtcDateTime })) {
        $value = Read-WorkJson $file.FullName
        if ($null -eq $value) { continue }
        $started = Get-RecordTime $value
        $duration = Get-DiagnosticProperty $value 'total_ms'
        if ($null -ne $started -and $null -ne $duration -and [double]$duration -gt 0 -and $started.AddMilliseconds([double]$duration) -lt $script:windowStart) { continue }
        Register-WorkIdentity $value
        Write-WorkValue $value $file.FullName (Join-Path $workRoot "turns/$($file.Name)") 'persisted turn diagnostic (not full prompt)'
    }
    $sessionsRoot = Join-Path $dataRoot 'im/sessions'
    foreach ($file in @(Get-WorkFiles $sessionsRoot | Where-Object { $_.Extension -eq '.jsonl' -and $_.LastWriteTimeUtc -ge $script:windowStart.UtcDateTime })) {
        Export-WorkLog $file.FullName (Join-Path $workRoot "rooms/$($file.Name)") -JsonLines
        $script:activeRooms[$file.BaseName] = $true
    }
    foreach ($blob in @($script:workBlobs.Keys)) {
        $base = [IO.Path]::GetFullPath($sessionsRoot).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
        $path = [IO.Path]::GetFullPath((Join-Path $base $blob))
        if (-not $path.StartsWith($base, [StringComparison]::OrdinalIgnoreCase) -or $path -notlike '*.json') {
            Add-WorkManifest $blob '' 'skipped' 'Invalid message blob path'; continue
        }
        $value = Read-WorkJson $path
        if ($null -ne $value) { Write-WorkValue $value $path (Join-Path $workRoot "rooms/$blob") 'referenced message body; attachment binaries excluded' }
    }
    $tasksRoot = Join-Path $dataRoot 'tasks'
    foreach ($file in @(Get-WorkFiles $tasksRoot | Where-Object { $_.Name -eq 'tasks.json' })) {
        # Include context of tasks used in an active room, even if the original plan is older.
        $value = Read-WorkJson $file.FullName
        if ($null -eq $value) { continue }
        $items = @(Get-DiagnosticProperty $value 'tasks')
        $relevant = $file.LastWriteTimeUtc -ge $script:windowStart.UtcDateTime
        foreach ($task in $items) {
            if ($script:activeRooms.ContainsKey([string](Get-DiagnosticProperty $task 'room_id'))) { $relevant = $true }
        }
        if (-not $relevant) { continue }
        foreach ($task in $items) { Register-WorkIdentity $task }
        $taskRoot = Join-Path $workRoot "tasks/$($file.Directory.Name)"
        Write-WorkValue $value $file.FullName (Join-Path $taskRoot 'tasks.json') 'current task/plan/review snapshot for recent work'
        Export-WorkLog (Join-Path $file.DirectoryName 'events.jsonl') (Join-Path $taskRoot 'events.jsonl') -JsonLines
    }
    $roomStatePath = Join-Path $dataRoot 'im/state.json'
    $roomState = Read-WorkJson $roomStatePath
    if ($null -ne $roomState) {
        $rooms = @(Get-DiagnosticProperty $roomState 'rooms' | Where-Object { $script:activeRooms.ContainsKey([string](Get-DiagnosticProperty $_ 'id')) })
        $members = @{}
        foreach ($room in $rooms) { foreach ($id in @(Get-DiagnosticProperty $room 'members')) { $members[[string]$id] = $true } }
        $users = @(Get-DiagnosticProperty $roomState 'users' | Where-Object { $members.ContainsKey([string](Get-DiagnosticProperty $_ 'id')) })
        Write-WorkValue @{rooms=$rooms; users=$users} $roomStatePath (Join-Path $workRoot 'room-state.json') 'current membership and policy, not historical snapshot'
    }
    # Detect runtime-only activity even when the IM diagnostic was never completed.
    foreach ($dir in @(Get-ChildItem -LiteralPath $AgentsDirectory -Directory -ErrorAction SilentlyContinue)) {
        foreach ($kind in @('codex', 'dsh')) {
            $recent = @(Get-RuntimeWorkFiles (Join-Path $dir.FullName ".$kind") | Where-Object {
                $_.LastWriteTimeUtc -ge $script:windowStart.UtcDateTime -and ($_.Extension -eq '.jsonl' -or $_.Name -like '*.jsonl.zstd' -or $_.Name -eq 'stderr.log')
            } | Select-Object -First 1)
            if ($recent.Count -gt 0) { $script:activeIdentities[$dir.Name] = $true }
        }
    }
    Write-WorkValue @($script:activeIdentities.Keys | Sort-Object) 'activity index' (Join-Path $workRoot 'active-identities.json') 'Agents, participants and humans referenced by recent work'
}

function Test-ActiveAgent {
    param([string]$ID)
    $shortID = $ID -replace '^(agent-|pt-|user-|u-)', ''
    foreach ($candidate in @($ID, $shortID, "agent-$shortID", "pt-$shortID", "user-$shortID", "u-$shortID")) {
        if ($script:activeIdentities.ContainsKey($candidate)) { return $true }
    }
    return $false
}

function Get-RuntimeWorkFiles {
    param([string]$Root)
    # Do not recursively walk runtime workspace repositories, node_modules or arbitrary documents.
    foreach ($name in @('home/sessions', 'home/archived_sessions', 'home/skills', 'workspace/.agents/skills')) {
        Get-WorkFiles (Join-Path $Root $name)
    }
    foreach ($name in @('stderr.log', 'home/stderr.log', 'home/AGENTS.md', 'home/AGENTS.override.md', 'home/config.toml', 'home/settings.yaml', 'csgclaw.patch.yml', 'csgclaw-context.patch.yml')) {
        $path = Join-Path $Root $name
        if (Test-Path -LiteralPath $path -PathType Leaf) { Get-Item -LiteralPath $path -Force }
    }
}

function Collect-AgentWorkContext {
    param([string]$AgentHome, [string]$Destination)
    foreach ($kind in @('codex', 'dsh')) {
        $root = Join-Path $AgentHome ".$kind"
        $target = Join-Path $Destination "$kind-context"
        $sessions = Join-Path $root 'home/sessions'
        if (-not (Test-Path -LiteralPath $sessions -PathType Container)) {
            Add-WorkManifest $sessions '' 'missing' 'No locally persisted runtime prompt/session records'
        }
        foreach ($file in @(Get-RuntimeWorkFiles $root)) {
            $relative = $file.FullName.Substring($root.Length).TrimStart('\', '/')
            $output = Join-Path $target $relative
            if ($file.Extension -eq '.jsonl') {
                Export-WorkLog $file.FullName $output -JsonLines -RuntimeSession
            } elseif ($file.Name -like '*.jsonl.zstd') {
                Add-WorkManifest $file.FullName $output 'skipped' 'Compressed DSH session requires runtime decompression; not copied without redaction'
            } elseif ($file.Name -in @('AGENTS.md', 'AGENTS.override.md', 'SKILL.md', 'config.toml', 'settings.yaml', 'csgclaw.patch.yml', 'csgclaw-context.patch.yml')) {
                Export-WorkLog $file.FullName $output -Snapshot
            }
        }
        # External workspace instructions are current snapshots, not the prompt of a previous turn.
        foreach ($name in @('session.json', 'runtime.json')) {
            $path = Join-Path $root $name
            if (-not (Test-Path -LiteralPath $path)) { continue }
            $value = Read-WorkJson $path
            $workspace = [string](Get-DiagnosticProperty $value 'workspace_dir')
            if ($workspace -and (Test-Path -LiteralPath $workspace -PathType Container)) {
                foreach ($instruction in @('AGENTS.md', 'AGENTS.override.md', '.codex/config.toml')) {
                    Export-WorkLog (Join-Path $workspace $instruction) (Join-Path $target "workspace/$instruction") -Snapshot
                }
            }
        }
    }
    $binding = Join-Path (Split-Path -Parent $AgentsDirectory.TrimEnd('\', '/')) "session-bindings/$([IO.Path]::GetFileName($AgentHome)).jsonl"
    Export-WorkLog $binding (Join-Path $Destination 'session-bindings.jsonl') -JsonLines -Snapshot
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw "collect-desktop-diagnostics.ps1 is only supported on Windows"
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$stagingDirectory = Join-Path ([IO.Path]::GetTempPath()) "csgclaw-diagnostics-$timestamp-$PID"
$archivePath = Join-Path $OutputDirectory "csgclaw-diagnostics-$timestamp-$PID.zip"
$temporaryArchivePath = Join-Path $OutputDirectory ".csgclaw-diagnostics-$timestamp-$PID.zip"
$collectedPaths = [Collections.Generic.List[string]]::new()
$collectionErrors = [Collections.Generic.List[string]]::new()
$workManifest = [Collections.Generic.List[object]]::new()
$activeIdentities = @{}
$activeRooms = @{}
$workBlobs = @{}
$workBytes = 0L
$windowEnd = [DateTimeOffset]::Now
$windowStart = $windowEnd.AddMinutes(-$EventLookbackMinutes)
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
        Where-Object { $_.Extension -in @(".dmp", ".meta") -and $_.LastWriteTimeUtc -ge $windowStart.UtcDateTime } |
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

    Write-Host "Collecting work from $($windowStart.ToString('o')) to $($windowEnd.ToString('o'))..."
    try { Collect-RecentWork }
    catch { Add-CollectionError -Area 'recent work' -Failure $_ }

    # Include instruction/config snapshots only for Agents associated with recent activity.
    $agentStatePath = Join-Path (Split-Path -Parent $AgentsDirectory.TrimEnd('\', '/')) "state.json"
    if (Test-Path -LiteralPath $agentStatePath -PathType Leaf) {
        try {
            $state = Read-WorkJson $agentStatePath
            $savedAgents = Get-DiagnosticProperty -InputObject $state -Name "agents"
            if ($null -ne $savedAgents) {
                $defaults = Get-DiagnosticProperty $savedAgents 'model_defaults'
                if ($null -ne $defaults) { Write-WorkValue $defaults $agentStatePath (Join-Path $agentDiagnosticsDirectory 'model-defaults.json') }
                $items = Get-DiagnosticProperty $savedAgents 'items'
                if ($null -ne $items) { $savedAgents = $items }
                $activeAgents = @($savedAgents | Where-Object { Test-ActiveAgent ([string](Get-DiagnosticProperty $_ 'id')) })
                Write-WorkValue $activeAgents $agentStatePath (Join-Path $agentDiagnosticsDirectory 'persisted-agents.json')
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
        # In Windows PowerShell 5.1, wrapping the pipeline directly can produce [[agents]].
        $parsedAgents = ConvertFrom-Json -InputObject $response.Content -ErrorAction Stop
        $agents = @($parsedAgents)
        $activeAgents = @($agents | Where-Object { Test-ActiveAgent ([string](Get-DiagnosticProperty $_ 'id')) })
        Write-WorkValue $activeAgents "$agentAPIBase/api/v1/agents" (Join-Path $agentDiagnosticsDirectory 'agents.json')
        "Agent API collection succeeded. agent_count=$($agents.Count)" |
            Set-Content -LiteralPath (Join-Path $agentDiagnosticsDirectory "api-status.txt") -Encoding UTF8

        foreach ($agentItem in $activeAgents) {
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
                if (-not (Test-ActiveAgent $agentHome.Name)) { return }
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
                try { Collect-AgentWorkContext -AgentHome $agentHome.FullName -Destination $agentDirectory }
                catch { Add-CollectionError -Area "agent $($agentHome.Name) work context" -Failure $_ }
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
        "Includes recent room messages, turns, task events/plans, active Agent instructions/config, and persisted runtime session prompts/tool calls."
        "Current configuration snapshots are not proof of the exact instructions loaded in a historical turn. Session headers and preceding turn context are included when available."
        "Auth files, arbitrary workspace documents and attachment binaries are excluded. No runtime instrumentation is changed."
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
        "CSGClaw diagnostics: send only $(Split-Path -Leaf $archivePath) to the person investigating the issue."
        "Collected at: $(Get-Date -Format o)"
        "All desktop, Codex, DSH, process, network, and Windows event diagnostics are in this single archive."
        "Each collection creates a new archive in the same output directory and keeps previous archives. No separate Agent or crash archive is generated."
        "Check collection-errors.txt for unavailable data; partial collection does not mean all diagnostics are missing."
        "Work window: $($windowStart.ToString('o')) through $($windowEnd.ToString('o')); default 6 hours."
        "Local logs use this window; API log tails are supplemental (at most $AgentLogLines lines). DesktopLogLines is retained for argument compatibility."
        "Work files: limit $MaxFileMB MiB/file and $MaxWorkDataMB MiB total. Check work-manifest.json for missing, skipped or truncated sources."
        "Records without timestamps fall back to file modification time, labelled in the manifest. Older session headers/task plans/current instructions are context."
        "The script cannot reconstruct prompts/tool metadata that were never saved by the runtime; compressed DSH sessions are explicitly marked unavailable."
        "Common credentials are best-effort redacted from text. Crashpad dumps are binary and cannot be redacted."
        "Recent messages and saved runtime prompts are included; current Agent instructions/config are separate snapshots. Auth files and arbitrary workspace documents are excluded."
    ) | Set-Content -LiteralPath (Join-Path $stagingDirectory "README.txt") -Encoding UTF8

    foreach ($entry in $workManifest) {
        if ($entry.output.StartsWith($stagingDirectory)) { $entry.output = $entry.output.Substring($stagingDirectory.Length).TrimStart('\', '/') }
    }
    ConvertTo-Json -InputObject @{window_start=$windowStart.ToString('o'); window_end=$windowEnd.ToString('o'); files=$workManifest.ToArray()} -Depth 10 |
        Set-Content -LiteralPath (Join-Path $stagingDirectory 'work-manifest.json') -Encoding UTF8

    # Build beside the destination, then publish a new archive after compression succeeds.
    Compress-Archive -Path (Join-Path $stagingDirectory "*") -DestinationPath $temporaryArchivePath -Force
    [IO.File]::Move($temporaryArchivePath, $archivePath)
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
