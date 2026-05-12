@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul
title DS2API - Dev Server

echo ============================================
echo   DS2API - Dev Server  (go run)
echo ============================================
echo.

set "ROOT=%~dp0"

:: ── Bước 1: Kiểm tra / tự cài Go ─────────────────────────────────────────
where go >nul 2>&1
if errorlevel 1 (
    echo [SETUP] Khong tim thay Go. Dang tu dong tai va cai dat Go 1.26.3...
    powershell -NoProfile -ExecutionPolicy Bypass -Command ^
        "$msi = \"$env:TEMP\go-installer.msi\";" ^
        "$url = 'https://go.dev/dl/go1.26.3.windows-amd64.msi';" ^
        "Write-Host '[SETUP] Dang tai...';" ^
        "try { Invoke-WebRequest -Uri $url -OutFile $msi -UseBasicParsing }" ^
        "catch { Write-Host '[LOI]' $_.Exception.Message; exit 1 };" ^
        "Write-Host '[SETUP] Dang cai dat (Windows se hoi quyen Admin)...';" ^
        "Start-Process msiexec.exe -ArgumentList \"/i `\"$msi`\" /quiet /norestart\" -Verb RunAs -Wait;" ^
        "Remove-Item $msi -Force -ErrorAction SilentlyContinue;" ^
        "Write-Host '[OK] Cai dat xong.'"
    if errorlevel 1 (
        echo [LOI] Cai dat that bai. Cai thu cong tai: https://go.dev/dl/
        pause & exit /b 1
    )
    :: Reload PATH trong session hien tai
    for /f "tokens=*" %%p in ('powershell -NoProfile -Command ^
        "[System.Environment]::GetEnvironmentVariable('PATH','Machine')"') do set "PATH=%%p;%PATH%"
    where go >nul 2>&1
    if errorlevel 1 (
        echo [INFO] Go da cai xong. Dong va mo lai cua so nay de PATH cap nhat.
        pause & exit /b 0
    )
)
for /f "tokens=3" %%v in ('go version') do set GO_VER=%%v
echo [OK] Go %GO_VER%

:: ── Bước 2: Kiểm tra config.json ─────────────────────────────────────────
if not exist "%ROOT%config.json" (
    echo [SETUP] Chua co config.json, sao chep tu config.example.json...
    copy "%ROOT%config.example.json" "%ROOT%config.json" >nul
    echo [SETUP] Da tao config.json. Hay dien tai khoan DeepSeek va API key.
    start "" notepad "%ROOT%config.json"
    pause & exit /b 0
)
echo [OK] config.json

:: ── Bước 3: Đọc PORT từ .env ─────────────────────────────────────────────
set "PORT=5001"
if exist "%ROOT%.env" (
    for /f "usebackq tokens=1,2 delims==" %%a in ("%ROOT%.env") do (
        if "%%a"=="PORT" set "PORT=%%b"
    )
)

:: ── Bước 4: Chạy server ──────────────────────────────────────────────────
echo.
echo   Admin : http://127.0.0.1:%PORT%/admin
echo   API   : http://127.0.0.1:%PORT%/v1
echo   Health: http://127.0.0.1:%PORT%/healthz
echo.
echo [INFO] Khoi dong server... (Ctrl+C de dung)
echo ============================================
echo.

cd /d "%ROOT%"
set "DS2API_CONFIG_PATH=%ROOT%config.json"
set "LOG_LEVEL=INFO"
set "PORT=%PORT%"

go run ./cmd/ds2api

echo.
echo [INFO] Server da dung.
pause
