package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nimishr2/shroud/internal/masker"
	"github.com/nimishr2/shroud/internal/session"
)

func TestResolveUpstreamUsesDefaultProvider(t *testing.T) {
	p := New(masker.New(), &session.Logger{}, nil, nil, map[string]string{
		"openai":    "https://api.openai.com/v1",
		"anthropic": "https://api.anthropic.com",
	}, "openai")

	r := httptest.NewRequest("POST", "http://127.0.0.1/responses", nil)
	if got := p.resolveUpstream(r); got != "https://api.openai.com/v1" {
		t.Fatalf("resolveUpstream() = %q, want %q", got, "https://api.openai.com/v1")
	}
}

func TestResolveUpstreamFallsBackToPathMatching(t *testing.T) {
	p := New(masker.New(), &session.Logger{}, nil, nil, map[string]string{
		"openai":    "https://api.openai.com/v1",
		"anthropic": "https://api.anthropic.com",
	}, "")

	r := httptest.NewRequest("POST", "http://127.0.0.1/v1/messages", nil)
	if got := p.resolveUpstream(r); got != "https://api.anthropic.com" {
		t.Fatalf("resolveUpstream() = %q, want %q", got, "https://api.anthropic.com")
	}
}

func TestJoinURLPath(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		req     string
		wantOut string
	}{
		{
			name:    "root base keeps request path",
			base:    "",
			req:     "/v1/messages",
			wantOut: "/v1/messages",
		},
		{
			name:    "base v1 prefixes bare responses path",
			base:    "/v1",
			req:     "/responses",
			wantOut: "/v1/responses",
		},
		{
			name:    "base v1 does not double prefix v1 path",
			base:    "/v1",
			req:     "/v1/chat/completions",
			wantOut: "/v1/chat/completions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinURLPath(tc.base, tc.req); got != tc.wantOut {
				t.Fatalf("joinURLPath(%q, %q) = %q, want %q", tc.base, tc.req, got, tc.wantOut)
			}
		})
	}
}

func TestProxyDebugLogCapturesHTTPStages(t *testing.T) {
	logger, debugLog := newTestLogger(t)

	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading upstream body: %v", err)
		}
		upstreamBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"message":"reply [EMAIL_1]"}`)
	}))
	defer upstream.Close()

	p := New(masker.New(), logger, debugLog, nil, map[string]string{"openai": upstream.URL}, "openai")

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"input":"john@example.com","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Shroud-Upstream", upstream.URL)

	rr := httptest.NewRecorder()
	p.handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "john@example.com") {
		t.Fatalf("client response = %q, want rehydrated email", rr.Body.String())
	}
	if strings.Contains(upstreamBody, "john@example.com") {
		t.Fatalf("upstream body leaked raw secret: %q", upstreamBody)
	}
	if !strings.Contains(upstreamBody, "[EMAIL_1]") {
		t.Fatalf("upstream body = %q, want masked placeholder", upstreamBody)
	}

	logText := readProxyLog(t, logger.ProxyLogPath())
	for _, stage := range []string{
		"client_request_received",
		"upstream_request_sent",
		"upstream_response_received",
		"client_response_sent",
	} {
		if !strings.Contains(logText, "stage="+stage) {
			t.Fatalf("proxy log missing stage %q:\n%s", stage, logText)
		}
	}
	if strings.Contains(logText, "john@example.com") {
		t.Fatalf("proxy log leaked raw secret:\n%s", logText)
	}
	if !strings.Contains(logText, "Bearer sha256:") {
		t.Fatalf("proxy log did not redact auth header:\n%s", logText)
	}
	assertSingleRequestID(t, logText)
}

func TestProxyDebugLogCapturesStreamStages(t *testing.T) {
	logger, debugLog := newTestLogger(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"delta\":\"hello [EMAIL_1]\"}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"delta\":\"done\"}\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	p := New(masker.New(), logger, debugLog, nil, map[string]string{"openai": upstream.URL}, "openai")

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"input":"john@example.com","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shroud-Upstream", upstream.URL)

	rr := httptest.NewRecorder()
	p.handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "john@example.com") {
		t.Fatalf("stream response = %q, want rehydrated email", rr.Body.String())
	}

	logText := readProxyLog(t, logger.ProxyLogPath())
	for _, stage := range []string{
		"upstream_response_received",
		"stream_chunk_received",
		"stream_final_aggregate",
		"client_response_sent",
	} {
		if !strings.Contains(logText, "stage="+stage) {
			t.Fatalf("proxy log missing stage %q:\n%s", stage, logText)
		}
	}
	if strings.Contains(logText, "john@example.com") {
		t.Fatalf("proxy log leaked raw streamed secret:\n%s", logText)
	}
	assertSingleRequestID(t, logText)
}

func TestProxyDebugLogTruncatesLargeBodies(t *testing.T) {
	logger, debugLog := newTestLogger(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	largeBody := `{"input":"` + strings.Repeat("a", maxLoggedBodyBytes+64) + `"}`
	p := New(masker.New(), logger, debugLog, nil, map[string]string{"openai": upstream.URL}, "openai")

	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shroud-Upstream", upstream.URL)

	rr := httptest.NewRecorder()
	p.handle(rr, req)

	logText := readProxyLog(t, logger.ProxyLogPath())
	if !strings.Contains(logText, "body_truncated=true") {
		t.Fatalf("proxy log did not record truncation:\n%s", logText)
	}
}

func TestProxyDebugLogConnectMetadataOnly(t *testing.T) {
	logger, debugLog := newTestLogger(t)

	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer targetLn.Close()

	targetDone := make(chan struct{})
	go func() {
		defer close(targetDone)
		conn, err := targetLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err == nil && string(buf) == "ping" {
			conn.Write([]byte("pong"))
		}
	}()

	p := New(masker.New(), logger, debugLog, nil, map[string]string{"openai": "https://api.openai.com/v1"}, "openai")
	server := httptest.NewServer(http.HandlerFunc(p.handle))
	defer server.Close()

	addr := strings.TrimPrefix(server.URL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}

	reader := bufio.NewReader(conn)
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Bearer tunnel-secret\r\nUser-Agent: proxy-test\r\n\r\n", targetLn.Addr().String(), targetLn.Addr().String())

	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read connect status: %v", err)
	}
	if !strings.Contains(statusLine, "200 Connection Established") {
		t.Fatalf("status line = %q", statusLine)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read connect headers: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write tunnel payload: %v", err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(reader, reply); err != nil {
		t.Fatalf("read tunnel reply: %v", err)
	}
	if string(reply) != "pong" {
		t.Fatalf("reply = %q, want %q", string(reply), "pong")
	}
	conn.Close()
	<-targetDone

	logText := waitForProxyLog(t, logger.ProxyLogPath(), "stage=connect_close")
	if !strings.Contains(logText, "stage=connect_open") || !strings.Contains(logText, "stage=connect_close") {
		t.Fatalf("proxy log missing connect stages:\n%s", logText)
	}
	if strings.Contains(logText, "ping") || strings.Contains(logText, "pong") || strings.Contains(logText, "tunnel-secret") {
		t.Fatalf("proxy log captured tunnel payload or raw secret:\n%s", logText)
	}
	if !strings.Contains(logText, "Bearer sha256:") {
		t.Fatalf("proxy log did not redact CONNECT auth header:\n%s", logText)
	}
}

func newTestLogger(t *testing.T) (*session.Logger, *session.ProxyDebugLogger) {
	t.Helper()
	t.Setenv("SHROUD_LOG_DIR", t.TempDir())

	logger, err := session.NewLogger("proxy-test")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	t.Cleanup(func() { logger.Close() })

	debugLog, err := logger.EnableProxyDebugLog()
	if err != nil {
		t.Fatalf("enable proxy debug log: %v", err)
	}
	return logger, debugLog
}

func readProxyLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read proxy log: %v", err)
	}
	return string(data)
}

func waitForProxyLog(t *testing.T, path, want string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logText := readProxyLog(t, path)
		if strings.Contains(logText, want) {
			return logText
		}
		time.Sleep(25 * time.Millisecond)
	}
	return readProxyLog(t, path)
}

func assertSingleRequestID(t *testing.T, logText string) {
	t.Helper()
	re := regexp.MustCompile(`request_id=([^\s]+)`)
	matches := re.FindAllStringSubmatch(logText, -1)
	if len(matches) == 0 {
		t.Fatalf("proxy log did not contain request_id:\n%s", logText)
	}

	seen := map[string]bool{}
	for _, match := range matches {
		seen[match[1]] = true
	}
	if len(seen) != 1 {
		t.Fatalf("proxy log contained multiple request ids: %v\n%s", seen, logText)
	}
}
