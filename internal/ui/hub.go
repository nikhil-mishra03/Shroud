package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const replayBufferSize = 100

// Event is the JSON payload sent to the browser over WebSocket.
type Event struct {
	Type        string `json:"type"`
	Timestamp   string `json:"ts"`
	Entity      string `json:"entity,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	Tool        string `json:"tool,omitempty"`
	// Body is the masked outbound request body, populated only for
	// request_body events. Never contains original secret values.
	Body        string `json:"body,omitempty"`
	// MaskedCount is the number of secrets masked in this request.
	// Zero means the request was clean (no secrets detected).
	MaskedCount int    `json:"masked_count,omitempty"`

	// Severity is the risk tier for mask_event events: "critical", "moderate", or "low".
	Severity string `json:"severity,omitempty"`

	// Structured request summary — populated for request_body events.
	// These replace the raw Body field with user-relevant content only.
	UserContent  string `json:"user_content,omitempty"`  // masked user message text
	Model        string `json:"model,omitempty"`          // model name from payload
	SystemLen    int    `json:"system_len,omitempty"`     // system prompt char count
	UserLen      int    `json:"user_len,omitempty"`       // user content char count
	ToolCount    int    `json:"tool_count,omitempty"`     // number of tool definitions
	MessageCount int    `json:"msg_count,omitempty"`      // total messages in conversation
	CriticalCount int   `json:"critical_count,omitempty"` // secrets masked at critical tier
	ModerateCount int   `json:"moderate_count,omitempty"` // secrets masked at moderate tier
	LowCount      int   `json:"low_count,omitempty"`      // secrets masked at low tier
}

// Hub manages WebSocket clients and broadcasts events.
//
//	┌─────────────────────────────────────────────┐
//	│                    Hub                       │
//	│                                              │
//	│  Publish(event) ──► replay buffer (ring)     │
//	│                 ──► broadcast to clients     │
//	│                       (non-blocking)         │
//	│                                              │
//	│  ServeHTTP() ──► WebSocket upgrade           │
//	│               ──► send replay buffer         │
//	│               ──► stream live events         │
//	│               ──► cleanup on disconnect      │
//	└─────────────────────────────────────────────┘
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
	replay  []Event // circular replay buffer (last N events)
}

type client struct {
	send chan Event
	done chan struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*client]struct{}),
		replay:  make([]Event, 0, replayBufferSize),
	}
}

// Publish emits an event to all connected clients non-blocking.
// If a client's send buffer is full, the event is dropped for that client only.
// The proxy hot path is never blocked.
func (h *Hub) Publish(e Event) {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	h.mu.Lock()
	// Append to replay buffer, evict oldest if full
	if len(h.replay) >= replayBufferSize {
		h.replay = h.replay[1:]
	}
	h.replay = append(h.replay, e)
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		select {
		case c.send <- e:
		default:
			// Client too slow — drop this event, never block
		}
	}
}

// ServeHTTP handles the WebSocket upgrade, replays buffered events,
// then streams live events until the client disconnects.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // local only, no CSRF risk
	})
	if err != nil {
		return
	}

	c := &client{
		send: make(chan Event, 256),
		done: make(chan struct{}),
	}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	replay := make([]Event, len(h.replay))
	copy(replay, h.replay)
	h.mu.Unlock()

	ctx := conn.CloseRead(context.Background())

	// Send replay buffer immediately on connect
	for _, e := range replay {
		if err := wsjson.Write(ctx, conn, e); err != nil {
			h.removeClient(c)
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}

	// Stream live events
	go func() {
		<-ctx.Done()
		close(c.done)
	}()

	for {
		select {
		case e, ok := <-c.send:
			if !ok {
				conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			if err := wsjson.Write(ctx, conn, e); err != nil {
				h.removeClient(c)
				conn.Close(websocket.StatusNormalClosure, "")
				return
			}
		case <-c.done:
			h.removeClient(c)
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

func (h *Hub) removeClient(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// ClientCount returns the number of currently connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ReplayLen returns the current number of events in the replay buffer.
func (h *Hub) ReplayLen() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.replay)
}

// Replay returns a copy of the current replay buffer.
func (h *Hub) Replay() []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Event, len(h.replay))
	copy(out, h.replay)
	return out
}

// MarshalEvent is a helper to serialise an event to JSON for logging.
func MarshalEvent(e Event) []byte {
	b, _ := json.Marshal(e)
	return b
}
