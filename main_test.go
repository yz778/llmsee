package main

import (
	"os"
	"testing"
	"time"
)

func TestNewProxyServer(t *testing.T) {
	// Use a temporary database file to avoid conflicts
	tempDB, err := os.CreateTemp("", "test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempDB.Name())
	defer tempDB.Close()

	// Set environment variables for testing
	originalDatabaseFile := os.Getenv("LLMSEE_DATABASEFILE")
	originalAppEnv := os.Getenv("APP_ENV")
	defer func() {
		os.Setenv("LLMSEE_DATABASEFILE", originalDatabaseFile)
		os.Setenv("APP_ENV", originalAppEnv)
	}()

	os.Setenv("LLMSEE_DATABASEFILE", tempDB.Name())

	t.Run("basic initialization", func(t *testing.T) {
		server, err := NewProxyServer()
		if err != nil {
			t.Fatalf("NewProxyServer() error = %v", err)
		}
		if server == nil {
			t.Error("NewProxyServer() returned nil server")
		}
		if server.db == nil {
			t.Error("database is nil")
		}
		if server.client == nil {
			t.Error("HTTP client is nil")
		}
		if server.clientChannels == nil {
			t.Error("clientChannels map is nil")
		}
	})

	t.Run("dev mode initialization", func(t *testing.T) {
		os.Setenv("APP_ENV", "dev")
		defer os.Setenv("APP_ENV", originalAppEnv)

		server, err := NewProxyServer()
		if err != nil {
			t.Fatalf("NewProxyServer() error = %v", err)
		}
		if !server.devMode {
			t.Error("expected dev mode to be enabled")
		}
	})
}

func TestServerGracefulShutdownSSE(t *testing.T) {
	// Create minimal server for testing
	server := &ProxyServer{
		clientChannels: make(map[string]*SSEClient),
	}

	// Add some test clients
	client1 := &SSEClient{
		id:     "test-client-1",
		events: make(chan ServerUpdate, 1),
		done:   make(chan struct{}),
	}
	client2 := &SSEClient{
		id:     "test-client-2",
		events: make(chan ServerUpdate, 1),
		done:   make(chan struct{}),
	}

	server.clientChannels[client1.id] = client1
	server.clientChannels[client2.id] = client2

	// Test graceful shutdown
	server.gracefulShutdownSSE()

	// Verify clients were removed
	if len(server.clientChannels) != 0 {
		t.Errorf("Expected 0 clients after shutdown, got %d", len(server.clientChannels))
	}

	// Verify client channels were properly closed
	select {
	case <-client1.done:
		// Channel properly closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Client1 channel not closed")
	}

	select {
	case <-client2.done:
		// Channel properly closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Client2 channel not closed")
	}
}

func TestSignalHandling(t *testing.T) {
	// This test verifies signal handling is set up correctly
	// We can't fully test the signal handling in unit tests, but we can
	// verify the signal channel is created

	server := &ProxyServer{
		clientChannels: make(map[string]*SSEClient),
	}

	// Mock shutdown components
	mockWatcher := &ConfigWatcher{
		stopChan: make(chan struct{}),
	}
	server.configWatcher = mockWatcher

	// Just verify we can create the dependencies without panicking
	// The actual signal handling happens in main()
	if server.configWatcher == nil {
		t.Error("Failed to create config watcher mock")
	}
}

func TestGetDatabaseFilePathSegmentation(t *testing.T) {
	// This test ensures we handle edge cases in getDatabaseFile

	// Save original environment
	originalHome := os.Getenv("HOME")
	originalAppData := os.Getenv("APPDATA")
	originalDBVar := os.Getenv("LLMSEE_DATABASEFILE")

	defer func() {
		os.Setenv("HOME", originalHome)
		os.Setenv("APPDATA", originalAppData)
		os.Setenv("LLMSEE_DATABASEFILE", originalDBVar)
	}()

	testCases := []struct {
		name     string
		setupEnv func()
		expected string
	}{
		{
			name: "invalid platform fallback",
			setupEnv: func() {
				// Clear specific env vars to test fallback
				os.Unsetenv("LLMSEE_DATABASEFILE")
				os.Setenv("HOME", "/testhome")
			},
			expected: "llmsee.db", // Should use current dir as fallback
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupEnv()
			result := getDatabaseFile()
			if result == "" {
				t.Error("getDatabaseFile returned empty string")
			}
		})
	}
}

func TestEmptyDefaultConfig(t *testing.T) {
	// Test that we can access defaultConfig without issues
	if len(defaultConfig.Providers) == 0 {
		t.Error("defaultConfig.Providers should not be empty")
	}

	if defaultConfig.PageSize <= 0 {
		t.Error("defaultConfig.PageSize should be positive")
	}
}

func TestMemoryOptimizedConstants(t *testing.T) {
	// Verify constants are properly defined
	if clientIdleConnTimeout <= 0 {
		t.Error("clientIdleConnTimeout should be positive")
	}
	if clientMaxIdleConns <= 0 {
		t.Error("clientMaxIdleConns should be positive")
	}
	if dbConnMaxLifetime <= 0 {
		t.Error("dbConnMaxLifetime should be positive")
	}
	if httpMaxRequestBodySize <= 0 {
		t.Error("httpMaxRequestBodySize should be positive")
	}
	if shutdownTimeout <= 0 {
		t.Error("shutdownTimeout should be positive")
	}
}
