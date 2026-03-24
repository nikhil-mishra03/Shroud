package session

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMaskedCountTracksEventsNotUniqueSecrets(t *testing.T) {
	t.Setenv("SHROUD_LOG_DIR", t.TempDir())

	logger, err := NewLogger("session-test")
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Log the same placeholder 3 times (simulating the same secret in 3 requests)
	logger.LogMask("EMAIL", "noreply@anthropic.com", "req1")
	logger.LogMask("EMAIL", "[EMAIL_1]", "req2")
	logger.LogMask("EMAIL", "[EMAIL_1]", "req3")

	logger.Close()

	f, err := os.Open(logger.Path())
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var sessionEnd map[string]interface{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e["type"] == "session_end" {
			sessionEnd = e
		}
	}

	if sessionEnd == nil {
		t.Fatal("session_end event not found")
	}
	// masked_count should be 3 (total events), not 1 (unique secrets)
	if got := sessionEnd["masked_count"].(float64); got != 3 {
		t.Errorf("masked_count = %v, want 3 (total events, not unique)", got)
	}
}

func TestEnableProxyDebugLogCreatesSeparateFileOnDemand(t *testing.T) {
	t.Setenv("SHROUD_LOG_DIR", t.TempDir())

	logger, err := NewLogger("session-test")
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	sessionDir := LogDir()
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries before proxy log = %d, want 1", len(entries))
	}
	if logger.ProxyLogPath() != "" {
		t.Fatalf("ProxyLogPath() = %q, want empty", logger.ProxyLogPath())
	}

	proxyLog, err := logger.EnableProxyDebugLog()
	if err != nil {
		t.Fatalf("EnableProxyDebugLog() error = %v", err)
	}
	if proxyLog.Path() == "" {
		t.Fatal("proxy debug log path was empty")
	}
	if !strings.HasSuffix(proxyLog.Path(), ".proxy.log") {
		t.Fatalf("proxy log path = %q, want .proxy.log suffix", proxyLog.Path())
	}

	entries, err = os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries after proxy log = %d, want 2", len(entries))
	}
	if logger.ProxyLogPath() != proxyLog.Path() {
		t.Fatalf("ProxyLogPath() = %q, want %q", logger.ProxyLogPath(), proxyLog.Path())
	}
}
