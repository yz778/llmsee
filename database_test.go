package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestGetDb(t *testing.T) {
	// Create temporary database file
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")

	db, err := getDb(dbFile)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Test that the database file was created
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		t.Error("database file was not created")
	}

	// Test that we can ping the database
	if err := db.Ping(); err != nil {
		t.Errorf("failed to ping database: %v", err)
	}

	// Test that the logs table exists
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='logs'").Scan(&tableName)
	if err != nil {
		t.Errorf("logs table does not exist: %v", err)
	}
	if tableName != "logs" {
		t.Errorf("expected table name 'logs', got %s", tableName)
	}

	// Test that the index exists
	var indexName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_timestamp'").Scan(&indexName)
	if err != nil {
		t.Errorf("idx_timestamp index does not exist: %v", err)
	}
	if indexName != "idx_timestamp" {
		t.Errorf("expected index name 'idx_timestamp', got %s", indexName)
	}
}

func TestLogEntryOperations(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := getDb(dbFile)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Create a test server instance
	server := &ProxyServer{db: db}

	// Test data
	entry := LogEntry{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Provider:        "test_provider",
		Model:           "test_model",
		Method:          "POST",
		TargetURL:       "http://localhost:11434/v1/chat/completions",
		RequestHeaders:  `{"Content-Type": ["application/json"]}`,
		RequestBody:     `{"model": "test_model", "messages": []}`,
		RequestBodySize: 35,
		UserAgent:       "test-agent/1.0",
	}

	// Test insertLogRequest
	id, err := server.insertLogRequest(entry)
	if err != nil {
		t.Fatalf("failed to insert log request: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	// Update the entry with response data
	entry.Id = id
	entry.ResponseStatus = 200
	entry.ResponseHeaders = `{"Content-Type": ["application/json"]}`
	entry.ResponseBody = `{"choices": [{"message": {"content": "Hello!"}}]}`
	entry.ResponseBodySize = 45
	entry.DurationMs = 150

	// Test updateLogRequest
	err = server.updateLogRequest(entry)
	if err != nil {
		t.Fatalf("failed to update log request: %v", err)
	}

	// Test getLogEntries
	logs, totalCount, err := server.getLogEntries(1, 10)
	if err != nil {
		t.Fatalf("failed to get log entries: %v", err)
	}

	if totalCount != 1 {
		t.Errorf("expected total count 1, got %d", totalCount)
	}

	if len(logs) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(logs))
	}

	retrievedLog := logs[0]
	if retrievedLog.Id != id {
		t.Errorf("expected ID %d, got %d", id, retrievedLog.Id)
	}
	if retrievedLog.Provider != entry.Provider {
		t.Errorf("expected provider %s, got %s", entry.Provider, retrievedLog.Provider)
	}
	if retrievedLog.Model != entry.Model {
		t.Errorf("expected model %s, got %s", entry.Model, retrievedLog.Model)
	}
	if retrievedLog.ResponseStatus != entry.ResponseStatus {
		t.Errorf("expected response status %d, got %d", entry.ResponseStatus, retrievedLog.ResponseStatus)
	}

	// Test getLogDetail
	detailLog, err := server.getLogDetail(id)
	if err != nil {
		t.Fatalf("failed to get log detail: %v", err)
	}

	if detailLog.Id != id {
		t.Errorf("expected ID %d, got %d", id, detailLog.Id)
	}
	if detailLog.RequestBody != entry.RequestBody {
		t.Errorf("expected request body %s, got %s", entry.RequestBody, detailLog.RequestBody)
	}
	if detailLog.ResponseBody != entry.ResponseBody {
		t.Errorf("expected response body %s, got %s", entry.ResponseBody, detailLog.ResponseBody)
	}
}

func TestLogEntriesPagination(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := getDb(dbFile)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	server := &ProxyServer{db: db}

	// Insert multiple log entries
	numEntries := 25
	for i := 0; i < numEntries; i++ {
		entry := LogEntry{
			Timestamp:       time.Now().Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339),
			Provider:        "test_provider",
			Model:           "test_model",
			Method:          "POST",
			TargetURL:       "http://localhost:11434/v1/chat/completions",
			RequestHeaders:  `{"Content-Type": ["application/json"]}`,
			RequestBody:     `{"model": "test_model", "messages": []}`,
			RequestBodySize: 35,
			UserAgent:       "test-agent/1.0",
			ResponseStatus:  200,
			DurationMs:      100 + i,
		}

		_, err := server.insertLogRequest(entry)
		if err != nil {
			t.Fatalf("failed to insert log entry %d: %v", i, err)
		}
	}

	// Test first page
	logs, totalCount, err := server.getLogEntries(1, 10)
	if err != nil {
		t.Fatalf("failed to get log entries page 1: %v", err)
	}

	if totalCount != numEntries {
		t.Errorf("expected total count %d, got %d", numEntries, totalCount)
	}

	if len(logs) != 10 {
		t.Errorf("expected 10 log entries on page 1, got %d", len(logs))
	}

	// Test second page
	logs, totalCount, err = server.getLogEntries(2, 10)
	if err != nil {
		t.Fatalf("failed to get log entries page 2: %v", err)
	}

	if totalCount != numEntries {
		t.Errorf("expected total count %d, got %d", numEntries, totalCount)
	}

	if len(logs) != 10 {
		t.Errorf("expected 10 log entries on page 2, got %d", len(logs))
	}

	// Test last page
	logs, totalCount, err = server.getLogEntries(3, 10)
	if err != nil {
		t.Fatalf("failed to get log entries page 3: %v", err)
	}

	if totalCount != numEntries {
		t.Errorf("expected total count %d, got %d", numEntries, totalCount)
	}

	if len(logs) != 5 {
		t.Errorf("expected 5 log entries on page 3, got %d", len(logs))
	}

	// Test page beyond available data
	logs, totalCount, err = server.getLogEntries(4, 10)
	if err != nil {
		t.Fatalf("failed to get log entries page 4: %v", err)
	}

	if totalCount != numEntries {
		t.Errorf("expected total count %d, got %d", numEntries, totalCount)
	}

	if len(logs) != 0 {
		t.Errorf("expected 0 log entries on page 4, got %d", len(logs))
	}
}

func TestGetLogDetail_NotFound(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := getDb(dbFile)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	server := &ProxyServer{db: db}

	// Try to get a log detail that doesn't exist
	_, err = server.getLogDetail(999)
	if err == nil {
		t.Error("expected error when getting non-existent log detail")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDatabaseSchema(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := getDb(dbFile)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Test that all expected columns exist
	expectedColumns := []string{
		"id", "timestamp", "provider", "method", "model", "target_url",
		"request_headers", "request_body", "response_status", "response_headers",
		"response_body", "useragent", "duration_ms",
	}

	rows, err := db.Query("PRAGMA table_info(logs)")
	if err != nil {
		t.Fatalf("failed to get table info: %v", err)
	}
	defer rows.Close()

	var actualColumns []string
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			t.Fatalf("failed to scan column info: %v", err)
		}
		actualColumns = append(actualColumns, name)
	}

	if len(actualColumns) != len(expectedColumns) {
		t.Errorf("expected %d columns, got %d", len(expectedColumns), len(actualColumns))
	}

	for _, expectedCol := range expectedColumns {
		found := false
		for _, actualCol := range actualColumns {
			if actualCol == expectedCol {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected column %s not found", expectedCol)
		}
	}
}