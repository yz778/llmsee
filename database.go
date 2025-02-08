package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

type LogEntry struct {
	Id               int64  `json:"id"`
	Timestamp        string `json:"timestamp"`
	Provider         string `json:"provider"`
	Method           string `json:"method"`
	Model            string `json:"model"`
	TargetURL        string `json:"target_url"`
	RequestHeaders   string `json:"request_headers"`
	RequestBody      string `json:"request_body"`
	RequestBodySize  int    `json:"request_body_size"`
	ResponseStatus   int    `json:"response_status"`
	ResponseHeaders  string `json:"response_headers"`
	ResponseBody     string `json:"response_body"`
	ResponseBodySize int    `json:"response_body_size"`
	UserAgent        string `json:"useragent"`
	DurationMs       int    `json:"duration_ms"`
}

type LogResponse struct {
	Logs        []LogEntry `json:"logs"`
	TotalPages  int        `json:"totalPages"`
	CurrentPage int        `json:"currentPage"`
	TotalLogs   int        `json:"totalLogs"`
}

func getDb(dbPath string) (db *sql.DB, err error) {
	log.Printf("Database file %s", dbPath)

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pooling
	db.SetMaxOpenConns(dbMaxOpenConns)
	db.SetMaxIdleConns(dbMaxIdleConns)
	db.SetConnMaxLifetime(dbConnMaxLifetime)

	// Test database connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}

	if err := initializeDatabase(db); err != nil {
		return nil, fmt.Errorf("creating log table: %w", err)
	}

	return db, nil
}

// initializeDatabase sets up the logs table and indexes
func initializeDatabase(db *sql.DB) error {
	_, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = normal;
		PRAGMA temp_store = memory;

		CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			provider TEXT NOT NULL,
			method TEXT NOT NULL,
			model TEXT NOT NULL,
			target_url TEXT NOT NULL,
			request_headers TEXT NOT NULL DEFAULT '',
			request_body TEXT NOT NULL DEFAULT '',
			response_status INT NOT NULL DEFAULT -1,
			response_headers TEXT NOT NULL DEFAULT '',
			response_body TEXT NOT NULL DEFAULT '',
			useragent TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT -1
		);

		CREATE INDEX IF NOT EXISTS idx_timestamp ON logs(timestamp);
	`)
	return err
}

// insertLogRequest stores the request logs into the database
func (s *ProxyServer) insertLogRequest(entry LogEntry) (id int64, err error) {
	result, err := s.db.Exec(`
		INSERT INTO logs (
			timestamp,
			provider,
			method,
			model,
			target_url,
			request_headers,
			request_body,
			response_headers,
			response_body,
			useragent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
		entry.Timestamp,
		entry.Provider,
		entry.Method,
		entry.Model,
		entry.TargetURL,
		entry.RequestHeaders,
		entry.RequestBody,
		entry.ResponseHeaders,
		entry.ResponseBody,
		entry.UserAgent,
	)
	if err != nil {
		return 0, err
	}

	id, err = result.LastInsertId()
	if err != nil {
		return 0, err
	}

	entry.Id = id

	log.Printf("[ID:%d] %s %s %s %d bytes sent", id, entry.Provider, entry.Method, entry.TargetURL, entry.RequestBodySize)
	s.sendSSEUpdate(ServerUpdate{EventType: "insert", Entry: entry})

	return id, nil
}

// updateLogRequest updates the log entry with response details and duration
func (s *ProxyServer) updateLogRequest(entry LogEntry) error {
	_, err := s.db.Exec(`
		UPDATE logs SET
			response_status = ?,
			response_headers = ?,
			response_body = ?,
			duration_ms = ?
		WHERE id = ?`,
		entry.ResponseStatus,
		entry.ResponseHeaders,
		entry.ResponseBody,
		entry.DurationMs,
		entry.Id,
	)
	if err != nil {
		return err
	}

	log.Printf("[ID:%d] %s (%d) %d bytes received in %dms", entry.Id, entry.Provider, entry.ResponseStatus, entry.ResponseBodySize, entry.DurationMs)
	s.sendSSEUpdate(ServerUpdate{EventType: "update", Entry: entry})
	return nil
}
