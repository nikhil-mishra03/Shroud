// Package proxy implements the local HTTP proxy that intercepts LLM API calls,
// masks sensitive data outbound, and rehydrates placeholders in responses.
//
// Data flow:
//
//	AI Tool ──► Proxy (localhost:N) ──► upstream LLM API
//	                │
//	         Mask outbound body
//	         Rehydrate inbound response
//	         Emit events to UI hub (non-blocking)
//
// Transparent forwarding: the proxy reads the X-Shroud-Upstream header (set by
// the child launcher) to determine the real upstream URL. This means the proxy
// works with any LLM provider — Anthropic, OpenAI, deepagents, or any future
// tool — with zero hardcoding.
package proxy

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nimishr2/shroud/internal/masker"
	"github.com/nimishr2/shroud/internal/session"
	"github.com/nimishr2/shroud/internal/ui"
)

const (
	maxLoggedBodyBytes            = 256 * 1024
	maxLoggedStreamChunkBytes     = 32 * 1024
	maxLoggedStreamAggregateBytes = 1024 * 1024
)

var requestHeaderKeys = []string{
	"Accept",
	"Authorization",
	"Api-Key",
	"Content-Type",
	"OpenAI-Organization",
	"OpenAI-Project",
	"Sec-WebSocket-Key",
	"Sec-WebSocket-Protocol",
	"Sec-WebSocket-Version",
	"Upgrade",
	"User-Agent",
	"X-API-Key",
	"X-Shroud-Upstream",
}

var connectHeaderKeys = []string{
	"Proxy-Authorization",
	"User-Agent",
}

// hopByHopHeaders must not be forwarded between proxy hops (RFC 7230 §6.1).
var hopByHopHeaders = map[string]bool{
	"Connection":        true,
	"Keep-Alive":        true,
	"Transfer-Encoding": true,
	"Te":                true,
	"Trailer":           true,
	"Upgrade":           true,
	"Proxy-Connection":  true,
}

// streamRe detects streaming requests regardless of whitespace in JSON.
var streamRe = regexp.MustCompile(`"stream"\s*:\s*true`)

// logSanitizer is a package-level masker used only for sanitizing bodies before
// writing to the debug log. It is separate from the session masker so that log
// sanitization does not affect session placeholder mappings.
var logSanitizer = masker.New()

// requestCounter generates unique, monotonically-increasing request IDs.
var requestCounter atomic.Uint64

// placeholderPrefixRe matches the start of a Shroud placeholder at the end
// of a string — used to detect placeholders split across chunk boundaries.
var placeholderPrefixRe = regexp.MustCompile(`\[(EMAIL|IP|KEY|TOKEN|ENV|CRED)(_\d*)?$`)

// MaskEvent is emitted to the UI hub on each masking action.
type MaskEvent struct {
	Placeholder string
	Entity      masker.EntityType
	RequestID   string
}

// Proxy intercepts HTTP requests from AI tools, masks sensitive data, forwards
// to the real upstream, and rehydrates the response.
type Proxy struct {
	masker          *masker.Masker
	logger          *session.Logger
	debugLog        *session.ProxyDebugLogger
	hub             *ui.Hub           // nil when --ui is not set
	upstreams       map[string]string // provider name → original base URL
	defaultProvider string
	server          *http.Server
	listener        net.Listener
	selfAddr        string // the proxy's own host:port, used to detect self-referencing loops
	mu              sync.Mutex
}

func New(m *masker.Masker, l *session.Logger, debugLog *session.ProxyDebugLogger, hub *ui.Hub, upstreams map[string]string, defaultProvider string) *Proxy {
	return &Proxy{
		masker:          m,
		logger:          l,
		debugLog:        debugLog,
		hub:             hub,
		upstreams:       upstreams,
		defaultProvider: defaultProvider,
	}
}

// Start binds on a random port and begins serving. Returns the proxy base URL.
func (p *Proxy) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listening: %w", err)
	}
	p.listener = ln
	p.selfAddr = ln.Addr().String() // e.g. "127.0.0.1:54321"
	p.server = &http.Server{Handler: http.HandlerFunc(p.handle)}
	go p.server.Serve(ln)
	return "http://" + ln.Addr().String(), nil
}

func (p *Proxy) Stop() {
	if p.server != nil {
		p.server.Close()
	}
}

func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	requestID := fmt.Sprintf("%d", requestCounter.Add(1))
	start := time.Now()
	p.logger.LogRequest()

	// Emit request event to UI (non-blocking)
	p.emit(ui.Event{Type: "request", RequestID: requestID})

	if r.Method == http.MethodConnect {
		p.handleConnect(w, r, requestID)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		p.logEntry(requestID, "client_request_read_error",
			"error="+err.Error(),
			fmt.Sprintf("method=%s", r.Method),
			fmt.Sprintf("path=%s", r.URL.Path),
		)
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Mask outbound body.
	masked, events := p.masker.Mask(string(body))
	for _, e := range events {
		p.logger.LogMask(string(e.Entity), e.Placeholder, requestID)
		p.emit(ui.Event{
			Type:        "mask_event",
			Entity:      string(e.Entity),
			Placeholder: e.Placeholder,
			RequestID:   requestID,
		})
	}

	// Emit request_body event so the UI can show the masked outbound prompt.
	// Emitted for all text/JSON requests — even ones with no secrets masked —
	// so users can see Shroud processed every request (not just masked ones).
	// Clean requests show as a dimmed row; masked ones expand with highlights.
	if shouldLogBody(r.Header.Get("Content-Type")) {
		uiBody := masked
		if len(uiBody) > maxLoggedStreamAggregateBytes {
			uiBody = uiBody[:maxLoggedStreamAggregateBytes] + " [TRUNCATED]"
		}
		p.emit(ui.Event{
			Type:        "request_body",
			RequestID:   requestID,
			Body:        uiBody,
			MaskedCount: len(events),
		})
	}

	isStreaming := streamRe.MatchString(masked)

	p.logEntry(
		requestID,
		"client_request_received",
		append(
			[]string{
				fmt.Sprintf("method=%s", r.Method),
				fmt.Sprintf("host=%s", valueOrDash(r.Host)),
				fmt.Sprintf("path=%s", valueOrDash(r.URL.Path)),
				fmt.Sprintf("raw_query=%s", valueOrDash(r.URL.RawQuery)),
				fmt.Sprintf("content_type=%s", valueOrDash(r.Header.Get("Content-Type"))),
				fmt.Sprintf("content_length=%d", len(body)),
				fmt.Sprintf("stream=%t", isStreaming),
			},
			append(
				formatHeaderBlock("headers", subsetHeaders(r.Header, requestHeaderKeys)),
				formatBodyBlock("body", r.Header.Get("Content-Type"), sanitizeBodyForLog(string(body)), maxLoggedBodyBytes)...,
			)...,
		)...,
	)

	upstreamURL, err := p.resolveUpstreamURL(r)
	if err != nil {
		p.logEntry(requestID, "upstream_resolution_error",
			"error="+err.Error(),
			fmt.Sprintf("method=%s", r.Method),
			fmt.Sprintf("path=%s", valueOrDash(r.URL.Path)),
			fmt.Sprintf("host=%s", valueOrDash(r.Host)),
		)
		http.Error(w, "cannot resolve upstream", http.StatusBadGateway)
		return
	}

	outURL := *upstreamURL
	outURL.Path = joinURLPath(upstreamURL.Path, r.URL.Path)
	outURL.RawQuery = r.URL.RawQuery

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), strings.NewReader(masked))
	if err != nil {
		p.logEntry(requestID, "upstream_request_build_error",
			"error="+err.Error(),
			fmt.Sprintf("upstream_url=%s", outURL.String()),
		)
		http.Error(w, "building upstream request", http.StatusInternalServerError)
		return
	}
	outReq.ContentLength = int64(len(masked))

	for k, vv := range r.Header {
		if hopByHopHeaders[k] || k == "X-Shroud-Upstream" {
			continue
		}
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Header.Set("Host", upstreamURL.Host)
	// Strip Accept-Encoding so upstream returns uncompressed text
	// that the masker can regex-scan for rehydration.
	outReq.Header.Del("Accept-Encoding")

	p.logEntry(
		requestID,
		"upstream_request_sent",
		append(
			[]string{
				fmt.Sprintf("upstream_url=%s", outURL.String()),
				fmt.Sprintf("content_length=%d", len(masked)),
				fmt.Sprintf("stream=%t", isStreaming),
			},
			append(
				formatHeaderBlock("headers", outReq.Header),
				formatBodyBlock("body", outReq.Header.Get("Content-Type"), masked, maxLoggedBodyBytes)...,
			)...,
		)...,
	)

	upstreamStart := time.Now()
	resp, err := http.DefaultTransport.RoundTrip(outReq)
	ttfb := time.Since(upstreamStart)
	if err != nil {
		if r.Context().Err() == nil {
			p.logEntry(requestID, "upstream_transport_error",
				"error="+err.Error(),
				fmt.Sprintf("upstream_url=%s", outURL.String()),
				fmt.Sprintf("duration_ms=%d", ttfb.Milliseconds()),
			)
		}
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		if hopByHopHeaders[k] {
			continue
		}
		// Skip Content-Length: rehydration changes body size
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if isStreaming {
		p.logEntry(
			requestID,
			"upstream_response_received",
			append(
				[]string{
					fmt.Sprintf("status=%d", resp.StatusCode),
					fmt.Sprintf("ttfb_ms=%d", ttfb.Milliseconds()),
					"body_capture=streaming see chunk events",
				},
				formatHeaderBlock("headers", resp.Header)...,
			)...,
		)

		result := p.streamRehydrate(w, resp.Body, requestID)
		if result.err != nil {
			p.logEntry(requestID, "stream_read_error", "error="+result.err.Error())
		}
		p.logEntry(
			requestID,
			"stream_final_aggregate",
			append(
				[]string{
					fmt.Sprintf("chunk_count=%d", result.chunkCount),
					fmt.Sprintf("bytes=%d", result.clientBytes),
					fmt.Sprintf("duration_ms=%d", time.Since(start).Milliseconds()),
				},
				formatBodyBlock("body", resp.Header.Get("Content-Type"), sanitizeBodyForLog(result.aggregate), maxLoggedStreamAggregateBytes)...,
			)...,
		)
		p.logEntry(
			requestID,
			"client_response_sent",
			fmt.Sprintf("status=%d", resp.StatusCode),
			fmt.Sprintf("bytes=%d", result.clientBytes),
			fmt.Sprintf("duration_ms=%d", time.Since(start).Milliseconds()),
		)
		return
	}

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		p.logEntry(requestID, "upstream_response_read_error", "error="+readErr.Error())
		http.Error(w, "reading upstream response", http.StatusBadGateway)
		return
	}

	p.logEntry(
		requestID,
		"upstream_response_received",
		append(
			[]string{
				fmt.Sprintf("status=%d", resp.StatusCode),
				fmt.Sprintf("ttfb_ms=%d", ttfb.Milliseconds()),
				fmt.Sprintf("bytes=%d", len(respBody)),
			},
			append(
				formatHeaderBlock("headers", resp.Header),
				formatBodyBlock("body", resp.Header.Get("Content-Type"), sanitizeBodyForLog(string(respBody)), maxLoggedBodyBytes)...,
			)...,
		)...,
	)

	rehydrated := p.masker.Rehydrate(string(respBody))
	if rehydrated != string(respBody) {
		p.logger.LogRehydrate("(body)", requestID)
		p.emit(ui.Event{Type: "rehydrate_event", RequestID: requestID})
	}

	written, writeErr := w.Write([]byte(rehydrated))
	p.logEntry(
		requestID,
		"client_response_sent",
		append(
			[]string{
				fmt.Sprintf("status=%d", resp.StatusCode),
				fmt.Sprintf("bytes=%d", written),
				fmt.Sprintf("duration_ms=%d", time.Since(start).Milliseconds()),
			},
			formatBodyBlock("body", resp.Header.Get("Content-Type"), sanitizeBodyForLog(rehydrated), maxLoggedBodyBytes)...,
		)...,
	)
	if writeErr != nil {
		p.logEntry(requestID, "client_response_write_error", "error="+writeErr.Error())
	}
}

type streamResult struct {
	aggregate   string
	chunkCount  int
	clientBytes int
	err         error
}

// streamRehydrate reads SSE/streaming response lines, rehydrates placeholders,
// and forwards to the client. Handles placeholders that span chunk boundaries.
//
//	chunk N:   "... data: {\"content\": \"check [EMAIL"
//	chunk N+1: "_1] for details\"}"
//	           └── partial held, combined, then rehydrated
func (p *Proxy) streamRehydrate(w http.ResponseWriter, body io.Reader, requestID string) streamResult {
	flusher, canFlush := w.(http.Flusher)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var aggregate strings.Builder
	var partial string
	var result streamResult

	writeChunk := func(chunk string, addNewline bool) {
		if chunk == "" && !addNewline {
			return
		}

		sanitized := sanitizeBodyForLog(chunk)
		size := len(chunk)
		if addNewline {
			fmt.Fprintln(w, chunk)
			aggregate.WriteString(chunk)
			aggregate.WriteByte('\n')
			size++
		} else {
			fmt.Fprint(w, chunk)
			aggregate.WriteString(chunk)
		}
		if canFlush {
			flusher.Flush()
		}

		result.chunkCount++
		result.clientBytes += size
		p.logEntry(
			requestID,
			"stream_chunk_received",
			append(
				[]string{
					fmt.Sprintf("chunk_index=%d", result.chunkCount),
					fmt.Sprintf("bytes=%d", size),
				},
				formatBodyBlock("body", "text/event-stream", sanitized, maxLoggedStreamChunkBytes)...,
			)...,
		)
	}

	for scanner.Scan() {
		line := scanner.Text()
		combined := partial + line
		partial = ""

		if idx := trailingPartialPlaceholder(combined); idx >= 0 {
			partial = combined[idx:]
			combined = combined[:idx]
		}

		rehydrated := p.masker.Rehydrate(combined)
		if rehydrated != combined {
			p.logger.LogRehydrate("(stream)", requestID)
			p.emit(ui.Event{Type: "rehydrate_event", RequestID: requestID})
		}

		writeChunk(rehydrated, true)
	}

	if partial != "" {
		rehydrated := p.masker.Rehydrate(partial)
		if rehydrated != partial {
			p.logger.LogRehydrate("(stream)", requestID)
			p.emit(ui.Event{Type: "rehydrate_event", RequestID: requestID})
		}
		writeChunk(rehydrated, false)
	}

	result.aggregate = aggregate.String()
	if err := scanner.Err(); err != nil {
		result.err = err
	}
	return result
}

// emit sends an event to the UI hub non-blocking. Safe to call when hub is nil.
func (p *Proxy) emit(e ui.Event) {
	if p.hub == nil {
		return
	}
	p.hub.Publish(e)
}

// resolveUpstream returns the upstream base URL for this proxy's provider.
// Each proxy instance is bound to a single provider (e.g. "openai" or "anthropic"),
// so routing is deterministic without path inspection.
func (p *Proxy) resolveUpstream(r *http.Request) string {
	if p.defaultProvider != "" {
		if upstream := p.upstreams[p.defaultProvider]; upstream != "" {
			return upstream
		}
	}
	// Fallback: try to use the Host header if it doesn't point to us.
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if host != p.selfAddr && !strings.HasPrefix(host, "127.") && !strings.HasPrefix(host, "localhost") {
		return "https://" + host
	}
	return p.upstreams["anthropic"]
}

func (p *Proxy) resolveUpstreamURL(r *http.Request) (*url.URL, error) {
	if r.URL.IsAbs() && r.URL.Host != "" {
		u := *r.URL
		return &u, nil
	}

	// Transparent forwarding: use X-Shroud-Upstream if set, else reconstruct
	// from Host header. This makes the proxy tool-agnostic — Anthropic, OpenAI,
	// deepagents, or any future provider works with zero configuration.
	upstream := r.Header.Get("X-Shroud-Upstream")
	if upstream == "" {
		upstream = p.resolveUpstream(r)
	}

	upstreamURL, err := url.Parse(upstream)
	if err != nil || upstreamURL.Host == "" {
		return nil, fmt.Errorf("url=%q: %w", upstream, err)
	}
	return upstreamURL, nil
}

func joinURLPath(basePath, reqPath string) string {
	switch {
	case basePath == "" || basePath == "/":
		if reqPath == "" {
			return "/"
		}
		return reqPath
	case reqPath == "" || reqPath == "/":
		return basePath
	}

	baseTrim := strings.Trim(basePath, "/")
	reqTrim := strings.Trim(reqPath, "/")
	if baseTrim != "" && (reqTrim == baseTrim || strings.HasPrefix(reqTrim, baseTrim+"/")) {
		return "/" + reqTrim
	}

	return strings.TrimSuffix(basePath, "/") + "/" + strings.TrimPrefix(reqPath, "/")
}

// trailingPartialPlaceholder returns the index of a partial Shroud placeholder
// at the end of the string, or -1 if none is found.
// Only matches our specific [TYPE_N] format — never splits on JSON brackets,
// markdown links, or code arrays.
func trailingPartialPlaceholder(s string) int {
	loc := placeholderPrefixRe.FindStringIndex(s)
	if loc == nil {
		return -1
	}
	return loc[0]
}

func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request, requestID string) {
	start := time.Now()

	target := r.Host
	if target == "" {
		target = r.URL.Host
	}
	if target == "" {
		target = r.URL.String()
	}

	p.logEntry(
		requestID,
		"connect_open",
		append(
			[]string{
				fmt.Sprintf("target=%s", valueOrDash(target)),
				fmt.Sprintf("method=%s", r.Method),
			},
			formatHeaderBlock("headers", subsetHeaders(r.Header, connectHeaderKeys))...,
		)...,
	)

	upstreamConn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		p.logEntry(requestID, "connect_error",
			fmt.Sprintf("target=%s", valueOrDash(target)),
			"error="+err.Error(),
		)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstreamConn.Close()
		p.logEntry(requestID, "connect_error",
			fmt.Sprintf("target=%s", valueOrDash(target)),
			"error=hijacking not supported",
		)
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		upstreamConn.Close()
		p.logEntry(requestID, "connect_error",
			fmt.Sprintf("target=%s", valueOrDash(target)),
			"error="+err.Error(),
		)
		// Do not call http.Error after a failed Hijack — the connection is in
		// an indeterminate state and writing through ResponseWriter is unsafe.
		return
	}

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		p.logEntry(requestID, "connect_error",
			fmt.Sprintf("target=%s", valueOrDash(target)),
			"error="+err.Error(),
		)
		clientConn.Close()
		upstreamConn.Close()
		return
	}

	results := make(chan tunnelResult, 2)
	go proxyCopy(results, "client_to_upstream", upstreamConn, clientConn)
	go proxyCopy(results, "upstream_to_client", clientConn, upstreamConn)

	var clientToUpstream int64
	var upstreamToClient int64
	var finalErr error
	for i := 0; i < 2; i++ {
		result := <-results
		switch result.dir {
		case "client_to_upstream":
			clientToUpstream = result.bytes
		case "upstream_to_client":
			upstreamToClient = result.bytes
		}
		if finalErr == nil && !isExpectedTunnelError(result.err) {
			finalErr = result.err
		}
	}

	lines := []string{
		fmt.Sprintf("target=%s", valueOrDash(target)),
		fmt.Sprintf("duration_ms=%d", time.Since(start).Milliseconds()),
		fmt.Sprintf("bytes_client_to_upstream=%d", clientToUpstream),
		fmt.Sprintf("bytes_upstream_to_client=%d", upstreamToClient),
	}
	if finalErr != nil {
		lines = append(lines, "error="+finalErr.Error())
		p.logEntry(requestID, "connect_error", lines...)
		return
	}
	p.logEntry(requestID, "connect_close", lines...)
}

func proxyCopy(results chan<- tunnelResult, dir string, dst net.Conn, src net.Conn) {
	defer dst.Close()
	defer src.Close()

	n, err := io.Copy(dst, src)
	results <- tunnelResult{dir: dir, bytes: n, err: err}
}

type tunnelResult struct {
	dir   string
	bytes int64
	err   error
}

func (p *Proxy) logEntry(requestID, stage string, lines ...string) {
	if p.debugLog == nil {
		return
	}
	p.debugLog.WriteEntry(stage, requestID, lines)
}

func formatHeaderBlock(name string, headers http.Header) []string {
	if len(headers) == 0 {
		return []string{name + "=none"}
	}

	var keys []string
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := []string{name + "<<"}
	for _, key := range keys {
		values := append([]string(nil), headers.Values(key)...)
		sort.Strings(values)
		for _, value := range values {
			lines = append(lines, fmt.Sprintf("  %s: %s", key, sanitizeHeaderValue(key, value)))
		}
	}
	lines = append(lines, "end_"+name)
	return lines
}

func formatBodyBlock(name, contentType, body string, limit int) []string {
	if body == "" {
		return []string{
			fmt.Sprintf("%s_bytes=0", name),
			name + "=empty",
		}
	}
	if !shouldLogBody(contentType) {
		return []string{
			fmt.Sprintf("%s_bytes=%d", name, len(body)),
			fmt.Sprintf("%s_capture=skipped non-text content-type=%s", name, valueOrDash(contentType)),
		}
	}

	truncated, wasTruncated := truncateForLog(body, limit)
	lines := []string{
		fmt.Sprintf("%s_bytes=%d", name, len(body)),
		fmt.Sprintf("%s_logged_bytes=%d", name, len(truncated)),
	}
	if wasTruncated {
		lines = append(lines,
			fmt.Sprintf("%s_truncated=true", name),
			fmt.Sprintf("%s_limit_bytes=%d", name, limit),
		)
	}
	lines = append(lines, name+"<<")
	for _, line := range strings.Split(truncated, "\n") {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, "end_"+name)
	return lines
}

func subsetHeaders(headers http.Header, keys []string) http.Header {
	out := make(http.Header)
	for _, key := range keys {
		values := headers.Values(key)
		for _, value := range values {
			out.Add(key, value)
		}
	}
	return out
}

func sanitizeHeaderValue(key, value string) string {
	switch http.CanonicalHeaderKey(key) {
	case "Authorization", "Proxy-Authorization", "Api-Key", "X-Api-Key", "Sec-Websocket-Key":
		return redactSecretValue(value)
	default:
		return value
	}
}

func sanitizeBodyForLog(body string) string {
	sanitized, _ := logSanitizer.Mask(body)
	return sanitized
}

func shouldLogBody(contentType string) bool {
	if contentType == "" {
		return true
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(contentType))
	}
	switch {
	case mediaType == "application/json":
		return true
	case mediaType == "text/event-stream":
		return true
	case strings.HasPrefix(mediaType, "text/"):
		return true
	default:
		return false
	}
}

func truncateForLog(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}

func redactSecretValue(value string) string {
	parts := strings.Fields(value)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return fmt.Sprintf("Bearer sha256:%s len=%d", shortHash(parts[1]), len(parts[1]))
	}
	return fmt.Sprintf("<redacted sha256:%s len=%d>", shortHash(value), len(value))
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:4])
}

func isExpectedTunnelError(err error) bool {
	if err == nil || errors.Is(err, net.ErrClosed) {
		return true
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
