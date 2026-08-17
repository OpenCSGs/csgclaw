param(
    [string]$UserDataDirectory = (Join-Path $env:APPDATA "CSGClaw"),
    [string]$OutputDirectory = [Environment]::GetFolderPath([Environment+SpecialFolder]::Desktop),
    [ValidateRange(1, 1440)]
    [int]$EventLookbackMinutes = 30
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

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

New-Item -ItemType Directory -Path $stagingDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null

try {
    foreach ($name in @("main.log", "main.previous.log", "backend.log", "channel-installer.log")) {
        $source = Get-ChildItem -LiteralPath $UserDataDirectory -Recurse -File -Filter $name -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending |
            Select-Object -First 1
        if ($null -eq $source) {
            continue
        }
        Copy-Item -LiteralPath $source.FullName -Destination (Join-Path $stagingDirectory $name) -Force
        $collectedPaths.Add($source.FullName)
    }

    $squirrelLog = Join-Path $env:LOCALAPPDATA "SquirrelTemp\SquirrelSetup.log"
    if (Test-Path -LiteralPath $squirrelLog -PathType Leaf) {
        Copy-Item -LiteralPath $squirrelLog `
            -Destination (Join-Path $stagingDirectory "squirrel-setup.log") `
            -Force
        $collectedPaths.Add($squirrelLog)
    }

    Get-ChildItem -LiteralPath $UserDataDirectory -Recurse -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Extension -in @(".dmp", ".meta") } |
        ForEach-Object {
            Copy-Item -LiteralPath $_.FullName `
                -Destination (Join-Path $stagingDirectory "crash-$($_.Name)") `
                -Force
            $collectedPaths.Add($_.FullName)
        }

    if ($collectedPaths.Count -eq 0) {
        "No desktop log or Crashpad dump was found under $UserDataDirectory." |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "collected-paths.txt") -Encoding UTF8
    }
    else {
        $collectedPaths |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "collected-paths.txt") -Encoding UTF8
    }

    $processes = @(Get-Process -Name "CSGClaw" -ErrorAction SilentlyContinue)
    if ($processes.Count -eq 0) {
        "No running CSGClaw process was found." |
            Set-Content -LiteralPath (Join-Path $stagingDirectory "process-status.txt") -Encoding UTF8
    }
    else {
        $processes |
            Select-Object Id, StartTime, Path |
            Format-List |
            Out-File -LiteralPath (Join-Path $stagingDirectory "process-status.txt") -Encoding UTF8
    }

    $installationDirectory = Join-Path $env:LOCALAPPDATA "csgclaw_desktop"
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
        "powershell_version=$($PSVersionTable.PSVersion)"
        "windows_version=$([Environment]::OSVersion.VersionString)"
    ) | Set-Content -LiteralPath (Join-Path $stagingDirectory "diagnostics-info.txt") -Encoding UTF8

    Compress-Archive -Path (Join-Path $stagingDirectory "*") -DestinationPath $archivePath -Force
}
finally {
    Remove-Item -LiteralPath $stagingDirectory -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "CSGClaw diagnostics archive created: $archivePath"
