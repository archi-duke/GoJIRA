@echo off
cd /d "%~dp0"

echo ============================================
echo   GoJIRA - Stop All Services
echo ============================================
echo.

echo [1/4] Stopping Platform-App (window title)...
taskkill /FI "WINDOWTITLE eq Platform-App*" /T /F >nul 2>&1

echo [2/4] Stopping GoJIRA-App (window title)...
taskkill /FI "WINDOWTITLE eq GoJIRA-App*" /T /F >nul 2>&1

echo [3/4] Stopping GoJIRA-API (window title)...
taskkill /FI "WINDOWTITLE eq GoJIRA-API*" /T /F >nul 2>&1

echo [4/4] Stopping Platform-API (window title)...
taskkill /FI "WINDOWTITLE eq Platform-API*" /T /F >nul 2>&1

echo.
echo ============================================
echo   Cleanup: orphan processes still holding ports
echo ============================================

call :killPort 3000 Platform-App
call :killPort 3001 GoJIRA-App
call :killPort 9000 Platform-API
call :killPort 9001 GoJIRA-API

echo.
echo All services stopped.
pause
exit /b 0

:killPort
set "_port=%~1"
set "_name=%~2"
for /f "tokens=5" %%a in ('netstat -aon ^| findstr LISTENING ^| findstr ":%_port% "') do (
    echo   port %_port% ^(%_name%^) held by PID %%a -- killing
    taskkill /F /PID %%a >nul 2>&1
)
exit /b 0
