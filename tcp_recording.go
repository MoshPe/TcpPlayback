package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StartRecording starts a TCP server to record incoming data
func (a *App) StartRecording(port int) error {
	a.logger.Printf("=== STARTING RECORDING ===\n")
	a.logger.Printf("Port: %d\n", port)
	a.logger.Printf("Current recording config:\n")
	a.logger.Printf("  Magic Word: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	a.logger.Printf("  Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	a.logger.Printf("  Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	a.logger.Printf("  Max File Size: %d\n", a.recordingConfig.maxFileSize)
	a.logger.Printf("========================\n")

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

	a.logger.Printf("Recording started on %s with session: %s\n", addr, a.currentSession)

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
				a.logger.Printf("Accept error: %v\n", err)
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
	a.logger.Printf("=== RECORDING CONFIGURATION ===\n")
	a.logger.Printf("Magic Word: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	a.logger.Printf("Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	a.logger.Printf("Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	a.logger.Printf("Max File Size: %d\n", a.recordingConfig.maxFileSize)
	a.logger.Printf("==============================\n")

	// Recording state
	var currentFile *os.File
	var currentFilename string
	var messageCount int
	var totalBytes int64
	var buffer []byte
	var fileIndex = 1

	// Create initial file
	currentFilename = fmt.Sprintf("%s_%d.bin", strings.TrimSuffix(baseFilename, ".bin"), fileIndex)
	currentFile, err := os.Create(currentFilename)
	if err != nil {
		a.logger.Printf("Failed to create file %s: %v\n", currentFilename, err)
		return
	}
	defer currentFile.Close()

	// Write file header with recording configuration
	if err := a.writeFileHeader(currentFile); err != nil {
		a.logger.Printf("Failed to write header to %s: %v\n", currentFilename, err)
		return
	}

	a.logger.Printf("Started recording to %s\n", currentFilename)

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
				a.logger.Printf("Error reading from connection: %v\n", err)
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
			a.logger.Printf("Using magic word: '%s' to separate messages\n", a.recordingConfig.magicWord)
			a.logger.Printf("Buffer length: %d bytes\n", len(buffer))
			a.logger.Printf("Buffer ASCII: %q\n", string(buffer))
			a.logger.Printf("Buffer HEX: %x\n", buffer)

			magicWordBytes := []byte(a.recordingConfig.magicWord)
			a.logger.Printf("Magic word bytes: %x\n", magicWordBytes)

			for {
				index := bytes.Index(buffer, magicWordBytes)
				if index == -1 {
					a.logger.Printf("No more magic words found in buffer\n")
					break // No magic word found in buffer
				}

				a.logger.Printf("Found magic word at index: %d\n", index)

				// Write message up to magic word
				message := buffer[:index]
				if len(message) > 0 {
					a.logger.Printf("Writing message: %q (length: %d)\n", string(message), len(message))
					_, writeErr := currentFile.Write(message)
					if writeErr != nil {
						a.logger.Printf("Error writing to file: %v\n", writeErr)
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
					a.logger.Printf("Empty message found, skipping\n")
				}

				// Write magic word
				a.logger.Printf("Writing magic word: %x\n", magicWordBytes)
				_, writeErr := currentFile.Write(magicWordBytes)
				if writeErr != nil {
					a.logger.Printf("Error writing magic word: %v\n", writeErr)
				}

				// Remove processed data from buffer
				buffer = buffer[index+len(magicWordBytes):]
				a.logger.Printf("Remaining buffer length: %d bytes\n", len(buffer))
				a.logger.Printf("Remaining buffer: %q\n", string(buffer))

				// Check if we need to rotate file
				if a.recordingConfig.messagesPerFile > 0 && messageCount >= a.recordingConfig.messagesPerFile {
					currentFile.Close()
					fileIndex++
					currentFilename = fmt.Sprintf("%s_%d.bin", strings.TrimSuffix(baseFilename, ".bin"), fileIndex)
					currentFile, err = os.Create(currentFilename)
					if err != nil {
						a.logger.Printf("Failed to create file %s: %v\n", currentFilename, err)
						return
					}
					// Write header to new file
					if err := a.writeFileHeader(currentFile); err != nil {
						a.logger.Printf("Failed to write header to %s: %v\n", currentFilename, err)
						return
					}
					messageCount = 0
					a.logger.Printf("Rotated to new file: %s\n", currentFilename)
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
							a.logger.Printf("Failed to create file %s: %v\n", currentFilename, err)
							return
						}
						// Write header to new file
						if err := a.writeFileHeader(currentFile); err != nil {
							a.logger.Printf("Failed to write header to %s: %v\n", currentFilename, err)
							return
						}
						messageCount = 0
						a.logger.Printf("File size limit reached, rotated to: %s\n", currentFilename)
					}
				}
			}
		} else {
			a.logger.Printf("No magic word set, treating all data as one message\n")
			a.logger.Printf("Received data - Length: %d bytes\n", len(buffer))
			a.logger.Printf("ASCII: %q\n", string(buffer))
			a.logger.Printf("HEX: %x\n", buffer)

			// No magic word - write all data as one message
			_, writeErr := currentFile.Write(buffer)
			if writeErr != nil {
				a.logger.Printf("Error writing to file: %v\n", writeErr)
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
		// Write any remaining buffer data
		if len(buffer) > 0 {
			a.logger.Printf("Final buffer data - Length: %d bytes\n", len(buffer))
			a.logger.Printf("ASCII: %q\n", string(buffer))
			a.logger.Printf("HEX: %x\n", buffer)

			_, writeErr := currentFile.Write(buffer)
			if writeErr != nil {
				a.logger.Printf("Error writing final buffer: %v\n", writeErr)
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

		a.logger.Printf("Finished recording: %d bytes, %d messages, %d files\n", totalBytes, messageCount, fileIndex)
	}
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
