# TCP Client Test Script for TCP Playback
# This script sends test data to the TCP recording server

param(
    [string]$ServerHost = "localhost",
    [int]$Port = 8080,
    [string]$Message = "Hello TCP Playback from PowerShell!",
    [int]$Delay = 1000
)

Write-Host "🔌 TCP Client Test Script" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host "Target: $ServerHost`:$Port" -ForegroundColor Yellow
Write-Host "Message: $Message" -ForegroundColor Yellow
Write-Host "Delay: $Delay ms" -ForegroundColor Yellow
Write-Host ""

try {
    Write-Host "📡 Connecting to $ServerHost`:$Port..." -ForegroundColor Green
    
    # Create TCP client
    $client = New-Object System.Net.Sockets.TcpClient
    
    # Connect to server
    $client.Connect($ServerHost, $Port)
    
    if ($client.Connected) {
        Write-Host "✅ Connected successfully!" -ForegroundColor Green
        
        # Get network stream
        $stream = $client.GetStream()
        
        # Convert message to bytes
        $messageBytes = [System.Text.Encoding]::UTF8.GetBytes($Message)
        
        # Send data
        Write-Host "📤 Sending data..." -ForegroundColor Yellow
        $stream.Write($messageBytes, 0, $messageBytes.Length)
        $stream.Flush()
        
        Write-Host "✅ Data sent successfully!" -ForegroundColor Green
        Write-Host "📊 Bytes sent: $($messageBytes.Length)" -ForegroundColor Cyan
        
        # Wait a bit before closing
        Start-Sleep -Milliseconds $Delay
        
        # Close connection
        $stream.Close()
        $client.Close()
        
        Write-Host "🔌 Connection closed." -ForegroundColor Yellow
        
    } else {
        Write-Host "❌ Failed to connect!" -ForegroundColor Red
    }
    
} catch {
    Write-Host "❌ Error: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "💡 Make sure the TCP Playback application is running and recording on port $Port" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "🎯 Test completed!" -ForegroundColor Cyan 