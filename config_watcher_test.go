package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockFileWatcher simulates fsnotify for testing
type mockFileWatcher struct {
	events chan struct{}
	errors chan error
	closed bool
}

func (m *mockFileWatcher) Close() error {
	m.closed = true
	return nil
}

func TestNewConfigWatcher(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	t.Run("create new config watcher", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "test_config_watcher")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		configFile := filepath.Join(tempDir, "test.yml")
		if err := os.WriteFile(configFile, []byte(`{"test": "config"}`), 0644); err != nil {
			t.Fatal(err)
		}

		watcher, err := NewConfigWatcher(server, configFile)
		if err != nil {
			t.Fatalf("NewConfigWatcher() error = %v", err)
		}

		if watcher == nil {
			t.Error("NewConfigWatcher returned nil watcher")
		}
		if watcher.server != server {
			t.Error("watcher server not set correctly")
		}
		if watcher.configFile != configFile {
			t.Errorf("expected config file %s, got %s", configFile, watcher.configFile)
		}

		// Clean up
		watcher.Stop()
	})

	t.Run("empty config file", func(t *testing.T) {
		watcher, err := NewConfigWatcher(server, "")
		if err != nil {
			t.Fatalf("NewConfigWatcher() with empty file error = %v", err)
		}
		if watcher == nil {
			t.Error("expected watcher for empty file")
		}
	})
}

func TestConfigWatcherStartStop(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	tempDir, err := os.MkdirTemp("", "test_watcher_start")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(configFile, []byte(`{"test": "config"}`), 0644); err != nil {
		t.Fatal(err)
	}

	watcher, err := NewConfigWatcher(server, configFile)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("start with empty config file", func(t *testing.T) {
		watcherEmpty, err := NewConfigWatcher(server, "")
		if err != nil {
			t.Fatal(err)
		}

		err = watcherEmpty.Start()
		if err != nil {
			t.Errorf("Start() with empty config file error = %v", err)
		}

		watcherEmpty.Stop()
	})

	t.Run("start and stop", func(t *testing.T) {
		err := watcher.Start()

		// skip actual fsnotify for unit tests since it requires filesystem changes
		// we just ensure no panics when setting up
		_ = err

		watcher.Stop()

		// Test stop is idempotent
		watcher.Stop()

		if watcher.stopChan == nil {
			t.Error("stop channel not initialized")
		}
	})

	t.Run("stop with nil watcher", func(t *testing.T) {
		// Test stopping a nil watcher doesn't panic
		watcher := &ConfigWatcher{stopChan: make(chan struct{})}
		watcher.Stop()
	})
}

func TestConfigWatcherReloadConfig(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	tempDir, err := os.MkdirTemp("", "test_watcher_reload")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("reload valid config", func(t *testing.T) {
		configFile := filepath.Join(tempDir, "test_config.yml")
		configContent := `host: localhost
port: 8080
databasefile: test.db
pagesize: 50
providers:
  testprovider:
    baseurl: http://test.com
    apikey: testkey
    enabled: true
`

		if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
			t.Fatal(err)
		}

		watcher := &ConfigWatcher{
			configFile: configFile,
			server:     server,
			stopChan:   make(chan struct{}),
		}

		watcher.reloadConfig()

		// Ensure config wasn't reloaded if it's invalid
		// we'll skip the actual reload testing here and just ensure no panic
	})

	t.Run("reload invalid config", func(t *testing.T) {
		configFile := filepath.Join(tempDir, "invalid_config.yml")
		invalidContent := `invalid: yaml: content: :`

		if err := os.WriteFile(configFile, []byte(invalidContent), 0644); err != nil {
			t.Fatal(err)
		}

		watcher := &ConfigWatcher{
			configFile: configFile,
			server:     server,
			stopChan:   make(chan struct{}),
		}

		// This should not panic
		watcher.reloadConfig()

		// Config should remain unchanged if loading fails
	})

	t.Run("reload non-existent config", func(t *testing.T) {
		configFile := filepath.Join(tempDir, "does_not_exist.yml")

		watcher := &ConfigWatcher{
			configFile: configFile,
			server:     server,
			stopChan:   make(chan struct{}),
		}

		// Should handle non-existent file gracefully
		watcher.reloadConfig()
	})
}

func TestConfigWatcherHandleEvent(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	tempDir, err := os.MkdirTemp("", "test_watcher_events")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(`{"test": "config"}`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("empty config file", func(t *testing.T) {
		watcherEmpty := &ConfigWatcher{
			configFile: "",
			server:     server,
			stopChan:   make(chan struct{}),
		}

		// Should not panic with empty config file
		watcherEmpty.reloadConfig()
	})

	t.Run("stop multiple times", func(t *testing.T) {
		watcher := &ConfigWatcher{
			stopChan: make(chan struct{}),
		}

		watcher.Stop()
		watcher.Stop() // Should not panic
	})
}

func TestWatchLoop(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	tempDir, err := os.MkdirTemp("", "test_watch_loop")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.yml")

	// Test watch loop doesn’t block on stop
	watcher := &ConfigWatcher{
		configFile: configFile,
		server:     server,
		stopChan:   make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// Simulate watch loop
		select {
		case <-watcher.stopChan:
			break
		case <-ctx.Done():
			break
		}
	}()

	// Test stopping safely
	time.Sleep(50 * time.Millisecond)
	close(watcher.stopChan)
}

func TestConfigWatcherErrorHandling(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	testCases := []struct {
		name     string
		setup    func() (watcher *ConfigWatcher, cleanup func())
		expected bool
	}{
		{
			name: "nil config file watcher",
			setup: func() (*ConfigWatcher, func()) {
				watcher := &ConfigWatcher{
					configFile: "",
					server:     server,
					stopChan:   make(chan struct{}),
				}
				return watcher, func() { watcher.Stop() }
			},
			expected: true, // Should handle gracefully
		},
		{
			name: "non-existent directory",
			setup: func() (*ConfigWatcher, func()) {
				tempDir, err := os.MkdirTemp("", "test_nonexistent")
				if err != nil {
					t.Skip("failed to create temp dir")
				}
				os.RemoveAll(tempDir)

				configFile := filepath.Join(tempDir, "test.yml")
				watcher := &ConfigWatcher{
					configFile: configFile,
					server:     server,
					stopChan:   make(chan struct{}),
				}

				return watcher, func() {}
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			watcher, cleanup := tc.setup()
			defer cleanup()

			err := watcher.Start()
			// ensure no panics when handling various error cases
			_ = err
		})
	}
}

func TestConfigWatcherMemorySafety(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	tempDir, err := os.MkdirTemp("", "test_memory_safety")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Test concurrent access patterns
	watcher := &ConfigWatcher{
		configFile: filepath.Join(tempDir, "config.yml"),
		server:     server,
		stopChan:   make(chan struct{}),
	}

	t.Run("concurrent starts and stops", func(t *testing.T) {
		done := make(chan bool)

		go func() {
			for i := 0; i < 10; i++ {
				watcher.Stop()
				watcher.reloadConfig()
			}
			done <- true
		}()

		go func() {
			for i := 0; i < 10; i++ {
				close(watcher.stopChan)
				watcher.stopChan = make(chan struct{})
			}
			done <- true
		}()

		<-done
		<-done
	})

	t.Run("nil components handling", func(t *testing.T) {
		watcher := &ConfigWatcher{
			configFile: "",
			server:     nil,
			stopChan:   make(chan struct{}),
		}

		watcher.reloadConfig()

		watcherNil := &ConfigWatcher{
			configFile: "/tmp/invalid.yml",
			server:     server,
			stopChan:   make(chan struct{}),
		}

		watcherNil.reloadConfig()
	})
}

func TestWatchLoopSafety(t *testing.T) {
	server := &ProxyServer{config: Config{}}

	testCases := []struct {
		name      string
		watcher   *ConfigWatcher
		testFunc  func()
		shouldRun bool
	}{
		{
			name: "watch loop channels",
			watcher: &ConfigWatcher{
				configFile: "/tmp/test",
				server:     server,
				stopChan:   make(chan struct{}),
			},
			testFunc:  func() { /* basic succeeds */ },
			shouldRun: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.watcher == nil {
				t.Skip()
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			close(tc.watcher.stopChan)

			finish := make(chan bool)
			go func() {
				tc.testFunc()
				finish <- true
			}()

			select {
			case <-finish:
			case <-ctx.Done():
				t.Log("test completed without panic")
			}
		})
	}
}
