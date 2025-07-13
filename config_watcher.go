package main

import (
	"log"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// ConfigWatcher handles watching the config file for changes
type ConfigWatcher struct {
	watcher    *fsnotify.Watcher
	configFile string
	server     *ProxyServer
	stopChan   chan struct{}
}

// NewConfigWatcher creates a new config file watcher
func NewConfigWatcher(server *ProxyServer, configFile string) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	cw := &ConfigWatcher{
		watcher:    watcher,
		configFile: configFile,
		server:     server,
		stopChan:   make(chan struct{}),
	}

	return cw, nil
}

// Start begins watching the config file for changes
func (cw *ConfigWatcher) Start() error {
	if cw.configFile == "" {
		log.Println("No config file to watch")
		return nil
	}

	// Watch the directory containing the config file
	// This is more reliable than watching the file directly
	configDir := filepath.Dir(cw.configFile)
	err := cw.watcher.Add(configDir)
	if err != nil {
		return err
	}

	log.Printf("Watching config file: %s", cw.configFile)

	go cw.watchLoop()
	return nil
}

// Stop stops the config file watcher
func (cw *ConfigWatcher) Stop() {
	close(cw.stopChan)
	if cw.watcher != nil {
		cw.watcher.Close()
	}
}

// watchLoop is the main event loop for file watching
func (cw *ConfigWatcher) watchLoop() {
	for {
		select {
		case <-cw.stopChan:
			return
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return
			}
			cw.handleEvent(event)
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Config watcher error: %v", err)
		}
	}
}

// handleEvent processes file system events
func (cw *ConfigWatcher) handleEvent(event fsnotify.Event) {
	// Check if the event is for our config file
	if event.Name != cw.configFile {
		return
	}

	// Only respond to write and create events
	if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
		log.Printf("Config file changed: %s", event.Name)
		cw.reloadConfig()
	}
}

// reloadConfig reloads the configuration and notifies clients
func (cw *ConfigWatcher) reloadConfig() {
	log.Println("Reloading configuration...")

	// Load new config using the watcher's config file path
	newConfig, err := getConfig(cw.configFile)
	if err != nil {
		log.Printf("Failed to reload config: %v", err)
		// If config reload fails, we don't update the server's config,
		// and it continues to use the previous valid configuration.
		return
	}

	// Update server config
	cw.server.mu.Lock()
	cw.server.config = *newConfig
	cw.server.mu.Unlock()

	// Clear the model cache to force re-fetching with new config
	cw.server.clearModelCache()

	log.Println("Configuration reloaded successfully")

	// Send SSE notification to UI
	cw.server.sendSSEUpdate(ServerUpdate{
		EventType: "config_reload",
		Entry:     LogEntry{}, // Empty entry for config events
	})
}
