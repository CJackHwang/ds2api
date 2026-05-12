@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul
title DS2API - Dev Server

echo ============================================
echo   DS2API - Dev Server  (go run)
echo ============================================
echo.

set "ROOT=%~dp0"

:: ── Step 1: Check / auto-install Go ──────────────────────────────────────
where go >nul 2>&1
if errorlevel 1 (
    echo [SETUP] Go not found. Downloading and installing Go 1.26.3...
    powershell -NoProfile -ExecutionPolicy Bypass -Command ^
        "$msi = \"$env:TEMP\go-installer.msi\";" ^
        "$url = 'https://go.dev/dl/go1.26.3.windows-amd64.msi';" ^
        "Write-Host '[SETUP] Downloading...';" ^
        "try { Invoke-WebRequest -Uri $url -OutFile $msi -UseBasicParsing }" ^
        "catch { Write-Host '[ERROR]' $_.Exception.Message; exit 1 };" ^
        "Write-Host '[SETUP] Installing (Windows will prompt for Admin)...';" ^
        "Start-Process msiexec.exe -ArgumentList \"/i `\"$msi`\" /quiet /norestart\" -Verb RunAs -Wait;" ^
        "Remove-Item $msi -Force -ErrorAction SilentlyContinue;" ^
        "Write-Host '[OK] Go installed.'"
    if errorlevel 1 (
        echo [ERROR] Installation failed. Install manually from: https://go.dev/dl/
        pause & exit /b 1
    )
    :: Reload PATH in current session
    for /f "tokens=*" %%p in ('powershell -NoProfile -Command ^
        "[System.Environment]::GetEnvironmentVariable('PATH','Machine')"') do set "PATH=%%p;%PATH%"
    where go >nul 2>&1
    if errorlevel 1 (
        echo [INFO] Go installed. Please close and reopen this window to reload PATH.
        pause & exit /b 0
    )
)
for /f "tokens=3" %%v in ('go version') do set GO_VER=%%v
echo [OK] Go %GO_VER%

:: ── Step 2: Check config.json ─────────────────────────────────────────────
if not exist "%ROOT%config.json" (
    echo [SETUP] config.json not found. Copying from config.example.json...
    copy "%ROOT%config.example.json" "%ROOT%config.json" >nul
    echo [SETUP] config.json created. Fill in your DeepSeek account and API key.
    start "" notepad "%ROOT%config.json"
    pause & exit /b 0
)
echo [OK] config.json

:: ── Step 3: Read PORT from .env ───────────────────────────────────────────
set "PORT=5001"
if exist "%ROOT%.env" (
    for /f "usebackq tokens=1,2 delims==" %%a in ("%ROOT%.env") do (
        if "%%a"=="PORT" set "PORT=%%b"
    )
)

:: ── Step 4: Start server ──────────────────────────────────────────────────
echo.
echo   Admin : http://127.0.0.1:%PORT%/admin
echo   API   : http://127.0.0.1:%PORT%/v1
echo   Health: http://127.0.0.1:%PORT%/healthz
echo.
echo [INFO] Starting server... (Ctrl+C to stop)
echo ============================================
echo.

cd /d "%ROOT%"
set "DS2API_CONFIG_PATH=%ROOT%config.json"
set "LOG_LEVEL=INFO"
set "PORT=%PORT%"

go run ./cmd/ds2api

echo.
echo [INFO] Server stopped.
pause
