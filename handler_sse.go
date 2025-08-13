package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	// "time" // Removed: Not used for direct time operations in this file

	"github.com/google/uuid"
)

type SSEClient struct {
	id     string
	events chan ServerUpdate
	done   chan struct{} // Closed when client disconnects
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
			// Client already disconnected, do nothing (will be cleaned up by handleSse's defer)
			continue
		case client.events <- update:
			// Successfully sent
		default:
			log.Printf("Warning: Client %s event buffer full, dropping message", client.id)
		}
	}
}

// handleSse handles Server-Sent Events requests
func (s *ProxyServer) handleSse(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Added for CORS

	client := &SSEClient{
		id:     generateClientID(),
		events: make(chan ServerUpdate, 100), // Buffered channel to prevent blocking
		done:   make(chan struct{}),
	}

	// Register SSE client
	s.mu.Lock()
	s.clientChannels[client.id] = client
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if client, ok := s.clientChannels[client.id]; ok {
			close(client.done) // Signal all goroutines to stop
			// Do NOT close client.events here. It might be read by other goroutines if not handled carefully.
			// The event loop in handleSse will stop when context is done or client.done is closed.
			delete(s.clientChannels, client.id)
		}
		s.mu.Unlock()
	}()

	// Send initial "init" event
	initialEvent, err := json.Marshal(ServerUpdate{EventType: "init"})
	if err != nil {
		log.Printf("Error marshalling init event: %v", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", initialEvent)
	flusher.Flush()

	// Context handling
	ctx := r.Context()

	// Start message pump
	for {
		select {
		case <-ctx.Done(): // Client disconnected via context cancellation (e.g., browser tab closed)
			log.Printf("Client %s disconnected from SSE (context done)", client.id)
			return
		case <-client.done: // Client explicitly disconnected (e.g., graceful shutdown)
			log.Printf("Client %s disconnected from SSE (graceful shutdown)", client.id)
			return
		case update, ok := <-client.events: // New event from server
			if !ok {
				// Channel closed, graceful exit
				log.Printf("Client %s events channel closed, exiting SSE loop", client.id)
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
	defer s.mu.Unlock()

	for id, client := range s.clientChannels {
		log.Printf("Closing SSE channel for client %s", id)
		close(client.done) // Signal client to stop receiving
		// Do NOT close client.events here. The individual handleSse goroutine is responsible for closing it.
		delete(s.clientChannels, id) // Remove client from map
	}
}
