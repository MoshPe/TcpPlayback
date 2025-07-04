package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

// GetRecordedFiles returns a list of all recorded files with their details
func (a *App) GetRecordedFiles() ([]FileInfo, error) {
	recordingsDir := a.GetRecordingsPath()
	a.logger.Printf("=== GETTING RECORDED FILES ===\n")
	a.logger.Printf("Recordings directory: %s\n", recordingsDir)

	// Check if recordings directory exists
	if _, err := os.Stat(recordingsDir); os.IsNotExist(err) {
		a.logger.Printf("Recordings directory does not exist, returning empty list\n")
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
			a.logger.Printf("Found file: %s\n", path)
		}

		return nil
	})

	if err != nil {
		a.logger.Printf("Error discovering recorded files: %v\n", err)
	}

	a.logger.Printf("Total .bin files found: %d\n", len(allFiles))

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

	a.logger.Printf("Returning %d file infos\n", len(fileInfos))
	a.logger.Printf("========================\n")
	return fileInfos, nil
}

// GetFileInfo returns detailed information about a recorded file
func (a *App) GetFileInfo(filepath string) FileInfo {
	info := FileInfo{
		Path: filepath,
	}

	a.logger.Printf("=== GETTING FILE INFO ===\n")
	a.logger.Printf("File: %s\n", filepath)

	// Get basic file info
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		a.logger.Printf("Error getting file stats: %v\n", err)
		return info
	}

	info.Size = fileInfo.Size()
	info.ModTime = fileInfo.ModTime().Format("2006-01-02 15:04:05")
	a.logger.Printf("File size: %d bytes\n", info.Size)

	// Extract session and connection info from path
	pathParts := strings.Split(filepath, string(os.PathSeparator))
	if len(pathParts) >= 2 {
		info.Session = pathParts[len(pathParts)-2]
		info.FileName = pathParts[len(pathParts)-1]
	}

	// Set IsOpen if this file is being written to (recording in progress)
	if a.isRecording && info.Session == a.currentSession {
		info.IsOpen = true
		a.logger.Printf("File is currently open (recording in progress)\n")
	}

	// Try to read file header for recording configuration
	file, err := os.Open(filepath)
	if err != nil {
		a.logger.Printf("Error opening file: %v\n", err)
		return info
	}
	defer file.Close()

	header, headerErr := a.readFileHeader(file)
	if headerErr == nil {
		// Header read successfully
		a.logger.Printf("Header read successfully:\n")
		a.logger.Printf("  Magic Word: '%s' (length: %d)\n", header.MagicWord, len(header.MagicWord))
		a.logger.Printf("  Magic Word HEX: %x\n", []byte(header.MagicWord))
		a.logger.Printf("  Messages Per File: %d\n", header.MessagesPerFile)
		a.logger.Printf("  Max File Size: %d\n", header.MaxFileSize)
		a.logger.Printf("  Recorded At: %s\n", header.RecordedAt)
		a.logger.Printf("  Version: %d\n", header.Version)

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
				a.logger.Printf("  Message count: %d (using magic word '%s')\n", messageCount, header.MagicWord)
			} else {
				a.logger.Printf("Error reading file content: %v\n", readErr)
			}
		} else {
			a.logger.Printf("No magic word in header, message count will be 0\n")
		}
	} else {
		// No header found, use current config and try to count messages
		a.logger.Printf("Header read failed: %v\n", headerErr)
		a.logger.Printf("Falling back to current config:\n")
		a.logger.Printf("  Current Magic Word: '%s'\n", a.recordingConfig.magicWord)
		a.logger.Printf("  Current Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
		a.logger.Printf("  Current Max File Size: %d\n", a.recordingConfig.maxFileSize)

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
				a.logger.Printf("  Message count: %d (using current magic word '%s')\n", messageCount, a.recordingConfig.magicWord)
			} else {
				a.logger.Printf("Error reading file content: %v\n", readErr)
			}
		} else {
			a.logger.Printf("No current magic word, message count will be 0\n")
		}
	}

	a.logger.Printf("Final file info:\n")
	a.logger.Printf("  Magic Word: '%s'\n", info.MagicWord)
	a.logger.Printf("  Messages Per File: %d\n", info.MessagesPerFile)
	a.logger.Printf("  Max File Size: %d\n", info.MaxFileSize)
	a.logger.Printf("  Message Count: %d\n", info.MessageCount)
	a.logger.Printf("  Is Open: %t\n", info.IsOpen)
	a.logger.Printf("========================\n")

	return info
}

// writeFileHeader writes the recording configuration as a header to the file
func (a *App) writeFileHeader(file *os.File) error {
	a.logger.Printf("=== WRITING FILE HEADER ===\n")
	a.logger.Printf("Magic Word to write: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	a.logger.Printf("Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	a.logger.Printf("Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	a.logger.Printf("Max File Size: %d\n", a.recordingConfig.maxFileSize)

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

	a.logger.Printf("Header JSON: %s\n", string(headerJSON))
	a.logger.Printf("Header JSON length: %d\n", len(headerJSON))
	a.logger.Printf("========================\n")

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
	a.logger.Printf("Attempting to read file header...\n")

	// Read header length
	var headerLength uint32
	if err := binary.Read(file, binary.LittleEndian, &headerLength); err != nil {
		a.logger.Printf("Failed to read header length: %v\n", err)
		return nil, fmt.Errorf("failed to read header length: %v", err)
	}

	a.logger.Printf("Header length: %d bytes\n", headerLength)

	// Validate header length (reasonable bounds)
	if headerLength == 0 || headerLength > 1024*1024 { // Max 1MB header
		a.logger.Printf("Invalid header length: %d\n", headerLength)
		return nil, fmt.Errorf("invalid header length: %d", headerLength)
	}

	// Read header JSON
	headerJSON := make([]byte, headerLength)
	if _, err := io.ReadFull(file, headerJSON); err != nil {
		a.logger.Printf("Failed to read header JSON: %v\n", err)
		return nil, fmt.Errorf("failed to read header: %v", err)
	}

	a.logger.Printf("Header JSON: %s\n", string(headerJSON))

	// Parse header
	var header FileHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		a.logger.Printf("Failed to unmarshal header: %v\n", err)
		return nil, fmt.Errorf("failed to unmarshal header: %v", err)
	}

	a.logger.Printf("Header parsed successfully:\n")
	a.logger.Printf("  Magic Word: '%s' (length: %d)\n", header.MagicWord, len(header.MagicWord))
	a.logger.Printf("  Magic Word HEX: %x\n", []byte(header.MagicWord))
	a.logger.Printf("  Messages Per File: %d\n", header.MessagesPerFile)
	a.logger.Printf("  Max File Size: %d\n", header.MaxFileSize)
	a.logger.Printf("  Recorded At: %s\n", header.RecordedAt)
	a.logger.Printf("  Version: %d\n", header.Version)

	return &header, nil
}
