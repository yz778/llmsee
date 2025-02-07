package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

const VERSION = "0.9.0"

type ProxyServer struct {
	devMode        bool
	config         Config
	db             *sql.DB
	mu             sync.RWMutex
	client         *http.Client
	clientChannels map[string]*SSEClient
}

// NewProxyServer initializes the proxy server
func NewProxyServer() (*ProxyServer, error) {
	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	db, err := getDb(config.DatabaseFile)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: httpRequestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        clientMaxIdleConns,
			MaxIdleConnsPerHost: clientMaxIdleConnsPerHost,
			IdleConnTimeout:     clientIdleConnTimeout,
		},
	}

	return &ProxyServer{
		devMode:        os.Getenv("APP_ENV") == "dev",
		config:         *config,
		db:             db,
		client:         client,
		clientChannels: make(map[string]*SSEClient),
	}, nil
}

// main function starts the server
func main() {
	log.Printf("LLMSee %s", VERSION)
	server, err := NewProxyServer()
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}
	defer server.db.Close()

	if server.devMode {
		log.Print("Developer mode enabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ui/sse", server.handleSse)
	mux.HandleFunc("/ui/", server.handleUI)
	mux.HandleFunc("/log", server.handleLogList)
	mux.HandleFunc("/log/detail", server.handleLogDetail)
	mux.HandleFunc("/favicon.ico", server.handleFavIcon)
	mux.HandleFunc("/", server.handleProxy)

	addr := fmt.Sprintf("%s:%d", server.config.Host, server.config.Port)

	httpServer := &http.Server{
		Addr:           addr,
		Handler:        mux,
		ReadTimeout:    httpReadTimeout,
		WriteTimeout:   httpWriteTimeout,
		IdleTimeout:    httpIdleConnTimeout,
		MaxHeaderBytes: httpMaxHeaderBytes,
	}

	// Start server in a goroutine
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait before proceeding to check if coroutine fails
	time.Sleep(100 * time.Millisecond)

	// Print server information
	log.Printf("Server Ready: http://%s", addr)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Stop all SSE
	server.gracefulShutdownSSE()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server stopped gracefully")
	}
}
