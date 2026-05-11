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
if not exist "Agent-Service\venv\Scripts\activate.bat" (
    echo [ERROR] Agent-Service\venv missing.
    echo         Create: cd Agent-Service ^&^& python -m venv venv ^&^& venv\Scripts\activate ^&^& pip install -r requirements.txt
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

REM Pre-check: WSL
where wsl >nul 2>&1
if errorlevel 1 (
    echo [ERROR] WSL not found in PATH.
    pause & exit /b 1
)

REM ── [1/6] OpenCode in WSL (port 4096) ──
call :isPortInUse 4096
if errorlevel 1 (
    echo [1/6] Starting OpenCode in WSL ^(port 4096^)...
    start "OpenCode Server" wsl bash -lc "opencode serve --port 4096 --hostname 0.0.0.0"
    timeout /t 5 /nobreak >nul
) else (
    echo [1/6] OpenCode already running on port 4096, skipping.
)

REM ── [2/6] Platform-API (port 9000) ──
call :isPortInUse 9000
if errorlevel 1 (
    echo [2/6] Starting Platform-API ^(port 9000^)...
    start "Platform-API" /D "%CD%\Platform-API" cmd /k platform-api.exe
    timeout /t 3 /nobreak >nul
) else (
    echo [2/6] Platform-API already running on port 9000, skipping.
)

REM ── [3/6] GoJIRA-API (port 9001) ──
call :isPortInUse 9001
if errorlevel 1 (
    echo [3/6] Starting GoJIRA-API ^(port 9001^)...
    start "GoJIRA-API" /D "%CD%\GoJIRA-API" cmd /k gojira-api.exe
    timeout /t 3 /nobreak >nul
) else (
    echo [3/6] GoJIRA-API already running on port 9001, skipping.
)

REM ── [4/6] Agent-Service (port 9002) ──
call :isPortInUse 9002
if errorlevel 1 (
    echo [4/6] Starting Agent-Service ^(port 9002^)...
    start "Agent-Service" /D "%CD%\Agent-Service" cmd /k "venv\Scripts\activate && python main.py"
    timeout /t 2 /nobreak >nul
) else (
    echo [4/6] Agent-Service already running on port 9002, skipping.
)

REM ── [5/6] Platform-App (port 3000) ──
call :isPortInUse 3000
if errorlevel 1 (
    echo [5/6] Starting Platform-App ^(port 3000^)... initial webpack compile may take 30s-2min
    start "Platform-App" /D "%CD%\Platform-App" cmd /k "set BROWSER=none && npm start"
    timeout /t 3 /nobreak >nul
) else (
    echo [5/6] Platform-App already running on port 3000, skipping.
)

REM ── [6/6] GoJIRA-App (port 3001) ──
call :isPortInUse 3001
if errorlevel 1 (
    echo [6/6] Starting GoJIRA-App ^(port 3001^)...
    start "GoJIRA-App" /D "%CD%\GoJIRA-App" cmd /k "set BROWSER=none && npm start"
    timeout /t 2 /nobreak >nul
) else (
    echo [6/6] GoJIRA-App already running on port 3001, skipping.
)

echo.
echo ============================================
echo   Launch sequence finished
echo ============================================
echo.
echo   [Backend]
echo   - OpenCode     : http://localhost:4096   (WSL)
echo   - Platform-API : http://localhost:9000
echo   - GoJIRA-API   : http://localhost:9001
echo   - Agent-Service: http://localhost:9002
echo.
echo   [Frontend - wait for "Compiled successfully"]
echo   - Platform-App : http://localhost:3000
echo   - GoJIRA-App   : http://localhost:3001
echo.
echo   [External infra]
echo   - MongoDB : 10.227.37.89:44330
echo   - Gitea   : 10.227.37.92:44350
echo.
echo To stop all: run stop-all.bat
pause
exit /b 0

REM ── Helper: port LISTENING check ──
REM   Returns errorlevel 0 if port in use, 1 if free.
:isPortInUse
netstat -aon | findstr LISTENING | findstr ":%~1 " >nul 2>&1
exit /b %errorlevel%
