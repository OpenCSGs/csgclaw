$ErrorActionPreference = "Stop"

$App = if ($env:APP) { $env:APP } else { "csgclaw" }
$Version = if ($env:VERSION) { $env:VERSION } else { "latest" }
$AppHome = if ($env:APP_HOME) { $env:APP_HOME } else { Join-Path $HOME "AppData\Local\Programs\csgclaw" }
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $AppHome "bin" }
$LibDir = if ($env:LIB_DIR) { $env:LIB_DIR } else { Join-Path $AppHome "lib\csgclaw" }
$MirrorHost = if ($env:MIRROR_HOST) { $env:MIRROR_HOST } else { "https://csgclaw.opencsg.com" }
$BaseUrl = if ($env:BASE_URL) { $env:BASE_URL } else { "$MirrorHost/releases" }
$LatestReleaseUrl = if ($env:LATEST_RELEASE_URL) { $env:LATEST_RELEASE_URL } else { "$MirrorHost/releases/latest" }

function Resolve-Arch {
    $name = $null
    try {
        $osArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
        if ($null -ne $osArch) {
            $name = $osArch.ToString().ToLowerInvariant()
        }
    } catch {
        $name = $null
    }
    if (-not $name) {
        # 32-bit PowerShell on 64-bit Windows exposes native CPU here.
        if ($env:PROCESSOR_ARCHITEW6432 -eq "AMD64") {
            return "amd64"
        }
        if ($env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
            return "amd64"
        }
        throw "Unsupported architecture: could not detect CPU (PROCESSOR_ARCHITECTURE=$($env:PROCESSOR_ARCHITECTURE); PROCESSOR_ARCHITEW6432=$($env:PROCESSOR_ARCHITEW6432)). Official Windows installer supports windows/amd64 only."
    }
    switch ($name) {
        "x64" { return "amd64" }
        default {
            throw "Unsupported architecture: $name. Official Windows installer currently supports windows/amd64 only."
        }
    }
}

function Resolve-Version {
    if ($Version -ne "latest") {
        return $Version
    }

    $release = Invoke-RestMethod -Uri $LatestReleaseUrl
    if (-not $release.tag_name) {
        throw "Failed to resolve latest release from $LatestReleaseUrl"
    }
    return $release.tag_name
}

function Ensure-Directory {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path | Out-Null
    }
}

function Test-PathContainsInstallDir {
    $pathEntries = ($env:Path -split ';') | Where-Object { $_ } | ForEach-Object { $_.TrimEnd('\') }
    $target = $InstallDir.TrimEnd('\')
    return $pathEntries -contains $target
}

function Add-InstallDirToUserPath {
    if (Test-PathContainsInstallDir) {
        return $false
    }

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @()
    if ($userPath) {
        $entries = $userPath -split ';' | Where-Object { $_ }
    }

    $normalizedInstallDir = $InstallDir.TrimEnd('\')
    foreach ($entry in $entries) {
        if ($entry.TrimEnd('\') -eq $normalizedInstallDir) {
            return $false
        }
    }

    $newUserPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
    $env:Path = if ($env:Path) { "$env:Path;$InstallDir" } else { $InstallDir }
    return $true
}

function Test-CommandAvailable {
    param([string]$Name)

    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Write-LauncherCmd {
    param(
        [string]$Path,
        [string]$TargetExePath
    )

    $escapedTarget = $TargetExePath.Replace('%', '%%')
    $content = @(
        "@echo off",
        "`"$escapedTarget`" %*"
    ) -join "`r`n"
    Set-Content -LiteralPath $Path -Value $content -Encoding ASCII
}

function Install-Launcher {
    param([string]$TargetExePath)

    $launcherExePath = Join-Path $InstallDir "${App}.exe"
    $launcherCmdPath = Join-Path $InstallDir "${App}.cmd"

    if (Test-Path -LiteralPath $launcherExePath) {
        Remove-Item -LiteralPath $launcherExePath -Force
    }
    if (Test-Path -LiteralPath $launcherCmdPath) {
        Remove-Item -LiteralPath $launcherCmdPath -Force
    }

    try {
        New-Item -ItemType SymbolicLink -Path $launcherExePath -Target $TargetExePath | Out-Null
        return $launcherExePath
    } catch {
        Write-LauncherCmd -Path $launcherCmdPath -TargetExePath $TargetExePath
        return $launcherCmdPath
    }
}

function Install-Bundle {
    param(
        [string]$ResolvedVersion,
        [string]$Arch
    )

    $archiveName = "${App}_${ResolvedVersion}_windows_${Arch}.zip"
    $downloadUrl = "$BaseUrl/$ResolvedVersion/$archiveName"
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("csgclaw-install-" + [System.Guid]::NewGuid().ToString("N"))

    try {
        Ensure-Directory -Path $tmpDir

        $archivePath = Join-Path $tmpDir $archiveName
        $extractDir = Join-Path $tmpDir "extract"

        Write-Host "Downloading $downloadUrl"
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force

        $bundlePath = Join-Path $extractDir $App
        $bundleBinPath = Join-Path $bundlePath "bin"
        $bundleExePath = Join-Path $bundleBinPath "${App}.exe"
        if (-not (Test-Path -LiteralPath $bundleExePath)) {
            throw "Archive did not contain $App/bin/${App}.exe"
        }

        Ensure-Directory -Path $InstallDir
        Ensure-Directory -Path $LibDir

        $installRoot = Join-Path $LibDir $ResolvedVersion
        if (Test-Path -LiteralPath $installRoot) {
            Remove-Item -LiteralPath $installRoot -Recurse -Force
        }
        Ensure-Directory -Path $installRoot

        $installedBundlePath = Join-Path $installRoot $App
        Copy-Item -LiteralPath $bundlePath -Destination $installedBundlePath -Recurse

        $targetExePath = Join-Path $installedBundlePath "bin\${App}.exe"
        $launcherPath = Install-Launcher -TargetExePath $targetExePath

        $pathUpdated = Add-InstallDirToUserPath

        Write-Host ""
        Write-Host "Installed $App $ResolvedVersion to $targetExePath"
        Write-Host "Launcher: $launcherPath"
        Write-Host ""
        Write-Host "Next steps:"
        Write-Host "  $App serve"

        if ($pathUpdated) {
            Write-Host ""
            Write-Host "Added $InstallDir to your user PATH."
            Write-Host "Open a new terminal if '$App' is not available in the current session."
        } elseif (-not (Test-PathContainsInstallDir)) {
            Write-Host ""
            Write-Host "$InstallDir is not on your PATH."
            Write-Host "Add it to your user PATH to run '$App' directly."
        }

        if (-not (Test-CommandAvailable -Name "docker")) {
            Write-Host ""
            Write-Host "Warning: Docker was not found on PATH."
            Write-Host "Windows bundles currently default to Docker. Install Docker Desktop or set [sandbox].docker_cli_path explicitly."
        }
    }
    finally {
        if (Test-Path -LiteralPath $tmpDir) {
            Remove-Item -LiteralPath $tmpDir -Recurse -Force
        }
    }
}

if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Unsupported platform: this installer is for Windows only."
}

$arch = Resolve-Arch
$resolvedVersion = Resolve-Version
Install-Bundle -ResolvedVersion $resolvedVersion -Arch $arch
