package main

import (
	"time"
)

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
