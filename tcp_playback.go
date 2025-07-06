package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// StartPlayback starts streaming data from a recorded file to a TCP client
func (a *App) StartPlayback(filename string, host string, port int) error {
	a.logger.Printf("=== StartPlayback called ===\n")
	a.logger.Printf("Filename: %s\n", filename)
	a.logger.Printf("Host: %s\n", host)
	a.logger.Printf("Port: %d\n", port)

	a.playMutex.Lock()
	defer a.playMutex.Unlock()

	if a.isPlaying {
		return fmt.Errorf("already playing")
	}

	a.logger.Printf("=== STARTING PLAYBACK ===\n")
	a.logger.Printf("File: %s\n", filename)
	a.logger.Printf("Target: %s:%d\n", host, port)
	a.logger.Printf("Current Config - Init Delay: %v, Repeat: %d, Repeat Delay: %v, Loop on End: %t\n",
		a.playbackConfig.initDelay, a.playbackConfig.repeat, a.playbackConfig.repeatDelay, a.playbackConfig.loopOnEnd)
	a.logger.Printf("========================\n")

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
		a.logger.Printf("Playback goroutine started for file: %s\n", filename)
		a.playDataWithConfig(filename, conn)
		a.logger.Printf("Playback goroutine finished for file: %s\n", filename)
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

	a.logger.Printf("Stopping playback...\n")
	a.isPlaying = false

	if a.client != nil {
		a.logger.Printf("Closing TCP connection...\n")
		a.client.Close()
		a.client = nil
	}

	a.logger.Printf("Playback stopped successfully\n")
	return nil
}

// playDataWithConfig handles playback with all configuration options
func (a *App) playDataWithConfig(filename string, conn net.Conn) {
	defer conn.Close()

	// Initial delay if configured
	if a.playbackConfig.initDelay > 0 {
		a.logger.Printf("Waiting %v before starting playback...\n", a.playbackConfig.initDelay)
		time.Sleep(a.playbackConfig.initDelay)
	}

	repeatCount := 0
	maxRepeats := a.playbackConfig.repeat

	for {
		// Check if we should stop (only during the loop, not at the beginning)
		a.playMutex.Lock()
		if !a.isPlaying {
			a.playMutex.Unlock()
			a.logger.Printf("Playback stopped by user\n")
			break
		}
		a.playMutex.Unlock()

		// Play the file
		bytesWritten, err := a.streamFile(filename, conn)
		if err != nil {
			a.logger.Printf("Error streaming file %s: %v\n", filename, err)
			break
		}

		repeatCount++
		a.logger.Printf("Completed playback #%d: %d bytes from %s\n", repeatCount, bytesWritten, filename)

		// Check if we should continue looping
		if !a.playbackConfig.loopOnEnd {
			// Loop on end disabled - stop after first play
			a.logger.Printf("Loop on end disabled, stopping after first play\n")
			break
		}

		// Loop on end is enabled - check repeat limit
		if maxRepeats > 0 && repeatCount >= maxRepeats {
			// Repeat limit reached - stop
			a.logger.Printf("Reached maximum repeats (%d), stopping playback\n", maxRepeats)
			break
		}

		// Continue looping - apply repeat delay if configured
		if a.playbackConfig.repeatDelay > 0 {
			a.logger.Printf("Waiting %v before next repeat...\n", a.playbackConfig.repeatDelay)
			time.Sleep(a.playbackConfig.repeatDelay)
		}
	}

	// Mark playback as stopped
	a.playMutex.Lock()
	a.isPlaying = false
	a.logger.Printf("Playback stopped for %s\n", filename)
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
		a.logger.Printf("No header found in %s, using current configuration\n", filename)
		file.Seek(0, 0) // Reset to beginning
		return a.streamFileAsSingleMessage(file, conn, filename)
	} else {
		a.logger.Printf("Found header in %s: magicWord='%s', messagesPerFile=%d, maxFileSize=%d\n",
			filename, header.MagicWord, header.MessagesPerFile, header.MaxFileSize)
		_, err := file.Seek(int64(header.MessagesPerFile), io.SeekCurrent)
		if err != nil {
			return 0, fmt.Errorf("failed to seek past header: %v", err)
		}
		return a.streamFileAsMessages(file, conn, filename, header.MagicWord)
	}
}

// streamFileAsMessages streams a file by parsing it into individual messages based on magic word
func (a *App) streamFileAsMessages(file *os.File, conn net.Conn, filename, magicWord string) (int64, error) {
	// Read entire file content
	content, err := io.ReadAll(file)
	if err != nil {
		return 0, fmt.Errorf("failed to read file %s: %v", filename, err)
	}

	if magicWord == "" {
		// No magic word - treat entire file as one message
		return a.streamFileAsSingleMessage(file, conn, filename)
	}

	// Get connection details for logging
	remoteAddr := conn.RemoteAddr().String()
	host, portStr, _ := net.SplitHostPort(remoteAddr)
	port := 0
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	magicWordBytes := []byte(magicWord)
	writer := bufio.NewWriter(conn)
	var totalBytes int64
	var messageCount int

	// Split content by magic word
	parts := bytes.Split(content, magicWordBytes)

	for i, part := range parts {
		// Send the message part (if not empty)
		if len(part) > 0 {
			_, writeErr := writer.Write(part)
			if writeErr != nil {
				return totalBytes, fmt.Errorf("error writing message part: %v", writeErr)
			}
			totalBytes += int64(len(part))
			messageCount++

			// Flush after each message part
			if flushErr := writer.Flush(); flushErr != nil {
				return totalBytes, fmt.Errorf("error flushing message part: %v", flushErr)
			}

			a.logger.Printf("Sent message part %d: %d bytes\n", messageCount, len(part))
		}

		// Send magic word (except after the last part)
		if i < len(parts)-1 {
			_, writeErr := writer.Write(magicWordBytes)
			if writeErr != nil {
				return totalBytes, fmt.Errorf("error writing magic word: %v", writeErr)
			}
			totalBytes += int64(len(magicWordBytes))

			// Flush after magic word to ensure message separation
			if flushErr := writer.Flush(); flushErr != nil {
				return totalBytes, fmt.Errorf("error flushing magic word: %v", flushErr)
			}

			a.logger.Printf("Sent magic word: %d bytes\n", len(magicWordBytes))
		}
	}

	// Log the data sent
	a.AddLogEntry(LogEntry{
		FromIP:    "localhost",
		FromPort:  0,
		ToAddress: host,
		ToPort:    port,
		Method:    "SEND",
		Data:      fmt.Sprintf("Sent %d messages (%d bytes) from %s", messageCount, totalBytes, filepath.Base(filename)),
		Bytes:     totalBytes,
	})

	return totalBytes, nil
}

// streamFileAsSingleMessage streams a file as a single message (for files without magic word)
func (a *App) streamFileAsSingleMessage(file *os.File, conn net.Conn, filename string) (int64, error) {
	// Reset file to beginning
	file.Seek(0, 0)

	// Get connection details for logging
	remoteAddr := conn.RemoteAddr().String()
	host, portStr, _ := net.SplitHostPort(remoteAddr)
	port := 0
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	// Use bufio.Writer to flush data immediately
	writer := bufio.NewWriter(conn)
	bytesWritten, err := io.Copy(writer, file)
	if err != nil {
		return bytesWritten, fmt.Errorf("error copying data from file %s: %v", filename, err)
	}
	if flushErr := writer.Flush(); flushErr != nil {
		return bytesWritten, fmt.Errorf("error flushing data to connection: %v", flushErr)
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
