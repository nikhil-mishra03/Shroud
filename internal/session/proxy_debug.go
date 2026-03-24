package session

import (
	"fmt"
	"os"
	"sync"
)

// ProxyDebugLogger writes human-readable per-session proxy events.
type ProxyDebugLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func newProxyDebugLogger(path string) (*ProxyDebugLogger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening proxy debug log: %w", err)
	}
	return &ProxyDebugLogger{file: f, path: path}, nil
}

func (l *ProxyDebugLogger) WriteEntry(stage, requestID string, lines []string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Fprintf(l.file, "[%s] stage=%s", now(), stage)
	if requestID != "" {
		fmt.Fprintf(l.file, " request_id=%s", requestID)
	}
	fmt.Fprintln(l.file)
	for _, line := range lines {
		fmt.Fprintln(l.file, line)
	}
	fmt.Fprintln(l.file)
}

func (l *ProxyDebugLogger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *ProxyDebugLogger) Close() {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.file.Close()
}
