package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
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
	logger     *log.Logger
}

// NewApp creates a new App application struct
func NewApp() *App {
	logFile, err := os.OpenFile("tcpplayback.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic("Failed to open log file: " + err.Error())
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	logger := log.New(mw, "", log.LstdFlags|log.Lshortfile)
	return &App{logger: logger}
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

// SetPlaybackConfig sets the playback configuration
func (a *App) SetPlaybackConfig(initDelayMs int, repeat int, repeatDelayMs int, loopOnEnd bool) {
	a.logger.Printf("=== SETTING PLAYBACK CONFIG ===\n")
	a.logger.Printf("Initial Delay: %d ms\n", initDelayMs)
	a.logger.Printf("Repeat Count: %d\n", repeat)
	a.logger.Printf("Repeat Delay: %d ms\n", repeatDelayMs)
	a.logger.Printf("Loop on End: %t\n", loopOnEnd)
	a.logger.Printf("==============================\n")

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
	a.logger.Printf("=== SETTING RECORDING CONFIG ===\n")
	a.logger.Printf("Original Magic Word: '%s' (length: %d)\n", magicWord, len(magicWord))
	a.logger.Printf("Original Magic Word HEX: %x\n", []byte(magicWord))

	// Interpret escape sequences in magic word
	interpretedMagicWord := a.interpretEscapeSequences(magicWord)
	a.logger.Printf("Interpreted Magic Word: '%s' (length: %d)\n", interpretedMagicWord, len(interpretedMagicWord))
	a.logger.Printf("Interpreted Magic Word HEX: %x\n", []byte(interpretedMagicWord))

	a.logger.Printf("Messages Per File: %d\n", messagesPerFile)
	a.logger.Printf("Max File Size: %d bytes\n", maxFileSizeBytes)
	a.logger.Printf("==============================\n")

	a.recordingConfig.magicWord = interpretedMagicWord
	a.recordingConfig.messagesPerFile = messagesPerFile
	a.recordingConfig.maxFileSize = maxFileSizeBytes

	// Verify the values were set
	a.logger.Printf("=== VERIFICATION ===\n")
	a.logger.Printf("Stored Magic Word: '%s' (length: %d)\n", a.recordingConfig.magicWord, len(a.recordingConfig.magicWord))
	a.logger.Printf("Stored Magic Word HEX: %x\n", []byte(a.recordingConfig.magicWord))
	a.logger.Printf("Stored Messages Per File: %d\n", a.recordingConfig.messagesPerFile)
	a.logger.Printf("Stored Max File Size: %d\n", a.recordingConfig.maxFileSize)
	a.logger.Printf("===================\n")
}

// SetRecordingsPath sets the base path for recordings
func (a *App) SetRecordingsPath(path string) error {
	a.logger.Printf("=== SETTING RECORDINGS PATH ===\n")
	a.logger.Printf("Path: %s\n", path)
	a.logger.Printf("==============================\n")

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

// GetPlaybackConfig returns the current playback configuration
func (a *App) GetPlaybackConfig() map[string]interface{} {
	a.logger.Printf("=== GETTING PLAYBACK CONFIG ===\n")
	a.logger.Printf("Current config - Init Delay: %v, Repeat: %d, Repeat Delay: %v, Loop on End: %t\n",
		a.playbackConfig.initDelay, a.playbackConfig.repeat, a.playbackConfig.repeatDelay, a.playbackConfig.loopOnEnd)
	a.logger.Printf("==============================\n")

	return map[string]interface{}{
		"initDelay":   int(a.playbackConfig.initDelay.Milliseconds()),
		"repeat":      a.playbackConfig.repeat,
		"repeatDelay": int(a.playbackConfig.repeatDelay.Milliseconds()),
		"loopOnEnd":   a.playbackConfig.loopOnEnd,
	}
}

// GetRecordingConfig returns the current recording configuration
func (a *App) GetRecordingConfig() map[string]interface{} {
	a.logger.Printf("=== GETTING RECORDING CONFIG ===\n")
	a.logger.Printf("Current config - Magic Word: %s, Messages Per File: %d, Max File Size: %d bytes\n",
		a.recordingConfig.magicWord, a.recordingConfig.messagesPerFile, a.recordingConfig.maxFileSize)
	a.logger.Printf("==============================\n")

	return map[string]interface{}{
		"magicWord":       a.recordingConfig.magicWord,
		"messagesPerFile": a.recordingConfig.messagesPerFile,
		"maxFileSize":     a.recordingConfig.maxFileSize,
	}
}

// GetServerAddress returns the current server address
func (a *App) GetServerAddress() string {
	return a.serverAddr
}

// GetCurrentSession returns the current recording session name
func (a *App) GetCurrentSession() string {
	return a.currentSession
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
