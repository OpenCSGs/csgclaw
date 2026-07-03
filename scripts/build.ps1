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
    if (-not (Test-Path -LiteralPath (Join-Path $script:WebAppDir "pnpm-lock.yaml") -PathType Leaf)) {
        throw "Web UI pnpm lockfile is missing: $(Join-Path $script:WebAppDir 'pnpm-lock.yaml'). Restore the lockfile before building the Web UI."
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
    $node = Get-CommandPathOrNull "node"
    if ($null -eq $node) {
        throw "Node.js >=22.13.0 and <25 is required for the Web UI. Install a supported Node.js version first."
    }

    $nodeVersion = Get-CommandOutput -FilePath $node -Arguments @("--version")
    if ([string]::IsNullOrWhiteSpace($nodeVersion) -or -not (Test-NodeSupported -Version $nodeVersion)) {
        throw "Node.js >=22.13.0 and <25 is required for the Web UI; current node is $nodeVersion."
    }

    $runner = Resolve-PnpmRunner
    Write-Host "Web toolchain OK: Node.js $nodeVersion, pnpm $($runner.Version)"
    return $runner
}

function Invoke-Pnpm {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    $runner = Resolve-PnpmRunner
    Invoke-Checked -FilePath $runner.FilePath -Arguments ($runner.Prefix + $Arguments)
}

function Ensure-WebDeps {
    Ensure-WebLayout
    Assert-WebToolchain | Out-Null

    $viteUnix = Join-Path $script:WebAppDir "node_modules/.bin/vite"
    $viteWindows = Join-Path $script:WebAppDir "node_modules/.bin/vite.cmd"
    if ((Test-Path -LiteralPath $viteUnix -PathType Leaf) -or (Test-Path -LiteralPath $viteWindows -PathType Leaf)) {
        return
    }

    Write-Host "Web UI dependencies are missing; running web-install before build."
    Invoke-TargetWebInstall
}

function Invoke-TargetHelp {
    @(
        "scripts\build.cmd build                         - Windows wrapper; same as build with execution-policy bypass"
        "powershell -File scripts/build.ps1 build        - build Web UI and binaries, install the Linux sandbox CLI under ~/.csgclaw/sandbox-tools"
        "powershell -File scripts/build.ps1 build-all    - same as build"
        "powershell -File scripts/build.ps1 fmt          - format Go files"
        "powershell -File scripts/build.ps1 test         - run go test ./..."
        "powershell -File scripts/build.ps1 web-install  - install Web UI dependencies"
        "powershell -File scripts/build.ps1 web-dev      - run Vite Web UI dev server"
        "powershell -File scripts/build.ps1 build-web    - build Web UI app into web/static-dist"
        "powershell -File scripts/build.ps1 build-server-bin - build bin/csgclaw and the host-platform bin/csgclaw-cli"
        "powershell -File scripts/build.ps1 install-sandbox-cli - build Linux csgclaw-cli into ~/.csgclaw/sandbox-tools"
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
    Assert-WebToolchain | Out-Null
}

function Invoke-TargetWebInstall {
    Ensure-WebLayout
    Assert-WebToolchain | Out-Null
    Write-Host "Installing Web UI dependencies in $script:WebAppDir."
    Write-Host "If this appears stuck on registry downloads, check npm registry network/proxy access."
    Invoke-Pnpm -Arguments @("--dir", $script:WebAppDir, "install", "--frozen-lockfile")
}

function Invoke-TargetWebDev {
    Ensure-WebDeps
    Invoke-Pnpm -Arguments @("--dir", $script:WebAppDir, "dev")
}

function Invoke-TargetBuildWeb {
    Write-Host "Building Web UI into $script:WebStaticDistDir."
    Ensure-WebDeps
    Ensure-Directory -Path $script:WebStaticDistDir
    Invoke-Pnpm -Arguments @("--dir", $script:WebAppDir, "build")
    $indexPath = Join-Path $script:WebStaticDistDir "index.html"
    if (-not (Test-Path -LiteralPath $indexPath -PathType Leaf)) {
        throw "Web UI build did not produce $indexPath."
    }
}

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory = $true)][string]$OutputPath,
        [Parameter(Mandatory = $true)][string]$PackagePath,
        [Parameter(Mandatory = $true)][string]$Ldflags,
        [hashtable]$Env = @{},
        [string]$Tags = ""
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

    Write-Host "Building $OutputPath from $PackagePath."
    Invoke-Checked -FilePath $go -Arguments $args -Env $baseEnv
}

function Invoke-TargetBuildServerBin {
    Ensure-Directory -Path $script:BinDir

    $serverBinary = Join-Path $script:BinDir (Get-BinaryName -BaseName "csgclaw" -Goos $script:TargetOs)
    $cliBinary = Join-Path $script:BinDir (Get-BinaryName -BaseName "csgclaw-cli" -Goos $script:TargetOs)

    Invoke-GoBuild -OutputPath $serverBinary -PackagePath "./cmd/csgclaw" -Ldflags $script:Ldflags
    Invoke-GoBuild -OutputPath $cliBinary -PackagePath "./cmd/csgclaw-cli" -Ldflags $script:CliLdflags -Env @{
        CGO_ENABLED = $script:CgoEnabled
        GOOS        = $script:TargetOs
        GOARCH      = $script:TargetArch
    }
}

function Invoke-TargetInstallSandboxCli {
    Ensure-Directory -Path $script:SandboxToolsDir
    Invoke-GoBuild -OutputPath $script:SandboxCliBin -PackagePath "./cmd/csgclaw-cli" -Ldflags $script:CliLdflags -Env @{
        CGO_ENABLED = "0"
        GOOS        = "linux"
        GOARCH      = $script:TargetArch
    }
}

function Supports-BoxLiteBundle {
    param(
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch
    )

    return ($Goos -eq "darwin" -and $Goarch -eq "arm64") -or
        ($Goos -eq "linux" -and $Goarch -eq "amd64") -or
        ($Goos -eq "linux" -and $Goarch -eq "arm64")
}

function Resolve-IncludeBoxlite {
    param(
        [Parameter(Mandatory = $true)][string]$AppName,
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch
    )

    if (-not [string]::IsNullOrWhiteSpace($script:IncludeBoxlite)) {
        return $script:IncludeBoxlite
    }

    if (-not [string]::IsNullOrWhiteSpace($script:PackageMode)) {
        switch ($script:PackageMode) {
            "bundled-boxlite-cli" { return "1" }
            "legacy-single-binary" { return "0" }
            default { throw "unsupported PACKAGE_MODE: $($script:PackageMode)" }
        }
    }

    if ($AppName -eq "csgclaw" -and (Supports-BoxLiteBundle -Goos $Goos -Goarch $Goarch)) {
        return "1"
    }

    return "0"
}

function Resolve-BoxLiteTargetSuffix {
    param(
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch
    )

    switch ("$Goos/$Goarch") {
        "darwin/arm64" { return "aarch64-apple-darwin" }
        "linux/amd64" { return "x86_64-unknown-linux-gnu" }
        "linux/arm64" { return "aarch64-unknown-linux-gnu" }
        default { throw "unsupported boxlite-cli target: $Goos/$Goarch" }
    }
}

function Fetch-BoxLiteCli {
    param(
        [Parameter(Mandatory = $true)][string]$Goos,
        [Parameter(Mandatory = $true)][string]$Goarch,
        [Parameter(Mandatory = $true)][string]$OutputDir
    )

    $suffix = Resolve-BoxLiteTargetSuffix -Goos $Goos -Goarch $Goarch
    $archiveName = "boxlite-cli-$($script:BoxliteCliVersion)-$suffix.tar.gz"
    $downloadUrl = "$($script:BoxliteCliBaseUrl)/$($script:BoxliteCliVersion)/$archiveName"

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("csgclaw-boxlite-" + [guid]::NewGuid().ToString("N"))
    Ensure-Directory -Path $tmpDir
    $archivePath = Join-Path $tmpDir $archiveName
    $extractDir = Join-Path $tmpDir "extract"
    Ensure-Directory -Path $extractDir
    Ensure-Directory -Path $OutputDir

    try {
        Write-Host "fetching $downloadUrl"
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        $tar = Get-CommandPathOrNull "tar"
        if ($null -eq $tar) {
            throw "missing required command: tar"
        }

        Invoke-Checked -FilePath $tar -Arguments @("-xzf", $archivePath, "-C", $extractDir)

        $boxlite = Get-ChildItem -Path $extractDir -Recurse -File | Where-Object { $_.Name -eq "boxlite" } | Select-Object -First 1
        if ($null -eq $boxlite) {
            throw "boxlite binary not found in $archiveName"
        }

        Copy-Item -LiteralPath $boxlite.FullName -Destination (Join-Path $OutputDir "boxlite") -Force
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

    $includeBoxlite = Resolve-IncludeBoxlite -AppName $AppName -Goos $Goos -Goarch $Goarch
    if ($includeBoxlite -notin @("0", "1")) {
        throw "INCLUDE_BOXLITE must be 0 or 1, got: $includeBoxlite"
    }
    if ($AppName -ne "csgclaw" -and $includeBoxlite -eq "1") {
        throw "INCLUDE_BOXLITE=1 is only supported for APP=csgclaw"
    }
    if ($AppName -eq "csgclaw" -and $includeBoxlite -eq "1" -and -not (Supports-BoxLiteBundle -Goos $Goos -Goarch $Goarch)) {
        throw "bundled boxlite is not supported for $Goos/$Goarch"
    }

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
            $sandboxCliDir = Join-Path (Split-Path -Parent $binaryOutput) "csgclaw_dir"
            Ensure-Directory -Path $sandboxCliDir
            Invoke-GoBuild -OutputPath (Join-Path $sandboxCliDir "csgclaw-cli") -PackagePath $script:SandboxCliCmdPath -Ldflags $script:CliLdflags -Env @{
                CGO_ENABLED = "0"
                GOOS        = "linux"
                GOARCH      = $Goarch
            }
        }

        if ($AppName -eq "csgclaw" -and $includeBoxlite -eq "1") {
            Fetch-BoxLiteCli -Goos $Goos -Goarch $Goarch -OutputDir (Split-Path -Parent $binaryOutput)
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
    Write-Host "Starting full build: Web UI, host binaries, and Linux sandbox CLI."
    Invoke-TargetBuildWeb
    Invoke-TargetBuildServerBin
    Invoke-TargetInstallSandboxCli
    Write-Host "Build complete."
}

function Invoke-TargetRun {
    Invoke-TargetBuild
    $serverBinary = Join-Path $script:BinDir (Get-BinaryName -BaseName "csgclaw" -Goos $script:TargetOs)
    Invoke-Checked -FilePath $serverBinary -Arguments @("serve")
}

function Invoke-TargetClean {
    foreach ($path in @($script:BinDir, $script:DistDir, $script:Gocache)) {
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
            @{ App = "csgclaw"; Goos = "linux"; Goarch = "amd64" }
            @{ App = "csgclaw-cli"; Goos = "linux"; Goarch = "amd64" }
            @{ App = "csgclaw"; Goos = "linux"; Goarch = "arm64" }
            @{ App = "csgclaw-cli"; Goos = "linux"; Goarch = "arm64" }
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
$script:WebAppDir = Get-EnvOrDefault -Name "WEB_APP_DIR" -Default (Join-Path $RootDir "web/app")
$script:WebStaticDistDir = Get-EnvOrDefault -Name "WEB_STATIC_DIST_DIR" -Default (Join-Path $RootDir "web/static-dist")
$script:SandboxToolsDir = Get-EnvOrDefault -Name "SANDBOX_TOOLS_DIR" -Default (Join-Path $HOME ".csgclaw/sandbox-tools")
$script:SandboxCliBin = Get-EnvOrDefault -Name "SANDBOX_CLI_BIN" -Default (Join-Path $script:SandboxToolsDir "csgclaw-cli")
$script:HostGoos = Resolve-GoEnv -Name "GOOS"
$script:HostGoarch = Resolve-GoEnv -Name "GOARCH"
$script:TargetOs = Get-EnvOrDefault -Name "TARGET_OS" -Default $script:HostGoos
$script:TargetArch = Get-EnvOrDefault -Name "TARGET_ARCH" -Default $script:HostGoarch
$script:GoBuildTags = Get-EnvOrDefault -Name "GO_BUILD_TAGS" -Default ""
$script:PackageMode = Get-EnvOrDefault -Name "PACKAGE_MODE" -Default ""
$script:IncludeBoxlite = Get-EnvOrDefault -Name "INCLUDE_BOXLITE" -Default ""
$script:BoxliteCliVersion = Get-EnvOrDefault -Name "BOXLITE_CLI_VERSION" -Default "v0.9.0"
$script:BoxliteCliBaseUrl = Get-EnvOrDefault -Name "BOXLITE_CLI_BASE_URL" -Default "https://github.com/boxlite-ai/boxlite/releases/download"
$script:SandboxCliCmdPath = Get-EnvOrDefault -Name "SANDBOX_CLI_CMD_PATH" -Default "./cmd/csgclaw-cli"

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
        "install-sandbox-cli" { Invoke-TargetInstallSandboxCli }
        "build" { Invoke-TargetBuild }
        "build-all" { Invoke-TargetBuild }
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
