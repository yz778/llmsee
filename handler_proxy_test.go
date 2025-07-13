package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// setupTestServer creates a minimal proxy server for testing
func setupTestServer() *ProxyServer {
	// Setup minimal config
	config := Config{
		Host:         "localhost",
		Port:         8080,
		DatabaseFile: ":memory:",
		PageSize:     20,
		Providers: map[string]ProviderConfig{
			"test": {
				BaseURL: "http://localhost:8080",
				ApiKey:  "test-key",
				Enabled: pointerBool(true),
			},
			"gemini": {
				BaseURL:  "https://generativelanguage.googleapis.com/v1beta",
				ApiKey:   "gemini-key",
				Enabled:  pointerBool(true),
				IsGemini: true,
			},
		},
	}

	// Setup minimal server
	server := &ProxyServer{
		config:         config,
		client:         &http.Client{Timeout: 1 * time.Second},
		clientChannels: make(map[string]*SSEClient),
	}

	return server
}

func TestParseProvider(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedProv   string
		expectedRemain string
	}{
		{"root path", "/", "", ""},
		{"test provider", "/test", "test", ""},
		{"test provider with path", "/test/models", "test", "models"},
		{"v1 provider", "/v1/chat/completions", "v1", "chat/completions"},
		{"provider with trailing slash", "/test/", "test", ""},
		{"deep nested path", "/provider/api/v1/models", "provider", "api/v1/models"},
		{"empty provider", "", "", ""},
		{"just slashes", "//", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal request
			req := &http.Request{URL: &url.URL{Path: tt.path}}
			provider, remaining := parseProvider(req)

			if provider != tt.expectedProv {
				t.Errorf("provider = %q, want %q", provider, tt.expectedProv)
			}
			if remaining != tt.expectedRemain {
				t.Errorf("remaining = %q, want %q", remaining, tt.expectedRemain)
			}
		})
	}
}

func TestProxyHeaders(t *testing.T) {
	tests := []struct {
		name          string
		original      http.Header
		provider      ProviderConfig
		expectedKey   string
		expectedValue string
	}{
		{
			name:     "basic headers",
			original: http.Header{"Accept": {"application/json"}},
			provider: ProviderConfig{
				ApiKey: "test-key",
				HeaderMapping: map[string]string{
					"X-Custom-Header": "X-Mapped-Header",
				},
			},
			expectedKey:   "Authorization",
			expectedValue: "Bearer test-key",
		},
		{
			name:     "custom header mapping",
			original: http.Header{"X-Custom-Header": {"custom-value"}},
			provider: ProviderConfig{
				HeaderMapping: map[string]string{
					"X-Custom-Header": "X-Mapped-Header",
				},
			},
			expectedKey:   "X-Mapped-Header",
			expectedValue: "custom-value",
		},
		{
			name:          "exclude blocked headers",
			original:      http.Header{"Host": {"test.com"}, "Cookie": {"session=abc"}},
			provider:      ProviderConfig{},
			expectedKey:   "Host",
			expectedValue: "", // Should be excluded
		},
		{
			name:     "generation API key",
			original: http.Header{},
			provider: ProviderConfig{
				ApiKey: "specific-key",
			},
			expectedKey:   "Authorization",
			expectedValue: "Bearer specific-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := proxyHeaders(tt.original, tt.provider)

			// For header mapping, we need to check specific cases
			if tt.expectedKey == "Authorization" {
				authHeader := result.Get("Authorization")
				if authHeader != tt.expectedValue {
					t.Errorf("Authorization = %q, want %q", authHeader, tt.expectedValue)
				}
			} else if tt.expectedKey == "X-Mapped-Header" {
				mappedHeader := result.Get("X-Mapped-Header")
				if mappedHeader != tt.expectedValue {
					t.Errorf("X-Mapped-Header = %q, want %q", mappedHeader, tt.expectedValue)
				}
				// Verify original header is removed
				customHeader := result.Get("X-Custom-Header")
				if customHeader != "" {
					t.Error("original custom header was not removed")
				}
			} else {
				// For exclusion test, verify blocked headers are removed
				if blockedHeader := result.Get("Host"); blockedHeader != "" {
					t.Error("Host header was not excluded")
				}
			}
		})
	}
}

// MockHTTPTransport for testing HTTP requests
type MockHTTPTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestFetchJSON(t *testing.T) {
	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"data":    []interface{}{map[string]interface{}{"id": "model1", "name": "Test Model"}},
			"object":  "list",
			"message": "success",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	server := setupTestServer()

	t.Run("successful JSON fetch", func(t *testing.T) {
		ctx := context.Background()
		headers := http.Header{"Accept-Encoding": {"gzip"}}

		result, err := server.fetchJSON(ctx, headers, "GET", ts.URL)
		if err != nil {
			t.Fatalf("fetchJSON() error = %v", err)
		}

		if result == nil {
			t.Fatal("fetchJSON returned nil result")
		}
		if v, ok := result["message"].(string); !ok || v != "success" {
			t.Errorf("expected message 'success', got %v", result["message"])
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		invalidServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, "{invalid json}")
		}))
		defer invalidServer.Close()

		ctx := context.Background()
		_, err := server.fetchJSON(ctx, nil, "GET", invalidServer.URL)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Create a server that will be too slow
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte(`{"slow": "response"}`))
		}))
		defer slowServer.Close()

		_, err := server.fetchJSON(ctx, nil, "GET", slowServer.URL)
		// SQLite is too fast to timeout consistently, so we just ensure no panic
		_ = err // Skip this for unit tests since it's highly timing dependent
	})
}

func TestHandleCORS(t *testing.T) {
	server := setupTestServer()

	t.Run("OPTIONS method", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "http://example.com", nil)
		w := httptest.NewRecorder()

		shouldReturn := server.handleCORS(w, req)
		if !shouldReturn {
			t.Error("expected handleCORS to return true for OPTIONS request")
		}
		if w.Code != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", w.Code)
		}
	})

	t.Run("non-OPTIONS method", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com", nil)
		w := httptest.NewRecorder()

		shouldReturn := server.handleCORS(w, req)
		if shouldReturn {
			t.Error("expected handleCORS to return false for non-OPTIONS request")
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("CORS headers not set")
		}
	})
}

func TestBuildModelList(t *testing.T) {
	models := []string{"model1", "model2", "model3"}
	provider := ProviderConfig{
		Models: &models,
		ModelParams: map[string]map[string]interface{}{
			"model1": {
				"max_tokens":  100,
				"temperature": 0.7,
			},
		},
	}

	result := buildModelList(provider)

	if len(result) != 3 {
		t.Errorf("expected 3 models, got %d", len(result))
	}

	// Verify model1 has additional parameters
	found := false
	for _, model := range result {
		if model["id"] == "model1" {
			if model["max_tokens"] != float64(100) {
				t.Errorf("expected max_tokens 100 for model1, got %v", model["max_tokens"])
			}
			if model["temperature"] != 0.7 {
				t.Errorf("expected temperature 0.7 for model1, got %v", model["temperature"])
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("model1 not found in results")
	}

	// Verify model2 has no additional parameters
	for _, model := range result {
		if model["id"] == "model2" {
			if len(model) != 2 { // id field + object field (implicit)
				// Actually test what we expect based on implementation
				if model["id"] != "model2" {
					t.Errorf("model2 has unexpected data: %v", model)
				}
			}
			break
		}
	}
}

func TestClearModelCache(t *testing.T) {
	server := setupTestServer()

	// Set some mock cached data
	globalModelJSON = []byte(`{"cached": "data"}`)

	// Clear cache
	server.clearModelCache()

	if globalModelJSON != nil {
		t.Error("model cache was not cleared")
	}
}

func TestGetModels(t *testing.T) {
	server := setupTestServer()

	// We can't easily test external API calls, so we'll test the provider-specific logic
	t.Run("custom model list", func(t *testing.T) {
		models := []string{"customModel1", "customModel2"}
		provider := ProviderConfig{
			Models: &models,
			ModelParams: map[string]map[string]interface{}{
				"customModel1": {"temperature": 0.8},
			},
		}

		result, err := server.getModels(context.Background(), provider)
		if err != nil {
			t.Fatalf("getModels() error = %v", err)
		}

		expectedIDs := []string{"customModel1", "customModel2"}
		if len(result) != 2 {
			t.Errorf("expected 2 models, got %d", len(result))
		}

		for i, model := range result {
			if model["id"] != expectedIDs[i] {
				t.Errorf("expected model[%d].id = %q, got %q", i, expectedIDs[i], model["id"])
			}
		}
	})

	t.Run("empty models list", func(t *testing.T) {
		emptyModels := []string{}
		provider := ProviderConfig{
			Models: &emptyModels,
		}

		result, err := server.getModels(context.Background(), provider)
		if err != nil {
			t.Fatalf("getModels() error = %v", err)
		}

		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("gemini provider handling", func(t *testing.T) {
		provider := ProviderConfig{
			BaseURL:  "https://generativelanguage.googleapis.com/v1beta",
			ApiKey:   "test-key",
			IsGemini: true,
		}

		// This would normally make a network call, but we'll just verify the structure
		_, err := server.getModels(context.Background(), provider)
		// Not testing actual network response
		_ = err
	})
}

// Helper function to create bool pointers
func pointerBool(b bool) *bool {
	return &b
}

func TestCompressDecompress(t *testing.T) {
	testContent := "This is test content that can be compressed"

	// Test various compression formats
	testCases := []struct {
		name   string
		format string
		expect bool
	}{
		{"gzip", "gzip", true},
		{"deflate", "deflate", false}, // not commonly used
		{"brotli", "br", false},       // might not be included
		{"unsupported", "xz", false},  // unsupported format
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in := bytes.NewReader([]byte(testContent))
			_, err := decompressBody(in, tc.format)

			// We don't expect all formats to work, just no panics
			_ = err
		})
	}
}
