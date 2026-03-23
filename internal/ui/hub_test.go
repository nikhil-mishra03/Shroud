package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// dialHub connects a WebSocket client to the hub's test server.
func dialHub(t *testing.T, srv *httptest.Server) (*websocket.Conn, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	wsURL := "ws" + srv.URL[4:] // http://... → ws://...
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: srv.Client(),
	})
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	return conn, cancel
}

// readEvents reads up to n events from a WebSocket connection with a timeout.
func readEvents(t *testing.T, conn *websocket.Conn, n int, timeout time.Duration) []Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var events []Event
	for i := 0; i < n; i++ {
		var e Event
		if err := wsjson.Read(ctx, conn, &e); err != nil {
			break
		}
		events = append(events, e)
	}
	return events
}

// TestReplayBuffer verifies that a client connecting after events were published
// receives all buffered events immediately on connect.
func TestReplayBuffer(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	// Publish 5 events before any client connects
	for i := 0; i < 5; i++ {
		hub.Publish(Event{Type: "mask_event", Entity: "EMAIL", Placeholder: "[EMAIL_1]"})
	}

	// Connect a client AFTER events were published
	conn, cancel := dialHub(t, srv)
	defer cancel()
	defer conn.Close(websocket.StatusNormalClosure, "")

	events := readEvents(t, conn, 5, 2*time.Second)
	if len(events) != 5 {
		t.Errorf("expected 5 replayed events, got %d", len(events))
	}
	for _, e := range events {
		if e.Type != "mask_event" {
			t.Errorf("unexpected event type: %s", e.Type)
		}
	}
}

// TestReplayBufferMaxSize verifies the replay buffer evicts oldest events when full.
func TestReplayBufferMaxSize(t *testing.T) {
	hub := NewHub()

	// Publish more than the replay buffer can hold
	for i := 0; i < replayBufferSize+20; i++ {
		hub.Publish(Event{Type: "mask_event", Entity: "IP"})
	}

	if hub.ReplayLen() != replayBufferSize {
		t.Errorf("expected replay buffer capped at %d, got %d", replayBufferSize, hub.ReplayLen())
	}
}

// TestLiveEvents verifies that events published AFTER a client connects are received.
func TestLiveEvents(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	conn, cancel := dialHub(t, srv)
	defer cancel()
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Small delay to ensure client is registered before publish
	time.Sleep(20 * time.Millisecond)

	hub.Publish(Event{Type: "mask_event", Entity: "KEY", Placeholder: "[KEY_1]"})

	events := readEvents(t, conn, 1, 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 live event, got %d", len(events))
	}
	if events[0].Entity != "KEY" {
		t.Errorf("expected KEY entity, got %s", events[0].Entity)
	}
}

// TestDisconnectCleanup verifies that a disconnected client is removed from the hub
// and does not cause a goroutine leak.
func TestDisconnectCleanup(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	goroutinesBefore := runtime.NumGoroutine()

	conn, cancel := dialHub(t, srv)
	defer cancel()

	// Wait for client to register
	time.Sleep(20 * time.Millisecond)
	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	// Disconnect the client
	conn.Close(websocket.StatusNormalClosure, "")

	// Give hub goroutine time to clean up
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.ClientCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients after disconnect, got %d", hub.ClientCount())
	}

	// Allow goroutines to settle
	time.Sleep(50 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()

	// Goroutine count should return close to baseline (allow ±3 for runtime variance)
	if goroutinesAfter > goroutinesBefore+3 {
		t.Errorf("possible goroutine leak: before=%d after=%d", goroutinesBefore, goroutinesAfter)
	}
}

// TestNonBlockingPublish verifies that a saturated client channel never blocks the publisher.
func TestNonBlockingPublish(t *testing.T) {
	hub := NewHub()

	// Add a fake client with a zero-buffer send channel (will always be "full")
	blocked := &client{
		send: make(chan Event, 0),
		done: make(chan struct{}),
	}
	hub.mu.Lock()
	hub.clients[blocked] = struct{}{}
	hub.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Publish 100 events — should all return instantly despite blocked client
		for i := 0; i < 100; i++ {
			hub.Publish(Event{Type: "mask_event"})
		}
	}()

	select {
	case <-done:
		// Good — all publishes completed without blocking
	case <-time.After(500 * time.Millisecond):
		t.Error("Publish blocked on slow client — should be non-blocking")
	}
}

// TestMultipleClients verifies all connected clients receive broadcast events.
func TestMultipleClients(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	conn1, cancel1 := dialHub(t, srv)
	defer cancel1()
	defer conn1.Close(websocket.StatusNormalClosure, "")

	conn2, cancel2 := dialHub(t, srv)
	defer cancel2()
	defer conn2.Close(websocket.StatusNormalClosure, "")

	time.Sleep(30 * time.Millisecond)

	hub.Publish(Event{Type: "mask_event", Entity: "EMAIL", Placeholder: "[EMAIL_1]"})

	e1 := readEvents(t, conn1, 1, 2*time.Second)
	e2 := readEvents(t, conn2, 1, 2*time.Second)

	if len(e1) != 1 || len(e2) != 1 {
		t.Errorf("expected both clients to receive event: conn1=%d conn2=%d", len(e1), len(e2))
	}
}
