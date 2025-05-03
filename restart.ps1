Write-Host "=== Blockchain System Restart Script ===" -ForegroundColor Cyan

# Kill frontend processes (Node.js and related)
Write-Host "Stopping frontend processes..." -ForegroundColor Yellow
Get-Process -Name "node" -ErrorAction SilentlyContinue | Stop-Process -Force
Get-Process -Name "npm" -ErrorAction SilentlyContinue | Stop-Process -Force

# Kill backend processes (Go blockchain server)
Write-Host "Stopping backend processes..." -ForegroundColor Yellow
Get-Process -Name "advanced-blockchain-go" -ErrorAction SilentlyContinue | Stop-Process -Force
Get-Process -Name "go" -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -match "main.go" } | Stop-Process -Force

# Wait for processes to terminate
Start-Sleep -Seconds 2

# Build and start the backend
Write-Host "Building and starting blockchain server..." -ForegroundColor Green
Start-Process -FilePath "go" -ArgumentList "build" -Wait -NoNewWindow
Start-Process -FilePath ".\advanced-blockchain-go.exe" -WindowStyle Minimized

# Wait for backend to initialize
Write-Host "Waiting for blockchain to initialize..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

# Start the frontend
Write-Host "Starting frontend..." -ForegroundColor Green
Set-Location -Path ".\frontend"
Start-Process -FilePath "cmd" -ArgumentList "/c npm start" -WindowStyle Minimized

Write-Host "System restart complete!" -ForegroundColor Cyan
Write-Host "Frontend URL: http://localhost:3000" -ForegroundColor Green
Write-Host "Backend API: http://localhost:8080/api" -ForegroundColor Green 