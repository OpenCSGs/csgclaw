param(
    [Parameter(Position = 0)]
    [string]$Target = "build"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent $PSScriptRoot

function Get-EnvOrDefault {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string]$Default
    )

    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        return $Default
    }
    return $value
}

function Get-CommandPathOrNull {
    param([Parameter(Mandatory = $true)][string]$Name)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -eq $cmd) {
        return $null
    }
    return $cmd.Source
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,
        [string[]]$Arguments = @(),
        [hashtable]$Env = @{}
    )

    $backup = @{}
    try {
        foreach ($key in $Env.Keys) {
            $existing = [Environment]::GetEnvironmentVariable($key)
            $backup[$key] = $existing
            [Environment]::SetEnvironmentVariable($key, [string]$Env[$key])
        }

        & $FilePath @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "command failed: $FilePath $($Arguments -join ' ')"
        }
    }
    finally {
        foreach ($key in $Env.Keys) {
            $previous = $backup[$key]
            if ($null -eq $previous) {
                [Environment]::SetEnvironmentVariable($key, $null)
            }
            else {
                [Environment]::SetEnvironmentVariable($key, $previous)
            }
        }
    }
}

function Get-CommandOutput {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,
        [string[]]$Arguments = @()
    )

    $previousErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $result = & $FilePath @Arguments 2>$null
        if ($LASTEXITCODE -ne 0) {
            return $null
        }
        return (($result | Out-String).Trim())
    }
    finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
}

function Test-VersionMajorRange {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Version,
        [Parameter(Mandatory = $true)]
        [int]$MinMajor,
        [Parameter(Mandatory = $true)]
        [int]$MaxMajorExclusive
    )

    if ($Version -notmatch '^[vV]?(\d+)(?:\.(\d+))?(?:\.(\d+))?') {
        return $false
    }

    $major = [int]$Matches[1]
    return $major -ge $MinMajor -and $major -lt $MaxMajorExclusive
}

function Test-NodeSupported {
    param([Parameter(Mandatory = $true)][string]$Version)

    if ($Version -notmatch '^[vV]?(\d+)\.(\d+)\.(\d+)') {
        return $false
    }

    $major = [int]$Matches[1]
    $minor = [int]$Matches[2]
    return ($major -eq 22 -and $minor -ge 13) -or $major -eq 23 -or $major -eq 24
}

function Resolve-Version {
    $git = Get-CommandPathOrNull "git"
    if ($null -eq $git) {
        return "dev+local"
    }

    & $git "rev-parse" "--git-dir" *> $null
    if ($LASTEXITCODE -ne 0) {
        return "dev+local"
    }

    $tag = Get-CommandOutput -FilePath $git -Arguments @("describe", "--tags", "--exact-match")
    if (-not [string]::IsNullOrWhiteSpace($tag)) {
        return "$tag+local"
    }

    $tag = Get-CommandOutput -FilePath $git -Arguments @("describe", "--tags", "--always", "--dirty")
    if (-not [string]::IsNullOrWhiteSpace($tag)) {
        return "$tag+local"
    }

    return "dev+local"
}

function Resolve-Commit {
    $git = Get-CommandPathOrNull "git"
    if ($null -eq $git) {
        return "unknown"
    }

    $commit = Get-CommandOutput -FilePath $git -Arguments @("rev-parse", "--short", "HEAD")
    if ([string]::IsNullOrWhiteSpace($commit)) {
        return "unknown"
    }
    return $commit
}

function Resolve-GoEnv {
    param([Parameter(Mandatory = $true)][string]$Name)

    $go = Get-CommandPathOrNull "go"
    if ($null -eq $go) {
        throw "missing required command: go"
    }

    $value = Get-CommandOutput -FilePath $go -Arguments @("env", $Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "failed to resolve go env $Name"
    }
    return $value
}

function Get-BinaryName {
    param(
        [Parameter(Mandatory = $true)][string]$BaseName,
        [Parameter(Mandatory = $true)][string]$Goos
    )

    if ($Goos -eq "windows") {
        return "$BaseName.exe"
    }
    return $BaseName
}

function Remove-StaleHostBinaries {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$Goos
    )

    $staleNames = if ($Goos -eq "windows") {
        @("csgclaw", "csgclaw-cli")
    }
    else {
        @("csgclaw.exe", "csgclaw-cli.exe")
    }
    foreach ($name in $staleNames) {
        $path = Join-Path $Directory $name
        if (Test-Path -LiteralPath $path -PathType Leaf) {
            Remove-Item -LiteralPath $path -Force
        }
    }
}

function Ensure-Directory {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Path $Path | Out-Null
    }
}

function Ensure-WebLayout {
    if (-not (Test-Path -LiteralPath $script:WebAppDir -PathType Container)) {
        throw "Web UI source directory is missing: $script:WebAppDir.`nRun the script from the csgclaw repository root, or set WEB_APP_DIR to an absolute path."
    }
    if (-not (Test-Path -LiteralPath (Join-Path $script:WebAppDir "package.json") -PathType Leaf)) {
        throw "Web UI package.json is missing: $(Join-Path $script:WebAppDir 'package.json')."
    }
    $workspaceLock = Join-Path $script:WebAppDir "../../pnpm-lock.yaml"
    if (-not (Test-Path -LiteralPath $workspaceLock -PathType Leaf)) {
        throw "Web UI pnpm lockfile is missing: $workspaceLock. Restore the lockfile before building the Web UI."
    }
}

function Resolve-PnpmRunner {
    $pnpm = Get-CommandPathOrNull "pnpm"
    if ($null -ne $pnpm) {
        $version = Get-CommandOutput -FilePath $pnpm -Arguments @("--version")
        if (-not [string]::IsNullOrWhiteSpace($version) -and (Test-VersionMajorRange -Version $version -MinMajor 9 -MaxMajorExclusive 12)) {
            return @{
                FilePath = $pnpm
                Prefix   = @()
                Version  = $version
            }
        }
    }

    $corepack = Get-CommandPathOrNull "corepack"
    if ($null -ne $corepack) {
        $version = Get-CommandOutput -FilePath $corepack -Arguments @("pnpm", "--version")
        if (-not [string]::IsNullOrWhiteSpace($version) -and (Test-VersionMajorRange -Version $version -MinMajor 9 -MaxMajorExclusive 12)) {
            return @{
                FilePath = $corepack
                Prefix   = @("pnpm")
                Version  = $version
            }
        }
    }

    throw "pnpm >=9 and <12 is required for the Web UI. Install a supported pnpm version, or use Node.js with Corepack."
}

function Assert-WebToolchain {
    param([Parameter(Mandatory = $true)][string]$PnpmWorkingDirectory)

    $node = Get-CommandPathOrNull "node"
    if ($null -eq $node) {
        throw "Node.js >=22.13.0 and <25 is required for the Web UI. Install a supported Node.js version first."
    }

    $nodeVersion = Get-CommandOutput -FilePath $node -Arguments @("--version")
    if ([string]::IsNullOrWhiteSpace($nodeVersion) -or -not (Test-NodeSupported -Version $nodeVersion)) {
        throw "Node.js >=22.13.0 and <25 is required for the Web UI; current node is $nodeVersion."
    }

    Push-Location $PnpmWorkingDirectory
    try {
        $runner = Resolve-PnpmRunner
    }
    finally {
        Pop-Location
    }
    Write-Host "Web toolchain OK: Node.js $nodeVersion, pnpm $($runner.Version)"
    return $runner
}

function Invoke-Pnpm {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory,
        [hashtable]$Env = @{}
    )

    Push-Location $WorkingDirectory
    try {
        $runner = Resolve-PnpmRunner
        Invoke-Checked -FilePath $runner.FilePath -Arguments ($runner.Prefix + $Arguments) -Env $Env
    }
    finally {
        Pop-Location
    }
}

function Ensure-WebDeps {
    Ensure-WebLayout
    Assert-WebToolchain -PnpmWorkingDirectory $script:WebAppDir | Out-Null

    $viteUnix = Join-Path $script:WebAppDir "node_modules/.bin/vite"
    $viteWindows = Join-Path $script:WebAppDir "node_modules/.bin/vite.cmd"
    $uiPackage = Join-Path $script:WebAppDir "node_modules/csgclaw-ui/package.json"
    if ((Test-Path -LiteralPath $uiPackage -PathType Leaf) -and
        ((Test-Path -LiteralPath $viteUnix -PathType Leaf) -or (Test-Path -LiteralPath $viteWindows -PathType Leaf))) {
        return
    }

    Write-Host "Web UI dependencies are missing; running web-install before build."
    Invoke-TargetWebInstall
}

function Ensure-DesktopDeps {
    foreach ($file in @("package.json", "pnpm-lock.yaml")) {
        $path = Join-Path $script:DesktopDir $file
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Electron Desktop $file is missing: $path."
        }
    }

    Assert-WebToolchain -PnpmWorkingDirectory $script:DesktopDir | Out-Null
    Write-Host "Checking Electron Desktop dependencies..."
    Invoke-Pnpm -WorkingDirectory $script:DesktopDir -Arguments @("install", "--frozen-lockfile")
}

function Invoke-TargetHelp {
    @(
        "scripts\build.cmd build                         - Windows wrapper; same as build with execution-policy bypass"
        "powershell -File scripts/build.ps1 build        - build Web UI, companion host binaries"
        "powershell -File scripts/build.ps1 build-all    - same as build"
        "powershell -File scripts/build.ps1 fmt          - format Go files"
        "powershell -File scripts/build.ps1 test         - run go test ./..."
        "powershell -File scripts/build.ps1 web-install  - install Web UI dependencies"
        "powershell -File scripts/build.ps1 web-dev      - run Vite Web UI dev server"
        "powershell -File scripts/build.ps1 build-web    - build Web UI app into web/static-dist"
        "scripts\build.cmd desktop-package               - build the Windows website package"
        "scripts\build.cmd desktop-msix                  - build the Windows Microsoft Store MSIX package"
        "powershell -File scripts/build.ps1 build-server-bin - build bin/csgclaw, bin/csgclaw-cli, and bundled Codex"
        "powershell -File scripts/build.ps1 run          - build, then run the server"
        "powershell -File scripts/build.ps1 package      - package the current platform"
        "powershell -File scripts/build.ps1 package-all  - build and package current platform artifacts"
        "powershell -File scripts/build.ps1 release      - build the configured cross-platform release bundles"
        "powershell -File scripts/build.ps1 clean        - remove local build outputs"
    ) | ForEach-Object { Write-Host $_ }
}

function Invoke-TargetFmt {
    $gofmt = Get-CommandPathOrNull "gofmt"
    if ($null -eq $gofmt) {
        throw "missing required command: gofmt"
    }

    $files = Get-ChildItem -Path (Join-Path $RootDir "cli"), (Join-Path $RootDir "cmd"), (Join-Path $RootDir "internal"), (Join-Path $RootDir "web") -Recurse -Filter *.go -File |
        ForEach-Object { $_.FullName }

    if ($files.Count -eq 0) {
        return
    }

    Invoke-Checked -FilePath $gofmt -Arguments (@("-w") + $files)
}

function Invoke-TargetTest {
    $go = Get-CommandPathOrNull "go"
    if ($null -eq $go) {
        throw "missing required command: go"
    }

    Invoke-Checked -FilePath $go -Arguments @("test", "./...") -Env @{ GOCACHE = $script:Gocache }
}

function Invoke-TargetCheckWebToolchain {
    Assert-WebToolchain -PnpmWorkingDirectory $script:WebAppDir | Out-Null
}

function Invoke-TargetWebInstall {
    Ensure-WebLayout
    Assert-WebToolchain -PnpmWorkingDirectory $script:WebAppDir | Out-Null
    Write-Host "Installing Web UI dependencies in $script:WebAppDir."
    Write-Host "If this appears stuck on registry downloads, check npm registry network/proxy access."
    Invoke-Pnpm -WorkingDirectory $script:WebAppDir -Arguments @("install", "--frozen-lockfile")
}

function Invoke-TargetWebDev {
    Ensure-WebDeps
    Invoke-Pnpm -WorkingDirectory $script:WebAppDir -Arguments @("dev")
}

function Invoke-TargetBuildWeb {
    param([switch]$Summary)

    if ($Summary) {
        Write-Host "Building Web UI..."
    }
    else {
        Write-Host "Building Web UI into $script:WebStaticDistDir."
    }
    Ensure-WebDeps
    Ensure-Directory -Path $script:WebStaticDistDir
    $arguments = @()
    if ($Summary) {
        $arguments += "--silent"
    }
    $arguments += "build"
    if ($Summary) {
        $arguments += @("--logLevel", "warn")
    }
    Invoke-Pnpm -WorkingDirectory $script:WebAppDir -Arguments $arguments
    $indexPath = Join-Path $script:WebStaticDistDir "index.html"
    if (-not (Test-Path -LiteralPath $indexPath -PathType Leaf)) {
        throw "Web UI build did not produce $indexPath."
    }
    if ($Summary) {
        $files = @(Get-ChildItem -LiteralPath $script:WebStaticDistDir -Recurse -File)
        $totalBytes = ($files | Measure-Object -Property Length -Sum).Sum
        $totalMegabytes = [math]::Round($totalBytes / 1MB, 1)
        Write-Host "Web UI ready: $script:WebStaticDistDir ($($files.Count) files, ${totalMegabytes}M total)."
    }
}

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory = $true)][string]$OutputPath,
        [Parameter(Mandatory = $true)][string]$PackagePath,
        [Parameter(Mandatory = $true)][string]$Ldflags,
        [hashtable]$Env = @{},
        [string]$Tags = "",
        [switch]$Quiet
    )

    $go = Get-CommandPathOrNull "go"
    if ($null -eq $go) {
        throw "missing required command: go"
    }

    $args = @("build")
    if (-not [string]::IsNullOrWhiteSpace($Tags)) {
        $args += @("-tags", $Tags)
    }
    $args += @("-ldflags", $Ldflags, "-o", $OutputPath, $PackagePath)

    $baseEnv = @{ GOCACHE = $script:Gocache }
    foreach ($key in $Env.Keys) {
        $baseEnv[$key] = $Env[$key]
    }

    if (-not $Quiet) {
        Write-Host "Building $OutputPath from $PackagePath."
    }
    Invoke-Checked -FilePath $go -Arguments $args -Env $baseEnv
}

function Invoke-TargetBuildServerBin {
    Ensure-Directory -Path $script:BinDir
    Remove-StaleHostBinaries -Directory $script:BinDir -Goos $script:TargetOs

    $serverBinary = Join-Path $script:BinDir (Get-BinaryName -BaseName "csgclaw" -Goos $script:TargetOs)
    $cliBinary = Join-Path $script:BinDir (Get-BinaryName -BaseName "csgclaw-cli" -Goos $script:TargetOs)

    Invoke-GoBuild -OutputPath $serverBinary -PackagePath "./cmd/csgclaw" -Ldflags $script:Ldflags
    Invoke-GoBuild -OutputPath $cliBinary -PackagePath "./cmd/csgclaw-cli" -Ldflags $script:CliLdflags -Env @{
        CGO_ENABLED = $script:CgoEnabled
        GOOS        = $script:TargetOs
        GOARCH      = $script:TargetArch
    }
    Fetch-CodexCli -Goos $script:TargetOs -Goarch $script:TargetArch -OutputDir $script:BinDir
}

function Invoke-DesktopBackendBundle {
    param(
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch
    )

    $outputRoot = Join-Path (Join-Path (Join-Path $script:DesktopDir "out") "input") "$Goos-$Goarch"
    if (Test-Path -LiteralPath $outputRoot) {
        Remove-Item -LiteralPath $outputRoot -Recurse -Force
    }

    $bundleRoot = Join-Path (Join-Path $outputRoot "backend") "csgclaw"
    $binDir = Join-Path $bundleRoot "bin"
    Ensure-Directory -Path $binDir
    Set-Content -LiteralPath (Join-Path $bundleRoot ".csgclaw-bundle.json") -Value "{""app"":""csgclaw"",""layout"":""official-bundle"",""version"":""$($script:Version)""}" -NoNewline

    $targetEnv = @{
        CGO_ENABLED = "0"
        GOOS        = $Goos
        GOARCH      = $Goarch
    }
    Invoke-GoBuild -OutputPath (Join-Path $binDir (Get-BinaryName -BaseName "csgclaw" -Goos $Goos)) -PackagePath "./cmd/csgclaw" -Ldflags $script:Ldflags -Env $targetEnv -Tags $script:GoBuildTags -Quiet
    Invoke-GoBuild -OutputPath (Join-Path $binDir (Get-BinaryName -BaseName "csgclaw-cli" -Goos $Goos)) -PackagePath $script:HostCliCmdPath -Ldflags $script:CliLdflags -Env $targetEnv -Quiet
    Fetch-CodexCli -Goos $Goos -Goarch $Goarch -OutputDir $binDir

    if ($Goos -eq "windows") {
        Invoke-GoBuild `
            -OutputPath (Join-Path $outputRoot "csgclaw-update-helper.exe") `
            -PackagePath "./cmd/csgclaw-desktop-update-helper" `
            -Ldflags "-s -w -H=windowsgui" `
            -Env $targetEnv `
            -Quiet
    }

    Write-Host "Desktop backend ready: $bundleRoot"
}

function Resolve-CodexCliDownloadTarget {
    param(
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch
    )

    if (-not (Test-Path -LiteralPath $script:CodexCliPlatformsPath -PathType Leaf)) {
        throw "Codex CLI platform map is missing: $script:CodexCliPlatformsPath"
    }

    foreach ($rawLine in Get-Content -LiteralPath $script:CodexCliPlatformsPath) {
        $line = $rawLine.Trim()
        if ([string]::IsNullOrWhiteSpace($line) -or $line.StartsWith("#")) {
            continue
        }
        $fields = $line -split "\s+"
        if ($fields.Count -ne 5) {
            throw "invalid Codex CLI platform map entry: $rawLine"
        }
        if ($fields[0] -eq $Goos -and $fields[1] -eq $Goarch) {
            return @{ Os = $fields[2]; Arch = $fields[3]; Binary = $fields[4] }
        }
    }

    throw "unsupported bundled Codex CLI target: $Goos/$Goarch"
}

function Invoke-CodexCliDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$OutputPath
    )

    $maxAttempts = 3
    $partialPath = "$OutputPath.download"
    $previousProgressPreference = $ProgressPreference
    try {
        # Windows PowerShell renders Invoke-WebRequest progress updates slowly
        # for large release binaries. Keep the setting local to this download.
        $ProgressPreference = "SilentlyContinue"

        for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
            if (Test-Path -LiteralPath $partialPath) {
                Remove-Item -LiteralPath $partialPath -Force
            }
            try {
                Invoke-WebRequest -Uri $Uri -OutFile $partialPath -UseBasicParsing -ErrorAction Stop
                if (-not (Test-Path -LiteralPath $partialPath -PathType Leaf) -or (Get-Item -LiteralPath $partialPath).Length -eq 0) {
                    throw "downloaded Codex CLI file is empty"
                }
                Move-Item -LiteralPath $partialPath -Destination $OutputPath -Force
                return
            }
            catch {
                if ($attempt -eq $maxAttempts) {
                    throw "download bundled Codex CLI after $maxAttempts attempts: $($_.Exception.Message)"
                }
                Write-Warning "Codex CLI download attempt $attempt of $maxAttempts failed: $($_.Exception.Message); retrying"
                Start-Sleep -Seconds (2 * $attempt)
            }
        }
    }
    finally {
        $ProgressPreference = $previousProgressPreference
        if (Test-Path -LiteralPath $partialPath) {
            Remove-Item -LiteralPath $partialPath -Force
        }
    }
}

function Assert-CodexCliChecksum {
    param([string]$Path, [string]$Expected)

    if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $Expected) {
        throw "Codex CLI checksum mismatch: $(Split-Path -Leaf $Path)"
    }
}

function Fetch-CodexCli {
    param(
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch,
        [Parameter(Mandatory = $true)][string]$OutputDir
    )

    $target = Resolve-CodexCliDownloadTarget -Goos $Goos -Goarch $Goarch
    $version = (Get-Content -LiteralPath (Join-Path $RootDir "scripts/codex-cli-version.txt") -Raw).Trim()
    if ($version -notmatch '^\d+\.\d+\.\d+$') {
        throw "invalid bundled Codex CLI version: $version"
    }
    $artifact = "codex-package_$($version)_$($Goos)_$($Goarch).tar.gz"
    $checksums = @(Get-Content -LiteralPath (Join-Path $RootDir "scripts/codex-cli-checksums.txt") | ForEach-Object {
        $fields = $_.Trim() -split '\s+'
        if ($fields.Count -eq 2 -and $fields[1] -eq $artifact) { $fields[0] }
    })
    if ($checksums.Count -ne 1 -or $checksums[0] -notmatch '^[0-9a-f]{64}$') {
        throw "missing or invalid Codex CLI checksum: $artifact"
    }
    $downloadUrl = "$($script:CodexCliDownloadBaseUrl.TrimEnd('/'))/v$version/$artifact"
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("csgclaw-codex-" + [guid]::NewGuid().ToString("N"))
    Ensure-Directory -Path $tmpDir
    Ensure-Directory -Path $OutputDir

    try {
        Write-Host "fetching bundled Codex CLI $downloadUrl"
        $archivePath = Join-Path $tmpDir "codex.tar.gz"
        $extractDir = Join-Path $tmpDir "extract"
        Ensure-Directory -Path $extractDir
        Invoke-CodexCliDownload -Uri $downloadUrl -OutputPath $archivePath
        Assert-CodexCliChecksum -Path $archivePath -Expected $checksums[0]
        $tar = Get-CommandPathOrNull "tar"
        if ($null -eq $tar) {
            throw "missing required command: tar"
        }
        Invoke-Checked -FilePath $tar -Arguments @("-xzf", $archivePath, "-C", $extractDir)
        $binaryPath = Join-Path (Join-Path $extractDir "bin") $target.Binary
        if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
            throw "Codex archive did not contain expected binary: $($target.Binary)"
        }
        foreach ($entry in @("codex-package.json", "codex-resources", "codex-path")) {
            if (-not (Test-Path -LiteralPath (Join-Path $extractDir $entry))) {
                throw "Codex package did not contain expected entry: $entry"
            }
        }
        $packageRoot = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputDir))
        foreach ($entry in @("codex-resources", "codex-path")) {
            $destination = Join-Path $packageRoot $entry
            if (Test-Path -LiteralPath $destination) {
                Remove-Item -LiteralPath $destination -Recurse -Force
            }
            Copy-Item -LiteralPath (Join-Path $extractDir $entry) -Destination $destination -Recurse -Force
        }
        Copy-Item -LiteralPath (Join-Path $extractDir "codex-package.json") -Destination $packageRoot -Force
        Get-ChildItem -LiteralPath (Join-Path $extractDir "bin") | Copy-Item -Destination $OutputDir -Recurse -Force
    }
    finally {
        if (Test-Path -LiteralPath $tmpDir) {
            Remove-Item -LiteralPath $tmpDir -Recurse -Force
        }
    }
}

function Require-WebAssets {
    param([Parameter(Mandatory = $true)][string]$AppName)

    if ($AppName -ne "csgclaw") {
        return
    }

    $indexPath = Join-Path $script:WebStaticDistDir "index.html"
    if (Test-Path -LiteralPath $indexPath -PathType Leaf) {
        return
    }

    throw "missing Web UI build output: $indexPath.`nBuild the embedded Web UI assets before packaging $AppName."
}

function Invoke-PackageRelease {
    param(
        [Parameter(Mandatory = $true)][string]$AppName,
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch
    )

    Require-WebAssets -AppName $AppName
    Ensure-Directory -Path $script:DistDir

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("csgclaw-package-" + [guid]::NewGuid().ToString("N"))
    Ensure-Directory -Path $tmpDir

    try {
        $binaryName = Get-BinaryName -BaseName $AppName -Goos $Goos
        $binaryOutput = Join-Path $tmpDir $binaryName
        $archiveSourcePath = $binaryOutput
        $archiveInput = $binaryName

        if ($AppName -eq "csgclaw") {
            $bundleRoot = Join-Path $tmpDir $AppName
            $stageDir = Join-Path $bundleRoot "bin"
            Ensure-Directory -Path $stageDir
            $binaryOutput = Join-Path $stageDir $binaryName
            $archiveSourcePath = $bundleRoot
            $archiveInput = $AppName
            Set-Content -LiteralPath (Join-Path $bundleRoot ".csgclaw-bundle.json") -Value "{""app"":""csgclaw"",""layout"":""official-bundle"",""version"":""$($script:Version)""}" -NoNewline
        }

        $packagePath = if ($AppName -eq "csgclaw-cli") { "./cmd/csgclaw-cli" } else { "./cmd/csgclaw" }
        $ldflags = if ($AppName -eq "csgclaw-cli") { $script:CliLdflags } else { $script:Ldflags }
        $env = @{
            GOOS   = $Goos
            GOARCH = $Goarch
        }
        if (-not [string]::IsNullOrWhiteSpace($script:CgoEnabled)) {
            $env["CGO_ENABLED"] = $script:CgoEnabled
        }

        Invoke-GoBuild -OutputPath $binaryOutput -PackagePath $packagePath -Ldflags $ldflags -Env $env -Tags $script:GoBuildTags

        if ($AppName -eq "csgclaw") {
            $stageDir = Split-Path -Parent $binaryOutput
            Invoke-GoBuild -OutputPath (Join-Path $stageDir (Get-BinaryName -BaseName "csgclaw-cli" -Goos $Goos)) -PackagePath $script:HostCliCmdPath -Ldflags $script:CliLdflags -Env @{
                CGO_ENABLED = "0"
                GOOS        = $Goos
                GOARCH      = $Goarch
            }
        }


        if ($AppName -eq "csgclaw") {
            Fetch-CodexCli -Goos $Goos -Goarch $Goarch -OutputDir (Split-Path -Parent $binaryOutput)
        }

        $archiveBase = "${AppName}_$($script:Version)_${Goos}_${Goarch}"
        if ($Goos -eq "windows") {
            $archivePath = Join-Path $script:DistDir "$archiveBase.zip"
            if (Test-Path -LiteralPath $archivePath) {
                Remove-Item -LiteralPath $archivePath -Force
            }
            Compress-Archive -LiteralPath $archiveSourcePath -DestinationPath $archivePath -Force
            Write-Host "packaged $archivePath"
            return
        }

        $tar = Get-CommandPathOrNull "tar"
        if ($null -eq $tar) {
            throw "missing required command: tar"
        }

        $archivePath = Join-Path $script:DistDir "$archiveBase.tar.gz"
        if (Test-Path -LiteralPath $archivePath) {
            Remove-Item -LiteralPath $archivePath -Force
        }

        Push-Location $tmpDir
        try {
            Invoke-Checked -FilePath $tar -Arguments @("-czf", $archivePath, $archiveInput)
        }
        finally {
            Pop-Location
        }

        Write-Host "packaged $archivePath"
    }
    finally {
        if (Test-Path -LiteralPath $tmpDir) {
            Remove-Item -LiteralPath $tmpDir -Recurse -Force
        }
    }
}

function Invoke-TargetBuild {
    Write-Host "Starting full build: Web UI, companion host binaries."
    Invoke-TargetBuildWeb
    Invoke-TargetBuildServerBin
    Write-Host "Build complete."
}

function Invoke-TargetDesktopPackage {
    param(
        [string]$WindowsChannel = ""
    )

    if ($script:TargetOs -ne "windows") {
        throw "Windows desktop packaging must run with TARGET_OS=windows."
    }

    $desktopArch = switch ($script:TargetArch) {
        "amd64" { "x64" }
        "arm64" { "arm64" }
        default { throw "unsupported Windows desktop architecture: $($script:TargetArch)" }
    }

    Ensure-DesktopDeps
    Invoke-TargetBuildWeb -Summary
    Write-Host "Building desktop backend ($($script:TargetOs)/$($script:TargetArch))..."
    Invoke-DesktopBackendBundle -Goos $script:TargetOs -Goarch $script:TargetArch
    Write-Host "Building desktop packages (win32/$desktopArch)..."
    if (Test-Path -LiteralPath $script:DesktopMakeDir) {
        Remove-Item -LiteralPath $script:DesktopMakeDir -Recurse -Force
    }
    $packageEnvironment = @{
        CSGCLAW_DESKTOP_GOOS    = $script:TargetOs
        CSGCLAW_DESKTOP_GOARCH  = $script:TargetArch
        CSGCLAW_DESKTOP_ARCH    = $desktopArch
        CSGCLAW_DESKTOP_VERSION = $script:Version
    }
    if (-not [string]::IsNullOrWhiteSpace($WindowsChannel)) {
        $packageEnvironment["CSGCLAW_DESKTOP_WINDOWS_CHANNEL"] = $WindowsChannel
    }
    Invoke-Pnpm -WorkingDirectory $script:DesktopDir -Arguments @("--silent", "make", "--platform=win32", "--arch=$desktopArch") -Env $packageEnvironment

    $packages = @(
        Get-ChildItem -LiteralPath $script:DesktopMakeDir -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object {
                $_.Length -gt 0 -and
                ($_.Name -like "*-Setup.exe" -or $_.Extension -eq ".msix")
            }
    )
    if ($packages.Count -eq 0) {
        throw "Electron Forge completed without producing a Windows Setup.exe or MSIX package under $script:DesktopMakeDir."
    }

    Write-Host "Desktop packages ready:"
    $packages | ForEach-Object { Write-Host "  $($_.FullName)" }
}

function Invoke-TargetRun {
    Invoke-TargetBuild
    $serverBinary = Join-Path $script:BinDir (Get-BinaryName -BaseName "csgclaw" -Goos $script:TargetOs)
    Invoke-Checked -FilePath $serverBinary -Arguments @("serve") -Env @{ Path = "$script:BinDir;$env:Path" }
}

function Invoke-TargetClean {
    $packageRoot = Split-Path -Parent ([System.IO.Path]::GetFullPath($script:BinDir))
    foreach ($path in @($script:BinDir, $script:DistDir, $script:Gocache,
        (Join-Path $packageRoot "codex-resources"), (Join-Path $packageRoot "codex-path"), (Join-Path $packageRoot "codex-package.json"))) {
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Recurse -Force
        }
    }
}

function Invoke-TargetPackage {
    Invoke-TargetBuildWeb
    Invoke-PackageRelease -AppName $script:App -Goos $script:HostGoos -Goarch $script:HostGoarch
}

function Invoke-TargetPackageAll {
    Invoke-TargetBuild
    Invoke-PackageRelease -AppName "csgclaw" -Goos $script:HostGoos -Goarch $script:HostGoarch
    Invoke-PackageRelease -AppName "csgclaw-cli" -Goos $script:HostGoos -Goarch $script:HostGoarch
}

function Invoke-TargetRelease {
    Invoke-TargetBuildWeb
    foreach ($spec in @(
            @{ App = "csgclaw"; Goos = "darwin"; Goarch = "arm64" }
            @{ App = "csgclaw-cli"; Goos = "darwin"; Goarch = "arm64" }
            @{ App = "csgclaw"; Goos = "darwin"; Goarch = "amd64" }
            @{ App = "csgclaw-cli"; Goos = "darwin"; Goarch = "amd64" }
            @{ App = "csgclaw"; Goos = "linux"; Goarch = "amd64" }
            @{ App = "csgclaw-cli"; Goos = "linux"; Goarch = "amd64" }
            @{ App = "csgclaw"; Goos = "linux"; Goarch = "arm64" }
            @{ App = "csgclaw-cli"; Goos = "linux"; Goarch = "arm64" }
            @{ App = "csgclaw"; Goos = "windows"; Goarch = "amd64" }
            @{ App = "csgclaw-cli"; Goos = "windows"; Goarch = "amd64" }
        )) {
        Invoke-PackageRelease -AppName $spec.App -Goos $spec.Goos -Goarch $spec.Goarch
    }
}

$script:App = Get-EnvOrDefault -Name "APP" -Default "csgclaw"
$script:BinDir = Get-EnvOrDefault -Name "BIN_DIR" -Default (Join-Path $RootDir "bin")
$script:DistDir = Get-EnvOrDefault -Name "DIST_DIR" -Default (Join-Path $RootDir "dist")
$script:Gocache = Get-EnvOrDefault -Name "GOCACHE" -Default (Join-Path $RootDir ".gocache")
$script:VersionPkg = Get-EnvOrDefault -Name "VERSION_PKG" -Default "csgclaw/internal/version"
$script:Version = Get-EnvOrDefault -Name "VERSION" -Default (Resolve-Version)
$script:Commit = Get-EnvOrDefault -Name "COMMIT" -Default (Resolve-Commit)
$script:BuildTime = Get-EnvOrDefault -Name "BUILD_TIME" -Default ((Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"))
$script:Ldflags = Get-EnvOrDefault -Name "LDFLAGS" -Default "-X $($script:VersionPkg).Version=$($script:Version) -X $($script:VersionPkg).Commit=$($script:Commit) -X $($script:VersionPkg).BuildTime=$($script:BuildTime)"
$script:CliLdflags = Get-EnvOrDefault -Name "CLI_LDFLAGS" -Default "-s -w $($script:Ldflags)"
$script:CgoEnabled = Get-EnvOrDefault -Name "CGO_ENABLED" -Default "0"
$script:WebAppDir = Get-EnvOrDefault -Name "WEB_APP_DIR" -Default (Join-Path $RootDir "web/packages/app")
$script:WebStaticDistDir = Get-EnvOrDefault -Name "WEB_STATIC_DIST_DIR" -Default (Join-Path $RootDir "web/static-dist")
$script:DesktopDir = Get-EnvOrDefault -Name "DESKTOP_DIR" -Default (Join-Path $RootDir "desktop")
$script:DesktopMakeDir = Join-Path $script:DesktopDir "out/make"
$script:HostGoos = Resolve-GoEnv -Name "GOOS"
$script:HostGoarch = Resolve-GoEnv -Name "GOARCH"
$script:TargetOs = Get-EnvOrDefault -Name "TARGET_OS" -Default $script:HostGoos
$script:TargetArch = Get-EnvOrDefault -Name "TARGET_ARCH" -Default $script:HostGoarch
$script:GoBuildTags = Get-EnvOrDefault -Name "GO_BUILD_TAGS" -Default ""
$script:CodexCliDownloadBaseUrl = Get-EnvOrDefault -Name "CODEX_CLI_DOWNLOAD_BASE_URL" -Default "https://csgclaw.opencsg.com/codex-cli"
$script:CodexCliPlatformsPath = Join-Path $RootDir "scripts/codex-cli-platforms.txt"
$script:HostCliCmdPath = Get-EnvOrDefault -Name "HOST_CLI_CMD_PATH" -Default "./cmd/csgclaw-cli"

Push-Location $RootDir
try {
    switch ($Target) {
        "help" { Invoke-TargetHelp }
        "fmt" { Invoke-TargetFmt }
        "test" { Invoke-TargetTest }
        "check-web-toolchain" { Invoke-TargetCheckWebToolchain }
        "check-web-layout" { Ensure-WebLayout }
        "web-install" { Invoke-TargetWebInstall }
        "web-dev" { Invoke-TargetWebDev }
        "build-web" { Invoke-TargetBuildWeb }
        "build-server-bin" { Invoke-TargetBuildServerBin }
        "build" { Invoke-TargetBuild }
        "build-all" { Invoke-TargetBuild }
        "desktop-package" { Invoke-TargetDesktopPackage }
        "desktop-msix" { Invoke-TargetDesktopPackage -WindowsChannel "store" }
        "run" { Invoke-TargetRun }
        "package" { Invoke-TargetPackage }
        "package-all" { Invoke-TargetPackageAll }
        "release" { Invoke-TargetRelease }
        "clean" { Invoke-TargetClean }
        default { throw "unknown target: $Target" }
    }
}
finally {
    Pop-Location
}
