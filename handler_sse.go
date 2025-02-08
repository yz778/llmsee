package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)

type SSEClient struct {
	id     string
	events chan ServerUpdate
	done   chan struct{}
}

type ServerUpdate struct {
	EventType string   `json:"eventType"`
	Entry     LogEntry `json:"entry"`
}

func generateClientID() string {
	return uuid.New().String()
}

// sendSSEUpdate sends broadcast to all connected clients
func (s *ProxyServer) sendSSEUpdate(update ServerUpdate) {
	s.mu.RLock() // Use read lock for better performance
	defer s.mu.RUnlock()

	for _, client := range s.clientChannels {
		select {
		case <-client.done:
			continue
		case client.events <- update:
			// Successfully sent
		default:
			log.Printf("Warning: Client %s event buffer full, dropping message", client.id)
		}
	}
}

// handle SSE
func (s *ProxyServer) handleSse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	client := &SSEClient{
		id:     generateClientID(),
		events: make(chan ServerUpdate, 100), // Buffered channel to prevent blocking
		done:   make(chan struct{}),
	}

	// Register SSE client
	s.mu.Lock()
	s.clientChannels[client.id] = client
	s.mu.Unlock()

	// Clean up on disconnect
	defer func() {
		s.mu.Lock()
		if client, ok := s.clientChannels[client.id]; ok {
			close(client.done)   // Signal all goroutines to stop
			close(client.events) // Close events channel
			delete(s.clientChannels, client.id)
		}
		s.mu.Unlock()
	}()

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"eventType\": \"init\", \"clientID\": \"%s\"}\n\n", client.id)
	flusher.Flush()

	// Context handling
	ctx := r.Context()

	// Start message pump
	for {
		select {
		case <-ctx.Done():
			return
		case <-client.done:
			return
		case update, ok := <-client.events:
			if !ok {
				return
			}
			jsonData, err := json.Marshal(update)
			if err != nil {
				log.Printf("failed to encode update: %v", err)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()
		}
	}
}

func (s *ProxyServer) gracefulShutdownSSE() {
	s.mu.Lock()
	for _, client := range s.clientChannels {
		close(client.done)   // Forcefully close client's done channel to stop the SSE loop
		close(client.events) // Forcefully close client's events channel
	}
	s.clientChannels = make(map[string]*SSEClient) // Clear the client list
	s.mu.Unlock()
}
