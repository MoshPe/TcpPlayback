package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// App struct
type App struct {
	ctx context.Context

	// TCP Server for recording
	server      net.Listener
	serverAddr  string
	isRecording bool
	recordMutex sync.Mutex

	// TCP Client for playback
	client    net.Conn
	isPlaying bool
	playMutex sync.Mutex

	// Playback configuration
	playbackConfig struct {
		initDelay   time.Duration
		repeat      int
		repeatDelay time.Duration
		loopOnEnd   bool
	}

	// Recording session info
	currentSession    string
	customSessionName string
	recordedFiles     []string

	// Recordings path configuration
	recordingsPath string // Configurable path for recordings

	// Recording configuration
	recordingConfig struct {
		magicWord       string
		messagesPerFile int
		maxFileSize     int64 // in bytes
	}

	// Logging
	logEntries []LogEntry
	logMutex   sync.RWMutex
}

// LogEntry represents a single log entry for TCP activity
type LogEntry struct {
	Time      time.Time `json:"time"`
	FromIP    string    `json:"fromIP"`
	FromPort  int       `json:"fromPort"`
	ToAddress string    `json:"toAddress"`
	ToPort    int       `json:"toPort"`
	Method    string    `json:"method"`
	Data      string    `json:"data"`
	Bytes     int64     `json:"bytes"`
}

// FileInfo represents information about a recorded file
type FileInfo struct {
	Path            string `json:"path"`
	Session         string `json:"session"`
	FileName        string `json:"fileName"`
	Size            int64  `json:"size"`
	ModTime         string `json:"modTime"`
	MagicWord       string `json:"magicWord"`
	MessagesPerFile int    `json:"messagesPerFile"`
	MaxFileSize     int64  `json:"maxFileSize"`
	MessageCount    int    `json:"messageCount"`
	IsOpen          bool   `json:"isOpen"`
}

// FileHeader represents the metadata stored at the beginning of recorded files
type FileHeader struct {
	MagicWord       string `json:"magicWord"`
	MessagesPerFile int    `json:"messagesPerFile"`
	MaxFileSize     int64  `json:"maxFileSize"`
	RecordedAt      string `json:"recordedAt"`
	Version         int    `json:"version"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SetSessionName sets a custom name for the next recording session
func (a *App) SetSessionName(name string) {
	a.customSessionName = name
}

// StartRecording starts a TCP server to record incoming data
func (a *App) StartRecording(port int) error {
	fmt.Printf("=== STARTING RECORDING ===\n")
	fmt.Printf("Port: %d\n", port)
	fmt.Printf("Current recording config:\n")
	fmt.Printf("  Magic Word: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	fmt.Printf("  Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	fmt.Printf("  Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	fmt.Printf("  Max File Size: %d\n", a.recordingConfig.maxFileSize)
	fmt.Printf("========================\n")

	a.recordMutex.Lock()
	defer a.recordMutex.Unlock()

	if a.isRecording {
		return fmt.Errorf("already recording")
	}

	// Create recordings directory if it doesn't exist
	recordingsDir := a.GetRecordingsPath()
	if err := os.MkdirAll(recordingsDir, 0755); err != nil {
		return fmt.Errorf("failed to create recordings directory: %v", err)
	}

	// Start TCP server
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server: %v", err)
	}

	a.server = listener
	a.serverAddr = addr
	a.isRecording = true

	// Generate session name
	if a.customSessionName != "" {
		a.currentSession = a.customSessionName
	} else {
		a.currentSession = fmt.Sprintf("session_%d", time.Now().Unix())
	}

	// Create session directory
	sessionDir := filepath.Join(recordingsDir, a.currentSession)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %v", err)
	}

	fmt.Printf("Recording started on %s with session: %s\n", addr, a.currentSession)

	// Start recording in background
	go a.recordData(sessionDir)

	return nil
}

// StopRecording stops the TCP server
func (a *App) StopRecording() error {
	a.recordMutex.Lock()
	defer a.recordMutex.Unlock()

	if !a.isRecording {
		return fmt.Errorf("not recording")
	}

	a.isRecording = false
	if a.server != nil {
		a.server.Close()
	}

	return nil
}

// recordData handles incoming TCP connections and saves data to files
func (a *App) recordData(sessionDir string) {
	connectionCount := 0

	for a.isRecording {
		conn, err := a.server.Accept()
		if err != nil {
			if a.isRecording {
				fmt.Printf("Accept error: %v\n", err)
			}
			break
		}

		connectionCount++
		filename := filepath.Join(sessionDir, fmt.Sprintf("connection_%d.bin", connectionCount))
		a.recordedFiles = append(a.recordedFiles, filename)

		go a.handleConnection(conn, filename)
	}
}

// handleConnection handles a single TCP connection and saves its data with configuration
func (a *App) handleConnection(conn net.Conn, baseFilename string) {
	defer conn.Close()

	// Get connection details
	remoteAddr := conn.RemoteAddr().String()
	host, portStr, _ := net.SplitHostPort(remoteAddr)
	port := 0
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	// Extract connection number from filename
	connectionNum := "1"
	if strings.Contains(baseFilename, "connection_") {
		parts := strings.Split(baseFilename, "connection_")
		if len(parts) > 1 {
			connectionNum = strings.TrimSuffix(parts[1], ".bin")
		}
	}

	// Log the incoming connection
	a.AddLogEntry(LogEntry{
		FromIP:    host,
		FromPort:  port,
		ToAddress: "localhost",
		ToPort:    a.getServerPort(),
		Method:    "CONNECT",
		Data:      fmt.Sprintf("Connection %s established from %s", connectionNum, remoteAddr),
		Bytes:     0,
	})

	// Debug: Show current recording configuration
	fmt.Printf("=== RECORDING CONFIGURATION ===\n")
	fmt.Printf("Magic Word: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	fmt.Printf("Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	fmt.Printf("Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	fmt.Printf("Max File Size: %d\n", a.recordingConfig.maxFileSize)
	fmt.Printf("==============================\n")

	// Recording state
	var currentFile *os.File
	var currentFilename string
	var messageCount int
	var totalBytes int64
	var buffer []byte
	var fileIndex int = 1

	// Create initial file
	currentFilename = fmt.Sprintf("%s_%d.bin", strings.TrimSuffix(baseFilename, ".bin"), fileIndex)
	currentFile, err := os.Create(currentFilename)
	if err != nil {
		fmt.Printf("Failed to create file %s: %v\n", currentFilename, err)
		return
	}
	defer currentFile.Close()

	// Write file header with recording configuration
	if err := a.writeFileHeader(currentFile); err != nil {
		fmt.Printf("Failed to write header to %s: %v\n", currentFilename, err)
		return
	}

	fmt.Printf("Started recording to %s\n", currentFilename)

	// Read data in chunks
	chunkSize := 4096
	data := make([]byte, chunkSize)

	for {
		// Check if recording is still active
		a.recordMutex.Lock()
		if !a.isRecording {
			a.recordMutex.Unlock()
			break
		}
		a.recordMutex.Unlock()

		// Read data from connection
		n, err := conn.Read(data)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Error reading from connection: %v\n", err)
			}
			break
		}

		if n == 0 {
			continue
		}

		// Add to buffer
		buffer = append(buffer, data[:n]...)
		totalBytes += int64(n)

		// Check for magic word to separate messages
		if a.recordingConfig.magicWord != "" {
			fmt.Printf("Using magic word: '%s' to separate messages\n", a.recordingConfig.magicWord)
			fmt.Printf("Buffer length: %d bytes\n", len(buffer))
			fmt.Printf("Buffer ASCII: %q\n", string(buffer))
			fmt.Printf("Buffer HEX: %x\n", buffer)

			magicWordBytes := []byte(a.recordingConfig.magicWord)
			fmt.Printf("Magic word bytes: %x\n", magicWordBytes)

			for {
				index := bytes.Index(buffer, magicWordBytes)
				if index == -1 {
					fmt.Printf("No more magic words found in buffer\n")
					break // No magic word found in buffer
				}

				fmt.Printf("Found magic word at index: %d\n", index)

				// Write message up to magic word
				message := buffer[:index]
				if len(message) > 0 {
					fmt.Printf("Writing message: %q (length: %d)\n", string(message), len(message))
					_, writeErr := currentFile.Write(message)
					if writeErr != nil {
						fmt.Printf("Error writing to file: %v\n", writeErr)
					}
					messageCount++

					// Log each individual message
					messageStr := string(message)
					if len(messageStr) > 100 {
						messageStr = messageStr[:100] + "..."
					}
					a.AddLogEntry(LogEntry{
						FromIP:    host,
						FromPort:  port,
						ToAddress: "localhost",
						ToPort:    a.getServerPort(),
						Method:    "RECEIVE",
						Data:      fmt.Sprintf("Connection %s: Message %d - %s", connectionNum, messageCount, messageStr),
						Bytes:     int64(len(message)),
					})
				} else {
					fmt.Printf("Empty message found, skipping\n")
				}

				// Write magic word
				fmt.Printf("Writing magic word: %x\n", magicWordBytes)
				_, writeErr := currentFile.Write(magicWordBytes)
				if writeErr != nil {
					fmt.Printf("Error writing magic word: %v\n", writeErr)
				}

				// Remove processed data from buffer
				buffer = buffer[index+len(magicWordBytes):]
				fmt.Printf("Remaining buffer length: %d bytes\n", len(buffer))
				fmt.Printf("Remaining buffer: %q\n", string(buffer))

				// Check if we need to rotate file
				if a.recordingConfig.messagesPerFile > 0 && messageCount >= a.recordingConfig.messagesPerFile {
					currentFile.Close()
					fileIndex++
					currentFilename = fmt.Sprintf("%s_%d.bin", strings.TrimSuffix(baseFilename, ".bin"), fileIndex)
					currentFile, err = os.Create(currentFilename)
					if err != nil {
						fmt.Printf("Failed to create file %s: %v\n", currentFilename, err)
						return
					}
					// Write header to new file
					if err := a.writeFileHeader(currentFile); err != nil {
						fmt.Printf("Failed to write header to %s: %v\n", currentFilename, err)
						return
					}
					messageCount = 0
					fmt.Printf("Rotated to new file: %s\n", currentFilename)
				}

				// Check file size limit
				if a.recordingConfig.maxFileSize > 0 {
					fileInfo, statErr := currentFile.Stat()
					if statErr == nil && fileInfo.Size() >= a.recordingConfig.maxFileSize {
						currentFile.Close()
						fileIndex++
						currentFilename = fmt.Sprintf("%s_%d.bin", strings.TrimSuffix(baseFilename, ".bin"), fileIndex)
						currentFile, err = os.Create(currentFilename)
						if err != nil {
							fmt.Printf("Failed to create file %s: %v\n", currentFilename, err)
							return
						}
						// Write header to new file
						if err := a.writeFileHeader(currentFile); err != nil {
							fmt.Printf("Failed to write header to %s: %v\n", currentFilename, err)
							return
						}
						messageCount = 0
						fmt.Printf("File size limit reached, rotated to: %s\n", currentFilename)
					}
				}
			}
		} else {
			fmt.Printf("No magic word set, treating all data as one message\n")
			fmt.Printf("Received data - Length: %d bytes\n", len(buffer))
			fmt.Printf("ASCII: %q\n", string(buffer))
			fmt.Printf("HEX: %x\n", buffer)

			// No magic word - write all data as one message
			_, writeErr := currentFile.Write(buffer)
			if writeErr != nil {
				fmt.Printf("Error writing to file: %v\n", writeErr)
			}
			messageCount++

			// Log the message
			messageStr := string(buffer)
			if len(messageStr) > 100 {
				messageStr = messageStr[:100] + "..."
			}
			a.AddLogEntry(LogEntry{
				FromIP:    host,
				FromPort:  port,
				ToAddress: "localhost",
				ToPort:    a.getServerPort(),
				Method:    "RECEIVE",
				Data:      fmt.Sprintf("Connection %s: Message %d - %s", connectionNum, messageCount, messageStr),
				Bytes:     int64(len(buffer)),
			})

			buffer = buffer[:0] // Clear buffer
		}
	}

	// Write any remaining buffer data
	if len(buffer) > 0 {
		fmt.Printf("Final buffer data - Length: %d bytes\n", len(buffer))
		fmt.Printf("ASCII: %q\n", string(buffer))
		fmt.Printf("HEX: %x\n", buffer)

		_, writeErr := currentFile.Write(buffer)
		if writeErr != nil {
			fmt.Printf("Error writing final buffer: %v\n", writeErr)
		}
		messageCount++

		// Log the final message
		messageStr := string(buffer)
		if len(messageStr) > 100 {
			messageStr = messageStr[:100] + "..."
		}
		a.AddLogEntry(LogEntry{
			FromIP:    host,
			FromPort:  port,
			ToAddress: "localhost",
			ToPort:    a.getServerPort(),
			Method:    "RECEIVE",
			Data:      fmt.Sprintf("Connection %s: Final Message %d - %s", connectionNum, messageCount, messageStr),
			Bytes:     int64(len(buffer)),
		})
	}

	// Log the connection closure
	a.AddLogEntry(LogEntry{
		FromIP:    host,
		FromPort:  port,
		ToAddress: "localhost",
		ToPort:    a.getServerPort(),
		Method:    "DISCONNECT",
		Data:      fmt.Sprintf("Connection %s closed - Total: %d bytes, %d messages", connectionNum, totalBytes, messageCount),
		Bytes:     totalBytes,
	})

	fmt.Printf("Finished recording: %d bytes, %d messages, %d files\n", totalBytes, messageCount, fileIndex)
}

// getServerPort extracts the port from the server address
func (a *App) getServerPort() int {
	if a.serverAddr == "" {
		return 0
	}

	// Remove the colon prefix if present
	addr := strings.TrimPrefix(a.serverAddr, ":")
	port := 0
	fmt.Sscanf(addr, "%d", &port)
	return port
}

// GetRecordedFiles returns a list of all recorded files with their details
func (a *App) GetRecordedFiles() ([]FileInfo, error) {
	recordingsDir := a.GetRecordingsPath()
	fmt.Printf("=== GETTING RECORDED FILES ===\n")
	fmt.Printf("Recordings directory: %s\n", recordingsDir)

	// Check if recordings directory exists
	if _, err := os.Stat(recordingsDir); os.IsNotExist(err) {
		fmt.Printf("Recordings directory does not exist, returning empty list\n")
		return []FileInfo{}, nil // Return empty list if directory doesn't exist
	}

	var allFiles []string
	err := filepath.Walk(recordingsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process .bin files
		if !info.IsDir() && filepath.Ext(path) == ".bin" {
			allFiles = append(allFiles, path)
			fmt.Printf("Found file: %s\n", path)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error discovering recorded files: %v\n", err)
	}

	fmt.Printf("Total .bin files found: %d\n", len(allFiles))

	// Sort files by session (directory name) and then by filename
	sort.Slice(allFiles, func(i, j int) bool {
		// Extract session names from paths
		pathI := strings.Split(allFiles[i], string(os.PathSeparator))
		pathJ := strings.Split(allFiles[j], string(os.PathSeparator))

		// Get session directory name (second to last element)
		sessionI := ""
		sessionJ := ""
		if len(pathI) >= 2 {
			sessionI = pathI[len(pathI)-2]
		}
		if len(pathJ) >= 2 {
			sessionJ = pathJ[len(pathJ)-2]
		}

		// First sort by session
		if sessionI != sessionJ {
			return sessionI < sessionJ
		}

		// Then sort by filename within the same session
		return allFiles[i] < allFiles[j]
	})

	var fileInfos []FileInfo
	for _, file := range allFiles {
		info := a.GetFileInfo(file)
		fileInfos = append(fileInfos, info)
	}

	fmt.Printf("Returning %d file infos\n", len(fileInfos))
	fmt.Printf("========================\n")
	return fileInfos, nil
}

// GetFileInfo returns detailed information about a recorded file
func (a *App) GetFileInfo(filepath string) FileInfo {
	info := FileInfo{
		Path: filepath,
	}

	fmt.Printf("=== GETTING FILE INFO ===\n")
	fmt.Printf("File: %s\n", filepath)

	// Get basic file info
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		fmt.Printf("Error getting file stats: %v\n", err)
		return info
	}

	info.Size = fileInfo.Size()
	info.ModTime = fileInfo.ModTime().Format("2006-01-02 15:04:05")
	fmt.Printf("File size: %d bytes\n", info.Size)

	// Extract session and connection info from path
	pathParts := strings.Split(filepath, string(os.PathSeparator))
	if len(pathParts) >= 2 {
		info.Session = pathParts[len(pathParts)-2]
		info.FileName = pathParts[len(pathParts)-1]
	}

	// Set IsOpen if this file is being written to (recording in progress)
	if a.isRecording && info.Session == a.currentSession {
		info.IsOpen = true
		fmt.Printf("File is currently open (recording in progress)\n")
	}

	// Try to read file header for recording configuration
	file, err := os.Open(filepath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return info
	}
	defer file.Close()

	header, headerErr := a.readFileHeader(file)
	if headerErr == nil {
		// Header read successfully
		fmt.Printf("Header read successfully:\n")
		fmt.Printf("  Magic Word: '%s' (length: %d)\n", header.MagicWord, len(header.MagicWord))
		fmt.Printf("  Magic Word HEX: %x\n", []byte(header.MagicWord))
		fmt.Printf("  Messages Per File: %d\n", header.MessagesPerFile)
		fmt.Printf("  Max File Size: %d\n", header.MaxFileSize)
		fmt.Printf("  Recorded At: %s\n", header.RecordedAt)
		fmt.Printf("  Version: %d\n", header.Version)

		info.MagicWord = header.MagicWord
		info.MessagesPerFile = header.MessagesPerFile
		info.MaxFileSize = header.MaxFileSize

		// Count messages if magic word is configured
		if header.MagicWord != "" {
			// Reset file position to after header
			file.Seek(0, 0)
			var headerLength uint32
			binary.Read(file, binary.LittleEndian, &headerLength)
			file.Seek(int64(4+headerLength), 0)

			// Read remaining content
			content, readErr := io.ReadAll(file)
			if readErr == nil {
				magicWordBytes := []byte(header.MagicWord)
				messageCount := bytes.Count(content, magicWordBytes)
				info.MessageCount = messageCount
				fmt.Printf("  Message count: %d (using magic word '%s')\n", messageCount, header.MagicWord)
			} else {
				fmt.Printf("Error reading file content: %v\n", readErr)
			}
		} else {
			fmt.Printf("No magic word in header, message count will be 0\n")
		}
	} else {
		// No header found, use current config and try to count messages
		fmt.Printf("Header read failed: %v\n", headerErr)
		fmt.Printf("Falling back to current config:\n")
		fmt.Printf("  Current Magic Word: '%s'\n", a.recordingConfig.magicWord)
		fmt.Printf("  Current Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
		fmt.Printf("  Current Max File Size: %d\n", a.recordingConfig.maxFileSize)

		info.MagicWord = a.recordingConfig.magicWord
		info.MessagesPerFile = a.recordingConfig.messagesPerFile
		info.MaxFileSize = a.recordingConfig.maxFileSize

		// Try to count messages using current config magic word
		if a.recordingConfig.magicWord != "" {
			file.Seek(0, 0)
			content, readErr := io.ReadAll(file)
			if readErr == nil {
				magicWordBytes := []byte(a.recordingConfig.magicWord)
				messageCount := bytes.Count(content, magicWordBytes)
				info.MessageCount = messageCount
				fmt.Printf("  Message count: %d (using current magic word '%s')\n", messageCount, a.recordingConfig.magicWord)
			} else {
				fmt.Printf("Error reading file content: %v\n", readErr)
			}
		} else {
			fmt.Printf("No current magic word, message count will be 0\n")
		}
	}

	fmt.Printf("Final file info:\n")
	fmt.Printf("  Magic Word: '%s'\n", info.MagicWord)
	fmt.Printf("  Messages Per File: %d\n", info.MessagesPerFile)
	fmt.Printf("  Max File Size: %d\n", info.MaxFileSize)
	fmt.Printf("  Message Count: %d\n", info.MessageCount)
	fmt.Printf("  Is Open: %t\n", info.IsOpen)
	fmt.Printf("========================\n")

	return info
}

// StartPlayback starts streaming data from a recorded file to a TCP client
func (a *App) StartPlayback(filename string, host string, port int) error {
	fmt.Printf("=== StartPlayback called ===\n")
	fmt.Printf("Filename: %s\n", filename)
	fmt.Printf("Host: %s\n", host)
	fmt.Printf("Port: %d\n", port)

	a.playMutex.Lock()
	defer a.playMutex.Unlock()

	if a.isPlaying {
		return fmt.Errorf("already playing")
	}

	fmt.Printf("=== STARTING PLAYBACK ===\n")
	fmt.Printf("File: %s\n", filename)
	fmt.Printf("Target: %s:%d\n", host, port)
	fmt.Printf("Current Config - Init Delay: %v, Repeat: %d, Repeat Delay: %v, Loop on End: %t\n",
		a.playbackConfig.initDelay, a.playbackConfig.repeat, a.playbackConfig.repeatDelay, a.playbackConfig.loopOnEnd)
	fmt.Printf("========================\n")

	// Connect to TCP server
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %v", addr, err)
	}

	// Log the outgoing connection
	localAddr := conn.LocalAddr().String()
	localHost, localPortStr, _ := net.SplitHostPort(localAddr)
	localPort := 0
	if localPortStr != "" {
		fmt.Sscanf(localPortStr, "%d", &localPort)
	}

	a.AddLogEntry(LogEntry{
		FromIP:    localHost,
		FromPort:  localPort,
		ToAddress: host,
		ToPort:    port,
		Method:    "CONNECT",
		Data:      fmt.Sprintf("Connected to %s", addr),
		Bytes:     0,
	})

	a.client = conn
	a.isPlaying = true

	// Start playback in background with configuration
	go func() {
		fmt.Printf("Playback goroutine started for file: %s\n", filename)
		a.playDataWithConfig(filename, conn)
		fmt.Printf("Playback goroutine finished for file: %s\n", filename)
	}()

	return nil
}

// StopPlayback stops the TCP client
func (a *App) StopPlayback() error {
	a.playMutex.Lock()
	defer a.playMutex.Unlock()

	if !a.isPlaying {
		return fmt.Errorf("not playing")
	}

	fmt.Printf("Stopping playback...\n")
	a.isPlaying = false

	if a.client != nil {
		fmt.Printf("Closing TCP connection...\n")
		a.client.Close()
		a.client = nil
	}

	fmt.Printf("Playback stopped successfully\n")
	return nil
}

// playDataWithConfig handles playback with all configuration options
func (a *App) playDataWithConfig(filename string, conn net.Conn) {
	defer conn.Close()

	// Initial delay if configured
	if a.playbackConfig.initDelay > 0 {
		fmt.Printf("Waiting %v before starting playback...\n", a.playbackConfig.initDelay)
		time.Sleep(a.playbackConfig.initDelay)
	}

	repeatCount := 0
	maxRepeats := a.playbackConfig.repeat

	for {
		// Check if we should stop (only during the loop, not at the beginning)
		a.playMutex.Lock()
		if !a.isPlaying {
			a.playMutex.Unlock()
			fmt.Printf("Playback stopped by user\n")
			break
		}
		a.playMutex.Unlock()

		// Play the file
		bytesWritten, err := a.streamFile(filename, conn)
		if err != nil {
			fmt.Printf("Error streaming file %s: %v\n", filename, err)
			break
		}

		repeatCount++
		fmt.Printf("Completed playback #%d: %d bytes from %s\n", repeatCount, bytesWritten, filename)

		// Check if we should continue looping
		if !a.playbackConfig.loopOnEnd {
			// Loop on end disabled - stop after first play
			fmt.Printf("Loop on end disabled, stopping after first play\n")
			break
		}

		// Loop on end is enabled - check repeat limit
		if maxRepeats > 0 && repeatCount >= maxRepeats {
			// Repeat limit reached - stop
			fmt.Printf("Reached maximum repeats (%d), stopping playback\n", maxRepeats)
			break
		}

		// Continue looping - apply repeat delay if configured
		if a.playbackConfig.repeatDelay > 0 {
			fmt.Printf("Waiting %v before next repeat...\n", a.playbackConfig.repeatDelay)
			time.Sleep(a.playbackConfig.repeatDelay)
		}
	}

	// Mark playback as stopped
	a.playMutex.Lock()
	a.isPlaying = false
	fmt.Printf("Playback stopped for %s\n", filename)
	a.playMutex.Unlock()
}

// streamFile streams a single file to the connection
func (a *App) streamFile(filename string, conn net.Conn) (int64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %v", filename, err)
	}
	defer file.Close()

	// Try to read file header to get recording configuration
	var header *FileHeader
	var headerErr error
	header, headerErr = a.readFileHeader(file)

	if headerErr != nil {
		// No header found, use current configuration
		fmt.Printf("No header found in %s, using current configuration\n", filename)
		file.Seek(0, 0) // Reset to beginning
	} else {
		fmt.Printf("Found header in %s: magicWord='%s', messagesPerFile=%d, maxFileSize=%d\n",
			filename, header.MagicWord, header.MessagesPerFile, header.MaxFileSize)
	}

	// Get connection details for logging
	remoteAddr := conn.RemoteAddr().String()
	host, portStr, _ := net.SplitHostPort(remoteAddr)
	port := 0
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	// Copy data from file to connection
	bytesWritten, err := io.Copy(conn, file)
	if err != nil {
		return bytesWritten, fmt.Errorf("error copying data from file %s: %v", filename, err)
	}

	// Log the data sent
	a.AddLogEntry(LogEntry{
		FromIP:    "localhost",
		FromPort:  0,
		ToAddress: host,
		ToPort:    port,
		Method:    "SEND",
		Data:      fmt.Sprintf("Sent %d bytes from %s", bytesWritten, filepath.Base(filename)),
		Bytes:     bytesWritten,
	})

	return bytesWritten, nil
}

// IsRecording returns current recording status
func (a *App) IsRecording() bool {
	a.recordMutex.Lock()
	defer a.recordMutex.Unlock()
	return a.isRecording
}

// IsPlaying returns current playback status
func (a *App) IsPlaying() bool {
	a.playMutex.Lock()
	defer a.playMutex.Unlock()
	return a.isPlaying
}

// GetServerAddress returns the current server address
func (a *App) GetServerAddress() string {
	return a.serverAddr
}

// GetCurrentSession returns the current recording session name
func (a *App) GetCurrentSession() string {
	return a.currentSession
}

// SetPlaybackConfig sets the playback configuration
func (a *App) SetPlaybackConfig(initDelayMs int, repeat int, repeatDelayMs int, loopOnEnd bool) {
	fmt.Printf("=== SETTING PLAYBACK CONFIG ===\n")
	fmt.Printf("Initial Delay: %d ms\n", initDelayMs)
	fmt.Printf("Repeat Count: %d\n", repeat)
	fmt.Printf("Repeat Delay: %d ms\n", repeatDelayMs)
	fmt.Printf("Loop on End: %t\n", loopOnEnd)
	fmt.Printf("==============================\n")

	a.playbackConfig.initDelay = time.Duration(initDelayMs) * time.Millisecond
	a.playbackConfig.repeat = repeat
	a.playbackConfig.repeatDelay = time.Duration(repeatDelayMs) * time.Millisecond
	a.playbackConfig.loopOnEnd = loopOnEnd
}

// interpretEscapeSequences converts escape sequences like \r, \n, \t to actual characters
func (a *App) interpretEscapeSequences(s string) string {
	// Replace common escape sequences
	s = strings.ReplaceAll(s, "\\r", "\r")  // Carriage return
	s = strings.ReplaceAll(s, "\\n", "\n")  // Line feed
	s = strings.ReplaceAll(s, "\\t", "\t")  // Tab
	s = strings.ReplaceAll(s, "\\\\", "\\") // Backslash
	return s
}

// SetRecordingConfig sets the recording configuration
func (a *App) SetRecordingConfig(magicWord string, messagesPerFile int, maxFileSizeBytes int64) {
	fmt.Printf("=== SETTING RECORDING CONFIG ===\n")
	fmt.Printf("Original Magic Word: '%s' (length: %d)\n", magicWord, len(magicWord))
	fmt.Printf("Original Magic Word HEX: %x\n", []byte(magicWord))

	// Interpret escape sequences in magic word
	interpretedMagicWord := a.interpretEscapeSequences(magicWord)
	fmt.Printf("Interpreted Magic Word: '%s' (length: %d)\n", interpretedMagicWord, len(interpretedMagicWord))
	fmt.Printf("Interpreted Magic Word HEX: %x\n", []byte(interpretedMagicWord))

	fmt.Printf("Messages Per File: %d\n", messagesPerFile)
	fmt.Printf("Max File Size: %d bytes\n", maxFileSizeBytes)
	fmt.Printf("==============================\n")

	a.recordingConfig.magicWord = interpretedMagicWord
	a.recordingConfig.messagesPerFile = messagesPerFile
	a.recordingConfig.maxFileSize = maxFileSizeBytes

	// Verify the values were set
	fmt.Printf("=== VERIFICATION ===\n")
	fmt.Printf("Stored Magic Word: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	fmt.Printf("Stored Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	fmt.Printf("Stored Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	fmt.Printf("Stored Max File Size: %d\n", a.recordingConfig.maxFileSize)
	fmt.Printf("===================\n")
}

// GetPlaybackConfig returns the current playback configuration
func (a *App) GetPlaybackConfig() map[string]interface{} {
	fmt.Printf("=== GETTING PLAYBACK CONFIG ===\n")
	fmt.Printf("Current config - Init Delay: %v, Repeat: %d, Repeat Delay: %v, Loop on End: %t\n",
		a.playbackConfig.initDelay, a.playbackConfig.repeat, a.playbackConfig.repeatDelay, a.playbackConfig.loopOnEnd)
	fmt.Printf("==============================\n")

	return map[string]interface{}{
		"initDelay":   int(a.playbackConfig.initDelay.Milliseconds()),
		"repeat":      a.playbackConfig.repeat,
		"repeatDelay": int(a.playbackConfig.repeatDelay.Milliseconds()),
		"loopOnEnd":   a.playbackConfig.loopOnEnd,
	}
}

// GetRecordingConfig returns the current recording configuration
func (a *App) GetRecordingConfig() map[string]interface{} {
	fmt.Printf("=== GETTING RECORDING CONFIG ===\n")
	fmt.Printf("Current config - Magic Word: %s, Messages Per File: %d, Max File Size: %d bytes\n",
		a.recordingConfig.magicWord, a.recordingConfig.messagesPerFile, a.recordingConfig.maxFileSize)
	fmt.Printf("==============================\n")

	return map[string]interface{}{
		"magicWord":       a.recordingConfig.magicWord,
		"messagesPerFile": a.recordingConfig.messagesPerFile,
		"maxFileSize":     a.recordingConfig.maxFileSize,
	}
}

// AddLogEntry adds a new log entry
func (a *App) AddLogEntry(entry LogEntry) {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()

	entry.Time = time.Now()
	a.logEntries = append(a.logEntries, entry)
}

// GetLogEntries returns all log entries
func (a *App) GetLogEntries() []LogEntry {
	a.logMutex.RLock()
	defer a.logMutex.RUnlock()

	// Return a copy to avoid race conditions
	entries := make([]LogEntry, len(a.logEntries))
	copy(entries, a.logEntries)
	return entries
}

// ClearLogs clears all log entries
func (a *App) ClearLogs() {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()

	a.logEntries = make([]LogEntry, 0)
}

// writeFileHeader writes the recording configuration as a header to the file
func (a *App) writeFileHeader(file *os.File) error {
	fmt.Printf("=== WRITING FILE HEADER ===\n")
	fmt.Printf("Magic Word to write: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	fmt.Printf("Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	fmt.Printf("Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	fmt.Printf("Max File Size: %d\n", a.recordingConfig.maxFileSize)

	header := FileHeader{
		MagicWord:       a.recordingConfig.magicWord,
		MessagesPerFile: a.recordingConfig.messagesPerFile,
		MaxFileSize:     a.recordingConfig.maxFileSize,
		RecordedAt:      time.Now().Format("2006-01-02 15:04:05"),
		Version:         1,
	}

	// Convert header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %v", err)
	}

	fmt.Printf("Header JSON: %s\n", string(headerJSON))
	fmt.Printf("Header JSON length: %d\n", len(headerJSON))
	fmt.Printf("========================\n")

	// Write header length as 4-byte integer
	headerLength := uint32(len(headerJSON))
	if err := binary.Write(file, binary.LittleEndian, headerLength); err != nil {
		return fmt.Errorf("failed to write header length: %v", err)
	}

	// Write header JSON
	if _, err := file.Write(headerJSON); err != nil {
		return fmt.Errorf("failed to write header: %v", err)
	}

	return nil
}

// readFileHeader reads the recording configuration header from a file
func (a *App) readFileHeader(file *os.File) (*FileHeader, error) {
	fmt.Printf("Attempting to read file header...\n")

	// Read header length
	var headerLength uint32
	if err := binary.Read(file, binary.LittleEndian, &headerLength); err != nil {
		fmt.Printf("Failed to read header length: %v\n", err)
		return nil, fmt.Errorf("failed to read header length: %v", err)
	}

	fmt.Printf("Header length: %d bytes\n", headerLength)

	// Validate header length (reasonable bounds)
	if headerLength == 0 || headerLength > 1024*1024 { // Max 1MB header
		fmt.Printf("Invalid header length: %d\n", headerLength)
		return nil, fmt.Errorf("invalid header length: %d", headerLength)
	}

	// Read header JSON
	headerJSON := make([]byte, headerLength)
	if _, err := io.ReadFull(file, headerJSON); err != nil {
		fmt.Printf("Failed to read header JSON: %v\n", err)
		return nil, fmt.Errorf("failed to read header: %v", err)
	}

	fmt.Printf("Header JSON: %s\n", string(headerJSON))

	// Parse header
	var header FileHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		fmt.Printf("Failed to unmarshal header: %v\n", err)
		return nil, fmt.Errorf("failed to unmarshal header: %v", err)
	}

	fmt.Printf("Header parsed successfully:\n")
	fmt.Printf("  Magic Word: '%s' (length: %d)\n", header.MagicWord, len(header.MagicWord))
	fmt.Printf("  Magic Word HEX: %x\n", []byte(header.MagicWord))
	fmt.Printf("  Messages Per File: %d\n", header.MessagesPerFile)
	fmt.Printf("  Max File Size: %d\n", header.MaxFileSize)
	fmt.Printf("  Recorded At: %s\n", header.RecordedAt)
	fmt.Printf("  Version: %d\n", header.Version)

	return &header, nil
}

// SetRecordingsPath sets the base path for recordings
func (a *App) SetRecordingsPath(path string) error {
	fmt.Printf("=== SETTING RECORDINGS PATH ===\n")
	fmt.Printf("Path: %s\n", path)
	fmt.Printf("==============================\n")

	// Validate path
	if path == "" {
		return fmt.Errorf("recordings path cannot be empty")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create recordings directory: %v", err)
	}

	a.recordingsPath = path
	return nil
}

// GetRecordingsPath returns the current recordings path
func (a *App) GetRecordingsPath() string {
	if a.recordingsPath == "" {
		// Default to "recordings" in current directory
		return "recordings"
	}
	return a.recordingsPath
}

// SelectFolder opens a folder selection dialog and returns the selected path
func (a *App) SelectFolder() (string, error) {
	// Windows folder selection dialog
	var (
		user32                  = syscall.NewLazyDLL("user32.dll")
		shell32                 = syscall.NewLazyDLL("shell32.dll")
		ole32                   = syscall.NewLazyDLL("ole32.dll")
		procGetActiveWindow     = user32.NewProc("GetActiveWindow")
		procSHBrowseForFolder   = shell32.NewProc("SHBrowseForFolderW")
		procSHGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
		procCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
	)

	// Get active window handle
	hwnd, _, _ := procGetActiveWindow.Call()

	// Create BROWSEINFO structure
	type BROWSEINFO struct {
		Owner        uintptr
		PidlRoot     uintptr
		DisplayName  *uint16
		Title        *uint16
		Flags        uint32
		CallbackFunc uintptr
		LParam       uintptr
		Image        int32
	}

	title, _ := syscall.UTF16PtrFromString("Select Recordings Folder")
	browseInfo := BROWSEINFO{
		Owner:        hwnd,
		PidlRoot:     0,
		DisplayName:  nil,
		Title:        title,
		Flags:        0x00000001, // BIF_RETURNONLYFSDIRS
		CallbackFunc: 0,
		LParam:       0,
		Image:        0,
	}

	// Show folder selection dialog
	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&browseInfo)))
	if pidl == 0 {
		return "", fmt.Errorf("folder selection cancelled")
	}
	defer procCoTaskMemFree.Call(pidl)

	// Get path from selected folder
	path := make([]uint16, 260)
	procSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&path[0])))

	// Convert to string
	selectedPath := syscall.UTF16ToString(path[:])
	return selectedPath, nil
}
