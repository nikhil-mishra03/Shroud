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
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nimishr2/shroud/internal/masker"
	"github.com/nimishr2/shroud/internal/session"
	"github.com/nimishr2/shroud/internal/ui"
)

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

// placeholderPrefixRe matches the start of a Shroud placeholder at the end
// of a string — used to detect placeholders split across chunk boundaries.
var placeholderPrefixRe = regexp.MustCompile(`\[(EMAIL|IP|KEY|TOKEN|ENV)(_\d*)?$`)

// MaskEvent is emitted to the UI hub on each masking action.
type MaskEvent struct {
	Placeholder string
	Entity      masker.EntityType
	RequestID   string
}

// Proxy intercepts HTTP requests from AI tools, masks sensitive data, forwards
// to the real upstream, and rehydrates the response.
type Proxy struct {
	masker    *masker.Masker
	logger    *session.Logger
	hub       *ui.Hub // nil when --ui is not set
	upstreams map[string]string // provider name → original base URL
	server    *http.Server
	listener  net.Listener
	selfAddr  string // the proxy's own host:port, used to detect self-referencing loops
	mu        sync.Mutex
}

func New(m *masker.Masker, l *session.Logger, hub *ui.Hub, upstreams map[string]string) *Proxy {
	return &Proxy{
		masker:    m,
		logger:    l,
		hub:       hub,
		upstreams: upstreams,
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
	requestID := fmt.Sprintf("%d", time.Now().UnixNano())
	p.logger.LogRequest()

	// Emit request event to UI (non-blocking)
	p.emit(ui.Event{Type: "request", RequestID: requestID})

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Mask outbound body
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

	// Transparent forwarding: use X-Shroud-Upstream if set, else reconstruct
	// from Host header. This makes the proxy tool-agnostic — Anthropic, OpenAI,
	// deepagents, or any future provider works with zero configuration.
	upstream := r.Header.Get("X-Shroud-Upstream")
	if upstream == "" {
		upstream = p.resolveUpstream(r)
	}

	upstreamURL, err := url.Parse(upstream)
	if err != nil || upstreamURL.Host == "" {
		log.Printf("shroud: cannot resolve upstream (url=%q): %v", upstream, err)
		http.Error(w, "cannot resolve upstream", http.StatusBadGateway)
		return
	}

	outURL := *upstreamURL
	outURL.Path = r.URL.Path
	outURL.RawQuery = r.URL.RawQuery

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), strings.NewReader(masked))
	if err != nil {
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

	isStreaming := streamRe.MatchString(masked)

	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		// context.Canceled and EOF are expected during shutdown — don't log them
		if r.Context().Err() == nil {
			log.Printf("shroud: upstream error: %v", err)
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
		p.streamRehydrate(w, resp.Body, requestID)
	} else {
		respBody, _ := io.ReadAll(resp.Body)
		rehydrated := p.masker.Rehydrate(string(respBody))
		if rehydrated != string(respBody) {
			p.logger.LogRehydrate("(body)", requestID)
			p.emit(ui.Event{Type: "rehydrate_event", RequestID: requestID})
		}
		w.Write([]byte(rehydrated))
	}
}

// streamRehydrate reads SSE/streaming response lines, rehydrates placeholders,
// and forwards to the client. Handles placeholders that span chunk boundaries.
//
//	chunk N:   "... data: {\"content\": \"check [EMAIL"
//	chunk N+1: "_1] for details\"}"
//	           └── partial held, combined, then rehydrated
func (p *Proxy) streamRehydrate(w http.ResponseWriter, body io.Reader, requestID string) {
	flusher, canFlush := w.(http.Flusher)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var partial string

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

		fmt.Fprintln(w, rehydrated)
		if canFlush {
			flusher.Flush()
		}
	}

	if partial != "" {
		rehydrated := p.masker.Rehydrate(partial)
		fmt.Fprint(w, rehydrated)
		if canFlush {
			flusher.Flush()
		}
	}
}

// emit sends an event to the UI hub non-blocking. Safe to call when hub is nil.
func (p *Proxy) emit(e ui.Event) {
	if p.hub == nil {
		return
	}
	p.hub.Publish(e)
}

// resolveUpstream determines the real upstream URL based on the request path.
// Uses path-based routing so the proxy works with any AI tool and any provider
// without hardcoding — the original base URLs are captured from the environment
// before Shroud overwrites them.
//
//	Path pattern              → Provider
//	/v1/messages              → anthropic
//	/v1/chat/completions      → openai
//	/v1/models                → openai
//	(default)                 → anthropic
func (p *Proxy) resolveUpstream(r *http.Request) string {
	path := r.URL.Path

	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		return p.upstreams["anthropic"]
	case strings.HasPrefix(path, "/v1/chat/completions"),
		strings.HasPrefix(path, "/v1/completions"),
		strings.HasPrefix(path, "/v1/models"),
		strings.HasPrefix(path, "/v1/embeddings"):
		return p.upstreams["openai"]
	default:
		// Fallback: try to use the Host header if it doesn't point to us
		host := r.Host
		if host == "" {
			host = r.URL.Host
		}
		if host != p.selfAddr && !strings.HasPrefix(host, "127.") && !strings.HasPrefix(host, "localhost") {
			return "https://" + host
		}
		// Last resort: default to Anthropic
		return p.upstreams["anthropic"]
	}
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
