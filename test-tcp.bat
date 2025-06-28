@echo off
echo 🔌 TCP Playback Test Client
echo ================================
echo.

REM Check if PowerShell is available
powershell -Command "Write-Host 'PowerShell is available'" >nul 2>&1
if errorlevel 1 (
    echo ❌ PowerShell is not available on this system
    pause
    exit /b 1
)

REM Run the PowerShell script
echo 🚀 Starting TCP test...
powershell -ExecutionPolicy Bypass -File "test-tcp-client.ps1" %*

REM powershell -ExecutionPolicy Bypass -File "test-tcp-client.ps1" -ServerHost "localhost" -Port 8080 -Message "Custom test message!" -Delay 2000

echo.
echo 🎯 Test completed! Press any key to exit...
pause >nul 