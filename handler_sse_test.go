package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateClientID(t *testing.T) {
	id1 := generateClientID()
	id2 := generateClientID()

	if id1 == "" {
		t.Error("generateClientID returned empty string")
	}
	if id1 == id2 {
		t.Error("generateClientID returned duplicate IDs")
	}

	// UUID format validation (basic check)
	if len(id1) != 36 {
		t.Errorf("expected UUID length 36, got %d", len(id1))
	}
}

func TestSendSSEUpdate(t *testing.T) {
	// Create test server
	server := &ProxyServer{
		clientChannels: make(map[string]*SSEClient),
	}

	// Create test clients with buffered channels to prevent blocking
	client1 := &SSEClient{
		id:     "test-client-1",
		events: make(chan ServerUpdate, 10),
		done:   make(chan struct{}),
	}
	client2 := &SSEClient{
		id:     "test-client-2",
		events: make(chan ServerUpdate, 10),
		done:   make(chan struct{}),
	}

	// Register clients
	server.clientChannels[client1.id] = client1
	server.clientChannels[client2.id] = client2

	update := ServerUpdate{
		EventType: "test_event",
		Entry:     LogEntry{Provider: "test-provider"},
	}

	// Send update
	server.sendSSEUpdate(update)

	// Verify both clients received the update
	receivedCount := 0
	select {
	case received1 := <-client1.events:
		if received1.EventType != "test_event" {
			t.Errorf("client1 received wrong event type: %s", received1.EventType)
		}
		receivedCount++
	case <-time.After(100 * time.Millisecond):
		t.Error("client1 did not receive update")
	}

	select {
	case received2 := <-client2.events:
		if received2.EventType != "test_event" {
			t.Errorf("client2 received wrong event type: %s", received2.EventType)
		}
		receivedCount++
	case <-time.After(100 * time.Millisecond):
		t.Error("client2 did not receive update")
	}

	if receivedCount != 2 {
		t.Errorf("expected 2 received updates, got %d", receivedCount)
	}
}

func TestSendSSEUpdateClientCleanup(t *testing.T) {
	server := &ProxyServer{
		clientChannels: make(map[string]*SSEClient),
	}

	// Create disconnected client
	closedClient := &SSEClient{
		id:     "disconnected-client",
		events: make(chan ServerUpdate, 10),
		done:   make(chan struct{}),
	}

	// Mark client as disconnected
	close(closedClient.done)

	stableClient := &SSEClient{
		id:     "stable-client",
		events: make(chan ServerUpdate, 10),
		done:   make(chan struct{}),
	}

	server.clientChannels[closedClient.id] = closedClient
	server.clientChannels[stableClient.id] = stableClient

	update := ServerUpdate{EventType: "test"}

	// Should not block on disconnected client
	server.sendSSEUpdate(update)

	// Verify stable client still works
	select {
	case <-stableClient.events:
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Error("stable client did not receive update")
	}
}

func TestHandleSSE(t *testing.T) {
	testCases := []struct {
		name           string
		setupFunc      func() *ServerTestSetup
		expectedStatus int
		expectedEvent  string
	}{
		{
			name: "valid SSE connection",
			setupFunc: func() *ServerTestSetup {
				server := &ProxyServer{
					clientChannels: make(map[string]*SSEClient),
				}
				req := httptest.NewRequest("GET", "/ui/sse", nil)
				w := &captureResponseWriter{ResponseRecorder: httptest.NewRecorder()}
				return &ServerTestSetup{server, w, req}
			},
			expectedStatus: http.StatusOK,
			expectedEvent:  "init",
		},
		{
			name: "non-flusher response writer",
			setupFunc: func() *ServerTestSetup {
				server := &ProxyServer{clientChannels: make(map[string]*SSEClient)}
				req := httptest.NewRequest("GET", "/ui/sse", nil)

				// Use a response recorder that doesn't implement Flusher
				w := httptest.NewRecorder()
				return &ServerTestSetup{server, w, req}
			},
			expectedStatus: http.StatusInternalServerError,
			expectedEvent:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setup := tc.setupFunc()

			// Handle the request in a goroutine to test streaming
			done := make(chan bool)
			go func() {
				setup.server.handleSse(setup.w, setup.req)
				done <- true
			}()

			if tc.expectedStatus == http.StatusOK {
				// For successful connection, test basic behavior
				// We'll close the request context to end the handler gracefully
				time.Sleep(50 * time.Millisecond) // Let handler start
			}

			<-done // Wait for handler to finish

			if tc.expectedStatus == http.StatusInternalServerError {
				if setup.w.(*httptest.ResponseRecorder).Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d",
						tc.expectedStatus, setup.w.(*httptest.ResponseRecorder).Code)
				}
			}
		})
	}
}

func TestGracefulShutdownSSE(t *testing.T) {
	server := &ProxyServer{
		clientChannels: make(map[string]*SSEClient),
	}

	// Create multiple test clients
	clients := []*SSEClient{
		{id: "client1", events: make(chan ServerUpdate, 1), done: make(chan struct{})},
		{id: "client2", events: make(chan ServerUpdate, 1), done: make(chan struct{})},
		{id: "client3", events: make(chan ServerUpdate, 1), done: make(chan struct{})},
	}

	// Register clients
	for _, client := range clients {
		server.clientChannels[client.id] = client
	}

	// Verify initial state
	if len(server.clientChannels) != 3 {
		t.Errorf("expected 3 clients, got %d", len(server.clientChannels))
	}

	// Perform graceful shutdown
	server.gracefulShutdownSSE()

	// Verify all clients were closed and removed
	if len(server.clientChannels) != 0 {
		t.Errorf("expected 0 clients after shutdown, got %d", len(server.clientChannels))
	}

	// Verify all client done channels were closed
	for _, client := range clients {
		select {
		case <-client.done:
			// Channel was properly closed
		default:
			t.Errorf("client %s done channel not closed", client.id)
		}
	}
}

func TestSSEClientRegistrationAndCleanup(t *testing.T) {
	server := &ProxyServer{
		clientChannels: make(map[string]*SSEClient),
	}

	t.Run("client registration", func(t *testing.T) {
		clientID := "test-registration"
		client := &SSEClient{
			id:     clientID,
			events: make(chan ServerUpdate, 1),
			done:   make(chan struct{}),
		}

		// Register client with lock
		server.clientChannels[clientID] = client

		// Verify registration
		if _, exists := server.clientChannels[clientID]; !exists {
			t.Error("client was not properly registered")
		}
	})

	t.Run("client cleanup on SSE handler exit", func(t *testing.T) {
		// Create a mock handler that can be controlled

		client := &SSEClient{
			id:     "cleanup-test",
			events: make(chan ServerUpdate, 1),
			done:   make(chan struct{}),
		}

		// Test the defer cleanup
		cleanup := func() {
			// Simulate cleanup when SSE handler exits
			close(client.done)
			delete(server.clientChannels, client.id)
		}

		defer cleanup()

		// Simulate registration
		server.clientChannels[client.id] = client
		if _, exists := server.clientChannels[client.id]; !exists {
			t.Error("client registration failed")
		}
	})
}

func TestSSEBufferedChannel(t *testing.T) {
	client := &SSEClient{
		id:     "buffer-test",
		events: make(chan ServerUpdate, 100), // Testbuffer capacity
		done:   make(chan struct{}),
	}

	// Test channel capacity
	for i := 0; i < cap(client.events); i++ {
		client.events <- ServerUpdate{EventType: "test"}
	}

	// Should be able to receive all sent messages
	received := 0
	for len(client.events) > 0 {
		<-client.events
		received++
	}

	if received != 100 {
		t.Errorf("expected to receive 100 messages, got %d", received)
	}

	close(client.done)
}

// Helper struct for testing SSE handler
type ServerTestSetup struct {
	server *ProxyServer
	w      http.ResponseWriter
	req    *http.Request
}

type mockFlusher struct {
	n      int
	count  int
	buffer bytes.Buffer
}

func (m *mockFlusher) flushResponse(w http.ResponseWriter) {

	m.count++
	// Mock response behaviour based on
	// context cancellation handling

	// Simulate successful SSE response
	if m.count <= 1 {
		fmt.Fprintf(w, "data: {}\n")
	} else if m.count >= 10 {
		return
	}
}

type mockResponseWriter struct {
	w http.ResponseWriter
	f *mockFlusher
}

func (m *mockResponseWriter) Header() http.Header {
	return m.w.Header()
}

func (m *mockResponseWriter) Write(data []byte) (int, error) {
	return m.w.Write(data)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.w.WriteHeader(statusCode)
}

func (m *mockResponseWriter) Flush() {
	// Mock flush functionality
	m.f.count++
}

func (m *mockResponseWriter) CloseNotifier() <-chan bool {
	ch := make(chan bool)
	return ch
}

// captureResponseWriter wraps ResponseRecorder to implement Flusher for testing
type captureResponseWriter struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (c *captureResponseWriter) Flush() {
	c.flushed = true
}
