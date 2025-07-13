package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatchConfig(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "test-config")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy config file
	configFile := filepath.Join(tmpDir, "llmsee.yml")
	err = os.WriteFile(configFile, []byte("port: 1234"), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Create a mock ProxyServer
	server, err := NewProxyServer()
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	// Override the config file path
	os.Setenv("LLMSEE_CONFIGFILE", configFile)
	config, err := getConfig()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}
	server.config = *config

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the watcher in a goroutine
	go server.watchConfig(ctx)

	// Wait for the watcher to start
	time.Sleep(100 * time.Millisecond)

	// Modify the config file
	err = os.WriteFile(configFile, []byte("port: 5678"), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Wait for the config to be reloaded
	time.Sleep(100 * time.Millisecond)

	// Check if the config was reloaded
	server.mu.RLock()
	newPort := server.config.Port
	server.mu.RUnlock()

	if newPort != 5678 {
		t.Errorf("Expected port to be 5678, but got %d", newPort)
	}

	// Check if an SSE event was sent
	var wg sync.WaitGroup
	wg.Add(1)
	client := &SSEClient{
		id:     "test-client",
		events: make(chan ServerUpdate, 1),
		done:   make(chan struct{}),
	}
	server.mu.Lock()
	server.clientChannels[client.id] = client
	server.mu.Unlock()

	go func() {
		defer wg.Done()
		select {
		case update := <-client.events:
			if update.EventType != "config-reloaded" {
				t.Errorf("Expected event type to be 'config-reloaded', but got '%s'", update.EventType)
			}
			if update.Message != "Configuration reloaded" {
				t.Errorf("Expected message to be 'Configuration reloaded', but got '%s'", update.Message)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timed out waiting for SSE event")
		}
	}()

	// Trigger another config reload to send the event
	err = os.WriteFile(configFile, []byte("port: 1234"), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	wg.Wait()
}
