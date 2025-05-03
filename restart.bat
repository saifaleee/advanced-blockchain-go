@echo off
echo === Blockchain System Restart Script ===

echo Stopping frontend processes...
taskkill /f /im node.exe >nul 2>&1
taskkill /f /im npm.exe >nul 2>&1

echo Stopping backend processes...
taskkill /f /im advanced-blockchain-go.exe >nul 2>&1
taskkill /f /im go.exe >nul 2>&1

timeout /t 2 /nobreak >nul

echo Building and starting blockchain server...
go build
start /min advanced-blockchain-go.exe

echo Waiting for blockchain to initialize...
timeout /t 5 /nobreak >nul

echo Starting frontend...
cd frontend
start /min cmd /c npm start

echo System restart complete!
echo Frontend URL: http://localhost:3000
echo Backend API: http://localhost:8080/api

echo.
echo Press any key to exit...
pause >nul 