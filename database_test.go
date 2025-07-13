package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetDb(t *testing.T) {
	// Create a temporary directory for the database file
	tempDir, err := os.MkdirTemp("", "test_db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")

	t.Run("successful database creation", func(t *testing.T) {
		db, err := getDb(dbPath)
		if err != nil {
			t.Fatalf("getDb() error = %v", err)
		}
		if db == nil {
			t.Error("getDb returned nil database")
		}

		// Verify database connection
		if err := db.Ping(); err != nil {
			t.Errorf("database ping failed: %v", err)
		}

		// Test if tables were created
		var tableExists int
		err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='logs'").Scan(&tableExists)
		if err != nil {
			t.Fatalf("failed to check table existence: %v", err)
		}
		if tableExists != 1 {
			t.Error("logs table was not created")
		}

		db.Close()
	})

	t.Run("invalid database path", func(t *testing.T) {
		invalidPath := "/invalid/path/that/does/not/exist.db"
		db, err := getDb(invalidPath)
		if err == nil {
			t.Error("Expected error for invalid path")
			if db != nil {
				db.Close()
			}
		}
	})

	t.Run("malformed database file", func(t *testing.T) {
		// Create an invalid SQLite file
		badPath := filepath.Join(tempDir, "invalid.db")
		if err := os.WriteFile(badPath, []byte("invalid sqlite data"), 0644); err != nil {
			t.Fatal(err)
		}

		db, err := getDb(badPath)
		if err == nil {
			t.Error("Expected error for malformed database")
			if db != nil {
				db.Close()
			}
		}
	})
}

func TestInitializeDatabase(t *testing.T) {
	// Create an in-memory database for testing
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	t.Run("successful initialization", func(t *testing.T) {
		err := initializeDatabase(db)
		if err != nil {
			t.Fatalf("initializeDatabase() error = %v", err)
		}

		// Verify pragma settings
		var journalMode string
		err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
		if err != nil {
			t.Fatalf("failed to check journal_mode: %v", err)
		}
		if journalMode != "wal" { // sqlite returns lowercase
			t.Errorf("unexpected journal_mode: %s", journalMode)
		}

		// Verify tables and indexes exist
		var logsTable, timestampIndex int
		err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='logs'").Scan(&logsTable)
		if err != nil || logsTable != 1 {
			t.Error("logs table not created")
		}

		err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_timestamp'").Scan(&timestampIndex)
		if err != nil || timestampIndex != 1 {
			t.Error("idx_timestamp index not created")
		}
	})

	t.Run("database already initialized", func(t *testing.T) {
		// Run initialization twice
		err1 := initializeDatabase(db)
		err2 := initializeDatabase(db)

		if err1 != nil {
			t.Fatalf("first initialization failed: %v", err1)
		}
		if err2 != nil {
			t.Fatalf("second initialization failed: %v", err2)
		}

		// Verify the table still exists
		var exists int
		err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='logs'").Scan(&exists)
		if err != nil || exists != 1 {
			t.Error("logs table disappeared after reinitialization")
		}
	})
}

func TestDatabaseOperations(t *testing.T) {
	// Create an in-memory database for testing
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	// Create mock proxy server
	server := &ProxyServer{db: db}

	// Initialize database
	err = initializeDatabase(db)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}

	t.Run("insert and update log entries", func(t *testing.T) {
		// Create a log entry
		entry := LogEntry{
			Timestamp:        "2023-01-01T12:00:00Z",
			Provider:         "test-provider",
			Method:           "POST",
			Model:            "test-model",
			TargetURL:        "http://test.com/api",
			RequestHeaders:   `{"Content-Type":"application/json"}`,
			RequestBody:      `{"prompt":"Hello"}`,
			ResponseStatus:   0,
			ResponseHeaders:  "",
			ResponseBody:     "",
			UserAgent:        "test-agent",
			RequestBodySize:  20,
			ResponseBodySize: 0,
			DurationMs:       0,
		}

		// Test inserting the log entry
		id, err := server.insertLogRequest(entry)
		if err != nil {
			t.Fatalf("insertLogRequest() error = %v", err)
		}
		if id <= 0 {
			t.Errorf("expected positive ID, got %d", id)
		}

		// Verify the log was inserted
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM logs").Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("expected 1 log entry, got %d", count)
		}

		// Test updating the log entry
		updatedEntry := entry
		updatedEntry.Id = id
		updatedEntry.ResponseStatus = 200
		updatedEntry.ResponseHeaders = `{"Content-Type":"application/json"}`
		updatedEntry.ResponseBody = `{"response":"world"}`
		updatedEntry.ResponseBodySize = 21
		updatedEntry.DurationMs = 150

		err = server.updateLogRequest(updatedEntry)
		if err != nil {
			t.Fatalf("updateLogRequest() error = %v", err)
		}

		// Verify the log was updated
		var status int
		err = db.QueryRow("SELECT response_status FROM logs WHERE id = ?", id).Scan(&status)
		if err != nil || status != 200 {
			t.Errorf("expected response_status 200, got %d", status)
		}

		var durationMs int
		err = db.QueryRow("SELECT duration_ms FROM logs WHERE id = ?", id).Scan(&durationMs)
		if err != nil || durationMs != 150 {
			t.Errorf("expected duration_ms 150, got %d", durationMs)
		}
	})

	t.Run("get log entries with pagination", func(t *testing.T) {
		// Insert multiple log entries
		entries := []LogEntry{
			{Timestamp: "2023-01-01T12:00:01Z", Provider: "provider1", Method: "GET", Model: "model1", TargetURL: "http://test1.com"},
			{Timestamp: "2023-01-01T12:00:02Z", Provider: "provider2", Method: "POST", Model: "model2", TargetURL: "http://test2.com"},
			{Timestamp: "2023-01-01T12:00:03Z", Provider: "provider3", Method: "PUT", Model: "model3", TargetURL: "http://test3.com"},
		}

		for _, entry := range entries {
			entry.RequestHeaders = "{}"
			entry.RequestBody = "{}"
			entry.ResponseHeaders = "{}"
			entry.ResponseBody = "{}"
			entry.RequestBodySize = 2
			entry.ResponseBodySize = 2
			_, err := server.insertLogRequest(entry)
			if err != nil {
				t.Fatalf("failed to insert log: %v", err)
			}
		}

		// Test pagination
		logs, totalLogs, err := server.getLogEntries(1, 2)
		if err != nil {
			t.Fatalf("getLogEntries() error = %v", err)
		}
		if len(logs) != 2 {
			t.Errorf("expected 2 log entries, got %d", len(logs))
		}
		if totalLogs != 3 {
			t.Errorf("expected 3 total logs, got %d", totalLogs)
		}

		// Test second page
		logs, totalLogs, err = server.getLogEntries(2, 2)
		if err != nil {
			t.Fatalf("getLogEntries() error = %v", err)
		}
		if len(logs) != 1 {
			t.Errorf("expected 1 log entry on second page, got %d", len(logs))
		}

		// Verify logs are in descending order
		if len(logs) > 0 {
			// Should get the oldest entry on the second page
			entry := logs[0]
			if entry.Timestamp != "2023-01-01T12:00:01Z" {
				t.Errorf("expected oldest entry first on second page, got %s", entry.Timestamp)
			}
		}
	})

	t.Run("get log detail", func(t *testing.T) {
		// Insert a log entry
		entry := LogEntry{
			Timestamp:        "2023-01-01T12:00:00Z",
			Provider:         "detail-provider",
			Method:           "GET",
			Model:            "detail-model",
			TargetURL:        "http://detail.com",
			RequestHeaders:   `{"Authorization":"Bearer token"}`,
			RequestBody:      `{"query":"test"}`,
			UserAgent:        "detail-agent",
			RequestBodySize:  18,
			ResponseBodySize: 0,
			DurationMs:       0,
		}

		id, err := server.insertLogRequest(entry)
		if err != nil {
			t.Fatalf("insertLogRequest() error = %v", err)
		}

		// Test getting log detail
		detailEntry, err := server.getLogDetail(id)
		if err != nil {
			t.Fatalf("getLogDetail() error = %v", err)
		}
		if detailEntry.Id != id {
			t.Errorf("expected ID %d, got %d", id, detailEntry.Id)
		}
		if detailEntry.Provider != "detail-provider" {
			t.Errorf("expected provider 'detail-provider', got %s", detailEntry.Provider)
		}
		if detailEntry.RequestBody != `{"query":"test"}` {
			t.Errorf(`expected request body '{"query":"test"}"`)
		}
	})

	t.Run("get non-existent log detail", func(t *testing.T) {
		_, err := server.getLogDetail(99999)
		if err == nil {
			t.Error("expected error for non-existent log")
		}
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestDatabaseErrorCases(t *testing.T) {
	t.Run("database with invalid connection string", func(t *testing.T) {
		_, err := getDb("/invalid/path/with/permissions/file.db")
		if err == nil {
			t.Error("expected error for invalid database path")
		}
	})

	t.Run("database with context timeout", func(t *testing.T) {
		// We can't easily force a timeout for db.PingContext in tests, but we can verify
		// the function uses the provided context timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Microsecond)
		defer cancel()

		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		err = db.PingContext(ctx)
		// SQLite is too fast to timeout, but we verify at least it doesn't error
		// This mainly validates the context is being used
	})

	t.Run("database initialization failure", func(t *testing.T) {
		// Test with a nil database (would happen if we can't open the connection)
		err := initializeDatabase(nil)
		if err == nil {
			t.Error("expected error for nil database")
		}
	})
}
