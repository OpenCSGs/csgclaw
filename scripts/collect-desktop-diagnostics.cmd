@echo off
setlocal

set "SCRIPT_DIR=%~dp0"
set "POWERSHELL_EXE=%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe"
if not exist "%SCRIPT_DIR%collect-desktop-diagnostics.ps1" (
  echo CSGClaw diagnostics script was not found: "%SCRIPT_DIR%collect-desktop-diagnostics.ps1" 1>&2
  exit /b 1
)
if not exist "%POWERSHELL_EXE%" (
  echo Windows PowerShell was not found: "%POWERSHELL_EXE%" 1>&2
  exit /b 1
)
"%POWERSHELL_EXE%" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%SCRIPT_DIR%collect-desktop-diagnostics.ps1" %*
exit /b %ERRORLEVEL%
