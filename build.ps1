# Performance Report Generator - Build Script

Write-Host "Building Performance Report Generator..." -ForegroundColor Cyan

# Step 1: Build frontend
Write-Host "`n[1/3] Building Vue frontend..." -ForegroundColor Yellow
Set-Location "$PSScriptRoot\frontend"
npm run build
if ($LASTEXITCODE -ne 0) {
    Write-Host "Frontend build failed!" -ForegroundColor Red
    exit 1
}

# Step 2: Build Go backend
Write-Host "`n[2/3] Building Go backend..." -ForegroundColor Yellow
Set-Location "$PSScriptRoot"
go build -ldflags "-s -w" -o PerformanceReport.exe .
if ($LASTEXITCODE -ne 0) {
    Write-Host "Go build failed!" -ForegroundColor Red
    exit 1
}

Write-Host "`n[3/3] Build complete!" -ForegroundColor Green
Write-Host "Run .\PerformanceReport.exe to start the application" -ForegroundColor Cyan
