@echo off
echo ========================================================
echo Building Monthly Performance Report Generator
echo ========================================================

echo [1/2] Building Vue 3 Frontend...
cd frontend
call npm run build
if %ERRORLEVEL% NEQ 0 (
    echo Frontend build failed!
    pause
    exit /b %ERRORLEVEL%
)
cd ..

echo [2/2] Building Go Executable...
go build -ldflags "-s -w" -o PerformanceReport.exe .
if %ERRORLEVEL% NEQ 0 (
    echo Go build failed!
    pause
    exit /b %ERRORLEVEL%
)

echo ========================================================
echo Build Succeeded! Executable: PerformanceReport.exe
echo ========================================================
pause
