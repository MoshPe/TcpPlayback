# TCP Playback

A modern desktop application built with Wails for recording and replaying TCP data streams. This tool allows you to capture TCP traffic and replay it exactly as it was received.

## Features

- **TCP Recording**: Start a TCP server to listen for incoming connections and save all data to binary files
- **TCP Playback**: Stream recorded data back to any TCP server with exact timing and data integrity
- **Session Management**: Organize recordings by sessions with timestamps
- **Modern UI**: Beautiful, responsive interface with real-time status indicators
- **File Management**: View and manage all recorded files with easy selection for playback

## How It Works

### Recording Mode
1. Start the TCP server on a specified port
2. The application listens for incoming TCP connections
3. Each connection's data is saved to a separate binary file
4. Files are organized in session directories with timestamps

### Playback Mode
1. Select a recorded binary file from the list
2. Specify the target host and port for playback
3. The application connects to the target and streams the recorded data
4. Data is sent exactly as it was received, maintaining integrity

## Usage

### Recording TCP Data
1. Set the desired port number (default: 8080)
2. Click "🎙️ Start Recording" to begin the TCP server
3. Send TCP data to the server from your application
4. Click "⏹️ Stop Recording" when finished
5. Recorded files will appear in the "Recorded Files" section

### Playing Back TCP Data
1. Select a recorded file from the dropdown
2. Enter the target host and port for playback
3. Click "▶️ Start Playback" to begin streaming
4. The recorded data will be sent to the target server
5. Click "⏹️ Stop Playback" to stop streaming

## File Structure

```
recordings/
├── session_1703123456/
│   ├── connection_1.bin
│   ├── connection_2.bin
│   └── ...
└── session_1703123789/
    ├── connection_1.bin
    └── ...
```

## Building and Running

### Prerequisites
- Go 1.18+
- Node.js 16+
- Wails CLI

### Development
```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Install frontend dependencies
cd frontend
npm install

# Run in development mode
wails dev
```

### Building
```bash
# Build for your platform
wails build

# Build for specific platform
wails build -platform windows/amd64
wails build -platform darwin/universal
wails build -platform linux/amd64
```

## Use Cases

- **Testing**: Replay recorded network traffic for testing applications
- **Debugging**: Capture and replay problematic network scenarios
- **Load Testing**: Replay realistic traffic patterns for performance testing
- **Security Testing**: Replay captured traffic for security analysis
- **Development**: Test applications with consistent network data

## Technical Details

- **Backend**: Go with standard library TCP networking
- **Frontend**: React with TypeScript and modern CSS
- **Framework**: Wails for cross-platform desktop development
- **Data Format**: Raw binary files preserving exact byte sequences
- **Concurrency**: Handles multiple simultaneous connections during recording

## License

This project is open source and available under the MIT License.
