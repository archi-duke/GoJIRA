@echo off
cd /d "%~dp0"

echo ============================================
echo   GoJIRA - All Services Launcher
echo ============================================
echo.

REM Pre-check: backend binaries / venv
if not exist "Platform-API\platform-api.exe" (
    echo [ERROR] Platform-API\platform-api.exe missing.
    echo         Build: cd Platform-API ^&^& go build -o platform-api.exe .
    pause & exit /b 1
)
if not exist "GoJIRA-API\gojira-api.exe" (
    echo [ERROR] GoJIRA-API\gojira-api.exe missing.
    echo         Build: cd GoJIRA-API ^&^& go build -o gojira-api.exe .
    pause & exit /b 1
)
REM Pre-check: frontend node_modules
if not exist "Platform-App\node_modules" (
    echo [ERROR] Platform-App\node_modules missing.
    echo         Install: cd Platform-App ^&^& npm install
    pause & exit /b 1
)
if not exist "GoJIRA-App\node_modules" (
    echo [ERROR] GoJIRA-App\node_modules missing.
    echo         Install: cd GoJIRA-App ^&^& npm install
    pause & exit /b 1
)

REM ── [1/4] Platform-API (port 9000) ──
call :isPortInUse 9000
if errorlevel 1 (
    echo [1/4] Starting Platform-API ^(port 9000^)...
    start "Platform-API" /D "%CD%\Platform-API" cmd /k platform-api.exe
    timeout /t 3 /nobreak >nul
) else (
    echo [1/4] Platform-API already running on port 9000, skipping.
)

REM ── [2/4] GoJIRA-API (port 9001) ──
call :isPortInUse 9001
if errorlevel 1 (
    echo [2/4] Starting GoJIRA-API ^(port 9001^)...
    start "GoJIRA-API" /D "%CD%\GoJIRA-API" cmd /k gojira-api.exe
    timeout /t 3 /nobreak >nul
) else (
    echo [2/4] GoJIRA-API already running on port 9001, skipping.
)

REM ── [3/4] Platform-App (port 3000) ──
call :isPortInUse 3000
if errorlevel 1 (
    echo [3/4] Starting Platform-App ^(port 3000, Vite^)...
    start "Platform-App" /D "%CD%\Platform-App" cmd /k "set BROWSER=none && npm start"
    timeout /t 3 /nobreak >nul
) else (
    echo [3/4] Platform-App already running on port 3000, skipping.
)

REM ── [4/4] GoJIRA-App (port 3001) ──
call :isPortInUse 3001
if errorlevel 1 (
    echo [4/4] Starting GoJIRA-App ^(port 3001, Vite^)...
    start "GoJIRA-App" /D "%CD%\GoJIRA-App" cmd /k "set BROWSER=none && npm start"
    timeout /t 2 /nobreak >nul
) else (
    echo [4/4] GoJIRA-App already running on port 3001, skipping.
)

echo.
echo ============================================
echo   Launch sequence finished
echo ============================================
echo.
echo   [Backend]
echo   - Platform-API : http://localhost:9000
echo   - GoJIRA-API   : http://localhost:9001
echo.
echo   [Frontend - Vite dev server, ready in a few seconds]
echo   - Platform-App : http://localhost:3000
echo   - GoJIRA-App   : http://localhost:3001
echo.
echo   [External infra - see docs/shared-infra-handoff.md]
echo   - MongoDB : localhost:27017 (shared-infra / shared-mongo)
echo   - Gitea   : per deploy/.env GITEA_SERVER
echo.
echo To stop all: run stop-all.bat
pause
exit /b 0

REM ── Helper: port LISTENING check ──
REM   Returns errorlevel 0 if port in use, 1 if free.
:isPortInUse
netstat -aon | findstr LISTENING | findstr ":%~1 " >nul 2>&1
exit /b %errorlevel%
