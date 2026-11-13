Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Load only the checksum helper, without starting a complete product build.
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    (Join-Path $PSScriptRoot "build.ps1"), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -ne 0) { throw "build.ps1 did not parse" }
$helper = $ast.Find({ param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq "Assert-CodexCliChecksum"
}, $true)
if ($null -eq $helper) { throw "Checksum helper was not found" }
Invoke-Expression $helper.Extent.Text

# Reproduce the missing command from the Windows release environment.
function Get-FileHash { throw "Get-FileHash is unavailable in this test" }
$directory = Join-Path ([IO.Path]::GetTempPath()) ("csgclaw-checksum-" + [guid]::NewGuid().ToString("N"))
$null = [IO.Directory]::CreateDirectory($directory)
$path = Join-Path $directory "codex[package].tar.gz"
try {
    [IO.File]::WriteAllBytes($path, [Text.Encoding]::ASCII.GetBytes("abc"))
    Assert-CodexCliChecksum -Path $path -Expected "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
    $rejected = $false
    try {
        Assert-CodexCliChecksum -Path $path -Expected ("0" * 64)
    }
    catch {
        if ($_.Exception.Message -notlike "Codex CLI checksum mismatch:*") { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw "Incorrect checksum was accepted" }
    # A verification failure must also close the input file on Windows.
    $exclusive = [IO.File]::Open($path, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $exclusive.Dispose()
    Write-Host "Codex checksum verification accepts valid files, rejects corruption, and closes its stream without Get-FileHash."
}
finally {
    [IO.Directory]::Delete($directory, $true)
}
