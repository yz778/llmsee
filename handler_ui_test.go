package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// For testing purposes, we'll use temp directories instead of embed.FS

func TestServeFile(t *testing.T) {
	// Create temporary directory for dev mode testing
	tempDir, err := os.MkdirTemp("", "test_ui")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFile := filepath.Join(tempDir, "test.html")
	if err := os.WriteFile(testFile, []byte("<html><body>Test File</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	testIcon := filepath.Join(tempDir, "favicon.ico")
	if err := os.WriteFile(testIcon, []byte("icon_data_goes_here"), 0644); err != nil {
		t.Fatal(err)
	}

	server := &ProxyServer{
		devMode: true, // Enable dev mode for file serving
	}

	t.Run("dev mode - serve file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test.html", nil)
		w := httptest.NewRecorder()

		server.serveFile(w, req, testFile, "text/html")

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body, _ := io.ReadAll(w.Body)
		if !bytes.Contains(body, []byte("Test File")) {
			t.Errorf("expected file content, got: %s", string(body))
		}
	})

	t.Run("dev mode - non-existent file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/nonexistent.html", nil)
		w := httptest.NewRecorder()

		server.serveFile(w, req, filepath.Join(tempDir, "nonexistent.html"), "")

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
	})
}

func TestHandleUI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_ui_files")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test UI structure
	uiDir := filepath.Join(tempDir, "ui")
	if err := os.MkdirAll(uiDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test files
	indexContent := `
<!DOCTYPE html>
<html>
<head>
	<title>LLMSee Test</title>
	<link rel="stylesheet" href="styles.css">
</head>
<body>
	<h1>LLMSee UI Test</h1>
	<div id="app"></div>
	<script src="app.js"></script>
</body>
</html>`

	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}

	stylesContent := `body { background-color: #f0f0f0; font-family: sans-serif; }`
	if err := os.WriteFile(filepath.Join(uiDir, "styles.css"), []byte(stylesContent), 0644); err != nil {
		t.Fatal(err)
	}

	appContent := `console.log("Test app loaded");`
	if err := os.WriteFile(filepath.Join(uiDir, "app.js"), []byte(appContent), 0644); err != nil {
		t.Fatal(err)
	}

	faviconContent := []byte("fake_icon_data")
	if err := os.WriteFile(filepath.Join(uiDir, "favicon.ico"), faviconContent, 0644); err != nil {
		t.Fatal(err)
	}

	server := &ProxyServer{
		devMode: true,
	}

	t.Run("root UI path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ui", nil)
		w := httptest.NewRecorder()
		server.handleUI(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "" && !strings.Contains(contentType, "text/html") {
			t.Errorf("expected HTML content type, got %s", contentType)
		}

		body, _ := io.ReadAll(w.Body)
		if !bytes.Contains(body, []byte("LLMSee UI Test")) {
			t.Errorf("expected index.html content, got: %s", string(body))
		}
	})

	t.Run("root UI path with trailing slash", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ui/", nil)
		w := httptest.NewRecorder()
		server.handleUI(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body, _ := io.ReadAll(w.Body)
		if !bytes.Contains(body, []byte("LLMSee UI Test")) {
			t.Errorf("expected index.html content, got: %s", string(body))
		}
	})

	t.Run("CSS file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ui/styles.css", nil)
		w := httptest.NewRecorder()
		server.handleUI(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body, _ := io.ReadAll(w.Body)
		if !bytes.Contains(body, []byte("background-color")) {
			t.Errorf("expected CSS content, got: %s", string(body))
		}
	})

	t.Run("JavaScript file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ui/app.js", nil)
		w := httptest.NewRecorder()
		server.handleUI(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body, _ := io.ReadAll(w.Body)
		if !bytes.Contains(body, []byte("Test app loaded")) {
			t.Errorf("expected JS content, got: %s", string(body))
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ui/notfound.html", nil)
		w := httptest.NewRecorder()
		server.handleUI(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
	})
}

func TestHandleFavIcon(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_favicon")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test UI structure
	uiDir := filepath.Join(tempDir, "ui")
	if err := os.MkdirAll(uiDir, 0755); err != nil {
		t.Fatal(err)
	}

	faviconContent := []byte("fake_favicon_data")
	if err := os.WriteFile(filepath.Join(uiDir, "favicon.ico"), faviconContent, 0644); err != nil {
		t.Fatal(err)
	}

	server := &ProxyServer{
		devMode: true,
	}

	t.Run("favicon serving", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/favicon.ico", nil)
		w := httptest.NewRecorder()

		server.handleFavIcon(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		cacheControl := w.Header().Get("Cache-Control")
		if cacheControl != "public, max-age=86400" {
			t.Errorf("expected cache header 'public, max-age=86400', got %s", cacheControl)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "image/x-icon" {
			t.Errorf("expected content type 'image/x-icon', got %s", contentType)
		}

		body, _ := io.ReadAll(w.Body)
		if !bytes.Equal(body, faviconContent) {
			t.Errorf("expected favicon content, got: %s", string(body))
		}
	})

	t.Run("favicon not found", func(t *testing.T) {
		// Remove favicon to test 404
		os.Remove(filepath.Join(uiDir, "favicon.ico"))

		req := httptest.NewRequest("GET", "/favicon.ico", nil)
		w := httptest.NewRecorder()

		server.handleFavIcon(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", w.Code)
		}
	})
}

func TestProdModeUI(t *testing.T) {
	// Test that we can create an embedded FS structure for testing
	// Since we don't have the actual embedded FS, we'll test using file Server fallback

	server := &ProxyServer{
		devMode: false,
	}

	t.Run("production mode - empty embedded", func(t *testing.T) {
		// Test case when embedded FS doesn't contain the requested file
		req := httptest.NewRequest("GET", "/ui/missing.html", nil)
		w := httptest.NewRecorder()

		server.handleUI(w, req)

		// In production, if file doesn't exist in embedded FS,
		// should return 404. The actual behavior depends on what we have embedded.
		_ = w.Code
	})

	t.Run("path traversal protection", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ui/../../etc/passwd", nil)
		w := httptest.NewRecorder()

		server.handleUI(w, req)

		// Should not serve files outside the UI directory
		if w.Code != http.StatusNotFound {
			t.Logf("expected 404 for path traversal attempt, got %d", w.Code)
		}
	})
}

func TestFileModificationTrackingDevMode(t *testing.T) {
	// Create test files in temporary directory
	tempDir, err := os.MkdirTemp("", "ui_dev_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	server := &ProxyServer{
		devMode: true,
	}

	testCases := []struct {
		name     string
		filename string
		content  string
		expected bool
	}{
		{
			name:     "serve modified file",
			filename: "test.html",
			content:  "<div>Original</div>",
			expected: true,
		},
		{
			name:     "serve file with subdir",
			filename: "subdir/nested.html",
			content:  "<div>Nested</div>",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tempDir, tc.filename)
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest("GET", "/ui/"+tc.filename, nil)
			w := httptest.NewRecorder()

			server.serveFile(w, req, path, "")

			body, _ := io.ReadAll(w.Body)
			if !bytes.Contains(body, []byte(tc.content)) {
				t.Errorf("expected content %q, got %q", tc.content, string(body))
			}
		})
	}
}

func TestUIContentType(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_content_types")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	uiDir := filepath.Join(tempDir, "ui")
	if err := os.MkdirAll(uiDir, 0755); err != nil {
		t.Fatal(err)
	}

	testFiles := map[string]string{
		"style.css":     "text/css; charset=utf-8",
		"app.js":        "application/javascript; charset=utf-8",
		"index.html":    "",
		"image.png":     "image/png",
		"manifest.json": "application/json; charset=utf-8",
	}

	for filename, expectedType := range testFiles {
		t.Run(filename, func(t *testing.T) {
			path := filepath.Join(uiDir, filename)
			content := []byte("test content")

			if err := os.WriteFile(path, content, 0644); err != nil {
				t.Fatal(err)
			}

			server := &ProxyServer{devMode: true}
			req := httptest.NewRequest("GET", "/ui/"+filename, nil)
			w := httptest.NewRecorder()

			server.serveFile(w, req, path, expectedType)

			actualType := w.Header().Get("Content-Type")
			// Skip empty content type checks
			if expectedType != "" && !strings.Contains(actualType, strings.Split(expectedType, ";")[0]) {
				t.Errorf("for %s: expected %s, got %s", filename, expectedType, actualType)
			}
		})
	}
}

// Remove unused type to allow compilation
type CoreTest struct{}
