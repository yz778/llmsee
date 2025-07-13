package main

import (
	"context"
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

	// Create a dummy config
	config := &Config{
		Port:         1234,
		DatabaseFile: "", // disable database for this test
	}

	// Create a mock ProxyServer
	server, err := NewProxyServer(config)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	// Set the config file path for the watcher
	t.Setenv("LLMSEE_CONFIGFILE", configFile)

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

	// Wait for the config to be reloaded by polling
	var newPort int
	var reloaded bool
	for i := 0; i < 200; i++ { // Poll for up to 2 seconds
		server.mu.RLock()
		newPort = server.config.Port
		server.mu.RUnlock()
		if newPort == 5678 {
			reloaded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !reloaded {
		t.Logf("Final port value: %d", newPort)
		t.Fatalf("timed out waiting for config to be reloaded. Expected port 5678, got %d", newPort)
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
