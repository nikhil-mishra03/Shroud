package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"log"
)

type EventType string

const (
	EventSessionStart EventType = "session_start"
	EventMask         EventType = "mask_event"
	EventRehydrate    EventType = "rehydrate_event"
	EventSessionEnd   EventType = "session_end"
)

type Event struct {
	Type        EventType `json:"type"`
	Timestamp   string    `json:"ts"`
	Tool        string    `json:"tool,omitempty"`
	PID         int       `json:"pid,omitempty"`
	Entity      string    `json:"entity,omitempty"`
	Placeholder string    `json:"placeholder,omitempty"`
	RequestID   string    `json:"request_id,omitempty"`
	MaskedCount int       `json:"masked_count,omitempty"`
	ReqCount    int       `json:"request_count,omitempty"`
}

type Logger struct {
	mu          sync.Mutex
	file        *os.File
	path        string
	prefix      string
	proxyLog    *ProxyDebugLogger
	// maskedCount tracks total mask events (may count the same secret multiple
	// times across requests). For unique secrets, use len(masker.Mappings()).
	maskedCount int
	reqCount    int
}

func LogDir() string {
	if dir := os.Getenv("SHROUD_LOG_DIR"); dir != "" {
		return dir
	}

	wd, err := os.Getwd()
	if err != nil {
		return "logs"
	}
	return filepath.Join(wd, "logs")
}

func NewLogger(tool string) (*Logger, error) {
	dir := LogDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating session dir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	prefix := filepath.Join(dir, fmt.Sprintf("%s-%s", tool, ts))
	path := prefix + ".jsonl"

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening session log: %w", err)
	}

	l := &Logger{file: f, path: path, prefix: prefix}
	l.writeLocked(Event{
		Type:      EventSessionStart,
		Timestamp: now(),
		Tool:      tool,
		PID:       os.Getpid(),
	})
	return l, nil
}

func (l *Logger) LogMask(entity, placeholder, requestID string) {
	l.mu.Lock()
	l.maskedCount++
	l.writeLocked(Event{
		Type:        EventMask,
		Timestamp:   now(),
		Entity:      entity,
		Placeholder: placeholder,
		RequestID:   requestID,
	})
	l.mu.Unlock()
}

func (l *Logger) LogRehydrate(placeholder, requestID string) {
	l.mu.Lock()
	l.writeLocked(Event{
		Type:        EventRehydrate,
		Timestamp:   now(),
		Placeholder: placeholder,
		RequestID:   requestID,
	})
	l.mu.Unlock()
}

func (l *Logger) LogRequest() {
	l.mu.Lock()
	l.reqCount++
	l.mu.Unlock()
}

func (l *Logger) Close() {
	l.mu.Lock()
	l.writeLocked(Event{
		Type:        EventSessionEnd,
		Timestamp:   now(),
		MaskedCount: l.maskedCount,
		ReqCount:    l.reqCount,
	})
	l.file.Close()
	if l.proxyLog != nil {
		l.proxyLog.Close()
	}
	l.mu.Unlock()
}

func (l *Logger) Path() string { return l.path }

func (l *Logger) EnableProxyDebugLog() (*ProxyDebugLogger, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.proxyLog != nil {
		return l.proxyLog, nil
	}

	proxyLog, err := newProxyDebugLogger(l.prefix + ".proxy.log")
	if err != nil {
		return nil, err
	}
	l.proxyLog = proxyLog
	return proxyLog, nil
}

func (l *Logger) ProxyLogPath() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.proxyLog == nil {
		return ""
	}
	return l.proxyLog.Path()
}

// writeLocked writes a JSON event to the log file. Caller must hold l.mu.
func (l *Logger) writeLocked(e Event) {
	b, _ := json.Marshal(e)
	b = append(b, '\n')
	if _, err := l.file.Write(b); err != nil {
		log.Printf("shroud: session log write error: %v", err)
	}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
