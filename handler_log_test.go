package main

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestHandleLogList(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Initialize database
	_, err = db.Exec(`
		CREATE TABLE logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			provider TEXT NOT NULL,
			method TEXT NOT NULL,
			model TEXT NOT NULL,
			target_url TEXT NOT NULL,
			request_headers TEXT DEFAULT '',
			request_body TEXT DEFAULT '',
			response_status INT DEFAULT -1,
			response_headers TEXT DEFAULT '',
			response_body TEXT DEFAULT '',
			useragent TEXT DEFAULT '',
			duration_ms INT DEFAULT -1
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert test data
	insertTestData := func(count int) {
		for i := 1; i <= count; i++ {
			_, err = db.Exec(`
				INSERT INTO logs (timestamp, provider, method, model, target_url, request_headers, response_headers, duration_ms)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, 
				"2023-01-01T12:00:00Z",
				"test-provider",
				"POST",
				"test-model",
				"http://test.com",
				"{}",
				"{}",
				i*100,
			)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	server := &ProxyServer{
		config: Config{PageSize: 10},
		db:     db,
	}

	t.Run("empty database", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/log", nil)
		w := httptest.NewRecorder()

		server.handleLogList(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var response LogResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		if err != nil {
			t.Fatal(err)
		}

		if len(response.Logs) != 0 {
			t.Errorf("expected 0 logs, got %d", len(response.Logs))
		}
		if response.TotalLogs != 0 {
			t.Errorf("expected 0 total logs, got %d", response.TotalLogs)
		}
		if response.TotalPages != 0 {
			t.Errorf("expected 0 total pages, got %d", response.TotalPages)
		}
		if response.CurrentPage != 1 {
			t.Errorf("expected current page 1, got %d", response.CurrentPage)
		}
	})

	t.Run("database with data", func(t *testing.T) {
		// Insert 25 test records
		insertTestData(25)

		req := httptest.NewRequest("GET", "/log", nil)
		w := httptest.NewRecorder()

		server.handleLogList(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var response LogResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		if err != nil {
			t.Fatal(err)
		}

		// Should get first page (10 items)
		if len(response.Logs) != 10 {
			t.Errorf("expected 10 logs on first page, got %d", len(response.Logs))
		}
		if response.TotalLogs != 25 {
			t.Errorf("expected 25 total logs, got %d", response.TotalLogs)
		}
		if response.TotalPages != 3 {
			t.Errorf("expected 3 total pages, got %d", response.TotalPages)
		}
		if response.CurrentPage != 1 {
			t.Errorf("expected current page 1, got %d", response.CurrentPage)
		}
	})

	t.Run("specific page", func(t *testing.T) {
		// Insert 25 test records
		insertTestData(25)

		req := httptest.NewRequest("GET", "/log?page=2", nil)
		w := httptest.NewRecorder()

		server.handleLogList(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var response LogResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		if err != nil {
			t.Fatal(err)
		}

		if response.CurrentPage != 2 {
			t.Errorf("expected current page 2, got %d", response.CurrentPage)
		}
	})

	t.Run("invalid page parameter", func(t *testing.T) {
		insertTestData(15)

		req := httptest.NewRequest("GET", "/log?page=invalid", nil)
		w := httptest.NewRecorder()

		server.handleLogList(w, req)

		var response LogResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		if err != nil {
			t.Fatal(err)
		}

		if response.CurrentPage != 1 {
			t.Errorf("expected default page 1, got %d", response.CurrentPage)
		}
	})

	t.Run("page out of bounds", func(t *testing.T) {
		insertTestData(5)

		req := httptest.NewRequest("GET", "/log?page=10", nil)
		w := httptest.NewRecorder()

		server.handleLogList(w, req)

		var response LogResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		if err != nil {
			t.Fatal(err)
		}

		// Should redirect to last available page
		if response.CurrentPage != 1 {
			t.Errorf("expected page 1, got %d", response.CurrentPage)
		}
	})

	t.Run("empty page size", func(t *testing.T) {
		// Create server with zero page size
		serverZero := &ProxyServer{
			config: Config{}, // PageSize defaults to 0
			db:     db,
		}
		insertTestData(5)

		req := httptest.NewRequest("GET", "/log", nil)
		w := httptest.NewRecorder()

		serverZero.handleLogList(w, req)

		var response LogResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		if err != nil {
			t.Fatal(err)
		}

		expectedPages := int(math.Ceil(float64(5) / float64(server.config.PageSize)))
		if response.TotalPages <= 0 {
			t.Errorf("expected %d total pages, got %d", expectedPages, response.TotalPages)
		}
	})

	t.Run("different content types", func(t *testing.T) {
		insertTestData(3)

		req := httptest.NewRequest("GET", "/log?page=1", nil)
		w := httptest.NewRecorder()

		server.handleLogList(w, req)

		ctype := w.Header().Get("Content-Type")
		if ctype != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %s", ctype)
		}
	})
}

func TestHandleLogDetail(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Initialize database
	_, err = db.Exec(`
		CREATE TABLE logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			provider TEXT NOT NULL,
			method TEXT NOT NULL,
			model TEXT NOT NULL,
			target_url TEXT NOT NULL,
			request_headers TEXT DEFAULT '',
			request_body TEXT DEFAULT '',
			request_body_size INT DEFAULT 0,
			response_status INT DEFAULT -1,
			response_headers TEXT DEFAULT '',
			response_body TEXT DEFAULT '',
			response_body_size INT DEFAULT 0,
			useragent TEXT DEFAULT '',
			duration_ms INT DEFAULT -1
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert test data
	result, err := db.Exec(`
		INSERT INTO logs (
			timestamp, provider, method, model, target_url, 
			request_headers, request_body, response_headers, response_body,
			useragent, response_status, request_body_size, response_body_size, duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 
		"2023-01-01T12:00:00Z",
		"test-provider",
		"POST", 
		"test-model",
		"http://test.com/api",
		`{"Content-Type":"application/json", "Authorization":"Bearer token"}`,
		`{"prompt":"test prompt"}`,
		`{"Content-Type":"application/json"}`,
		`{"response":"test response"}`,
		"Mozilla/5.0",
		200,
		42,
		55,
		123,
	)
	if err != nil {
		t.Fatal(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	server := &ProxyServer{db: db}

	t.Run("valid log ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/log/detail?id="+strconv.Itoa(int(id)), nil)
		w := httptest.NewRecorder()

		server.handleLogDetail(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var entry LogEntry
		err := json.NewDecoder(w.Body).Decode(&entry)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Id != id {
			t.Errorf("expected ID %d, got %d", id, entry.Id)
		}
		if entry.Provider != "test-provider" {
			t.Errorf("expected provider 'test-provider', got %s", entry.Provider)
		}
		if entry.Method != "POST" {
			t.Errorf("expected method 'POST', got %s", entry.Method)
		}
		if entry.Model != "test-model" {
			t.Errorf("expected model 'test-model', got %s", entry.Model)
		}
		if entry.RequestHeaders == "" {
			t.Error("expected request headers to be populated")
		}
		if entry.ResponseBody == "" {
			t.Error("expected response body to be populated")
		}
		if entry.RequestBodySize != 42 {
			t.Errorf("expected request body size 42, got %d", entry.RequestBodySize)
		}
		if entry.ResponseBodySize != 55 {
			t.Errorf("expected response body size 55, got %d", entry.ResponseBodySize)
		}
		if entry.DurationMs != 123 {
			t.Errorf("expected duration 123ms, got %d", entry.DurationMs)
		}
		if entry.ResponseStatus != 200 {
			t.Errorf("expected status 200, got %d", entry.ResponseStatus)
		}
	})

	t.Run("invalid log ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/log/detail?id=invalid", nil)
		w := httptest.NewRecorder()

		server.handleLogDetail(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("negative log ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/log/detail?id=-1", nil)
		w := httptest.NewRecorder()

		server.handleLogDetail(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("non-existent log ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/log/detail?id=99999", nil)
		w := httptest.NewRecorder()

		server.handleLogDetail(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
	})

	t.Run("missing log ID parameter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/log/detail", nil)
		w := httptest.NewRecorder()

		server.handleLogDetail(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("log ID zero", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/log/detail?id=0", nil)
		w := httptest.NewRecorder()

		server.handleLogDetail(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404 for ID 0, got %d", w.Code)
		}
	})
}

func TestLogResponseStructure(t *testing.T) {
	// Test the structure of LogResponse
	response := LogResponse{
		Logs: []LogEntry{
			{
				Id:        1,
				Timestamp: "2023-01-01T12:00:00Z",
				Provider:  "test",
				Method:    "GET",
				Model:     "model1",
				TargetURL: "http://test.com",
			},
		},
		TotalPages:  5,
		CurrentPage: 2,
		TotalLogs:   50,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}

	// Verify JSON structure
	var decoded LogResponse
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.TotalPages != 5 {
		t.Errorf("expected TotalPages 5, got %d", decoded.TotalPages)
	}
	if decoded.CurrentPage != 2 {
		t.Errorf("expected CurrentPage 2, got %d", decoded.CurrentPage)
	}
	if decoded.TotalLogs != 50 {
		t.Errorf("expected TotalLogs 50, got %d", decoded.TotalLogs)
	}
	if len(decoded.Logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(decoded.Logs))
	}
}

func TestErrNotFound(t *testing.T) {
	// Test the custom error
	if ErrNotFound.Error() != "resource not found" {
		t.Errorf(`expected "resource not found", got %q`, ErrNotFound.Error())
	}
}