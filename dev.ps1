# Performance Report Generator - Development Script
# Runs the Go backend and Vue dev server simultaneously

Write-Host "Starting development servers..." -ForegroundColor Cyan

# Start Go backend in background
$goJob = Start-Job -ScriptBlock {
    Set-Location "d:\GoWorkspace\src\PerformanceReport"
    $env:GOPATH = "d:\GoWorkspace"
    # Create placeholder dist if needed
    if (-not (Test-Path "frontend\dist\index.html")) {
        New-Item -Path "frontend\dist" -ItemType Directory -Force | Out-Null
        "<!DOCTYPE html><html><body>Dev mode - use port 5173</body></html>" | Out-File "frontend\dist\index.html"
    }
    go run main.go -port 8765
}

Write-Host "Go backend starting on port 8765..." -ForegroundColor Green

# Start Vite dev server
Write-Host "Starting Vue dev server on port 5173..." -ForegroundColor Green
Set-Location "d:\GoWorkspace\src\PerformanceReport\frontend"
npm run dev
