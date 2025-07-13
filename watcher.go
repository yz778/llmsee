package main

import (
	"context"
	"log"

	"github.com/fsnotify/fsnotify"
)

func (s *ProxyServer) watchConfig(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Failed to create config watcher: %v", err)
		return
	}
	defer watcher.Close()

	configFile := findConfigFile("")
	if configFile == "" {
		log.Printf("No config file found to watch")
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("Config file changed. Reloading...")
					s.reloadConfig()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Config watcher error: %v", err)
			}
		}
	}()

	err = watcher.Add(configFile)
	if err != nil {
		log.Printf("Failed to add config file to watcher: %v", err)
	}

	<-ctx.Done()
}

func (s *ProxyServer) reloadConfig() {
	s.mu.Lock()
	defer s.mu.Unlock()

	config, err := getConfig()
	if err != nil {
		log.Printf("Failed to reload config: %v", err)
		return
	}
	s.config = *config

	log.Println("Config reloaded successfully")

	// Send SSE event
	s.sendSSEUpdate(ServerUpdate{
		EventType: "config-reloaded",
		Message:   "Configuration reloaded",
	})
}
