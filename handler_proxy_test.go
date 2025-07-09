package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

func TestParseProvider(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		expectedProvider string
		expectedPath     string
	}{
		{
			name:             "root path",
			path:             "/",
			expectedProvider: "",
			expectedPath:     "",
		},
		{
			name:             "provider only",
			path:             "/ollama",
			expectedProvider: "ollama",
			expectedPath:     "",
		},
		{
			name:             "provider with path",
			path:             "/ollama/v1/chat/completions",
			expectedProvider: "ollama",
			expectedPath:     "v1/chat/completions",
		},
		{
			name:             "v1 endpoint",
			path:             "/v1/models",
			expectedProvider: "v1",
			expectedPath:     "models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			provider, path := parseProvider(req)

			if provider != tt.expectedProvider {
				t.Errorf("parseProvider() provider = %v, want %v", provider, tt.expectedProvider)
			}
			if path != tt.expectedPath {
				t.Errorf("parseProvider() path = %v, want %v", path, tt.expectedPath)
			}
		})
	}
}

func TestProxyHeaders(t *testing.T) {
	tests := []struct {
		name           string
		originalHeader http.Header
		providerConfig ProviderConfig
		expectedHeader http.Header
	}{
		{
			name: "basic header proxy with API key",
			originalHeader: http.Header{
				"Content-Type":  []string{"application/json"},
				"User-Agent":    []string{"test-agent"},
				"Authorization": []string{"Bearer old-key"},
				"Host":          []string{"example.com"}, // Should be removed
				"Cookie":        []string{"session=123"}, // Should be removed
			},
			providerConfig: ProviderConfig{
				ApiKey: "new-api-key",
			},
			expectedHeader: http.Header{
				"Content-Type":  []string{"application/json"},
				"User-Agent":    []string{"test-agent"},
				"Authorization": []string{"Bearer new-api-key"},
			},
		},
		{
			name: "header mapping",
			originalHeader: http.Header{
				"X-Custom-Header": []string{"custom-value"},
				"Content-Type":    []string{"application/json"},
			},
			providerConfig: ProviderConfig{
				HeaderMapping: map[string]string{
					"X-Custom-Header": "X-Mapped-Header",
				},
			},
			expectedHeader: http.Header{
				"X-Mapped-Header": []string{"custom-value"},
				"Content-Type":    []string{"application/json"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := proxyHeaders(tt.originalHeader, tt.providerConfig)

			for key, expectedValues := range tt.expectedHeader {
				actualValues := result[key]
				if !reflect.DeepEqual(actualValues, expectedValues) {
					t.Errorf("Header %s = %v, want %v", key, actualValues, expectedValues)
				}
			}

			// Check that unwanted headers are removed
			unwantedHeaders := []string{"Host", "Cookie", "Origin"}
			for _, header := range unwantedHeaders {
				if result.Get(header) != "" {
					t.Errorf("Expected header %s to be removed, but it's present", header)
				}
			}
		})
	}
}

func TestGetModels_CustomModels(t *testing.T) {
	server := &ProxyServer{}

	tests := []struct {
		name           string
		providerConfig ProviderConfig
		expectedModels []map[string]interface{}
	}{
		{
			name: "simple string models",
			providerConfig: ProviderConfig{
				Models: &[]string{"model1", "model2"},
			},
			expectedModels: []map[string]interface{}{
				{"id": "model1"},
				{"id": "model2"},
			},
		},
		{
			name: "models with parameters",
			providerConfig: ProviderConfig{
				Models: &[]string{"thinking-model", "creative-model"},
				ModelParams: map[string]map[string]interface{}{
					"thinking-model": {"think": true, "temperature": 0.7},
					"creative-model": {"think": false, "temperature": 1.2, "top_p": 0.9},
				},
			},
			expectedModels: []map[string]interface{}{
				{"id": "thinking-model", "think": true, "temperature": 0.7},
				{"id": "creative-model", "think": false, "temperature": 1.2, "top_p": 0.9},
			},
		},
		{
			name: "mixed models",
			providerConfig: ProviderConfig{
				Models: &[]string{"simple-model", "enhanced-model"},
				ModelParams: map[string]map[string]interface{}{
					"enhanced-model": {"think": true},
				},
			},
			expectedModels: []map[string]interface{}{
				{"id": "simple-model"},
				{"id": "enhanced-model", "think": true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			models, err := server.getModels(ctx, tt.providerConfig)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(models) != len(tt.expectedModels) {
				t.Errorf("expected %d models, got %d", len(tt.expectedModels), len(models))
				return
			}

			for i, expectedModel := range tt.expectedModels {
				if !reflect.DeepEqual(models[i], expectedModel) {
					t.Errorf("model %d = %v, want %v", i, models[i], expectedModel)
				}
			}
		})
	}
}

func TestWriteModels(t *testing.T) {
	tests := []struct {
		name           string
		providerConfig ProviderConfig
		expectedJSON   string
	}{
		{
			name: "simple models",
			providerConfig: ProviderConfig{
				Models: &[]string{"model1", "model2"},
			},
			expectedJSON: `{"object":"list","data":[{"id":"model1"},{"id":"model2"}]}`,
		},
		{
			name: "models with parameters",
			providerConfig: ProviderConfig{
				Models: &[]string{"thinking-model"},
				ModelParams: map[string]map[string]interface{}{
					"thinking-model": {"think": true, "temperature": 0.7},
				},
			},
			expectedJSON: `{"object":"list","data":[{"id":"thinking-model","temperature":0.7,"think":true}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeModels(w, tt.providerConfig)

			if w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("expected Content-Type to be application/json, got %s", w.Header().Get("Content-Type"))
			}

			// Parse both expected and actual JSON to compare (order-independent)
			var expected, actual map[string]interface{}
			if err := json.Unmarshal([]byte(tt.expectedJSON), &expected); err != nil {
				t.Fatalf("failed to unmarshal expected JSON: %v", err)
			}
			if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
				t.Fatalf("failed to unmarshal actual JSON: %v", err)
			}

			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("writeModels() = %s, want %s", w.Body.String(), tt.expectedJSON)
			}
		})
	}
}

func TestModelParameterApplication(t *testing.T) {
	// This test simulates the model parameter application logic
	tests := []struct {
		name           string
		model          string
		providerConfig ProviderConfig
		requestBody    map[string]interface{}
		expectedBody   map[string]interface{}
	}{
		{
			name:  "no custom models configured",
			model: "gpt-4",
			providerConfig: ProviderConfig{
				Models: nil,
			},
			requestBody: map[string]interface{}{
				"model":    "gpt-4",
				"messages": []interface{}{},
			},
			expectedBody: map[string]interface{}{
				"model":    "gpt-4",
				"messages": []interface{}{},
			},
		},
		{
			name:  "model not in custom list",
			model: "gpt-4",
			providerConfig: ProviderConfig{
				Models: &[]string{"other-model"},
				ModelParams: map[string]map[string]interface{}{
					"other-model": {"think": true},
				},
			},
			requestBody: map[string]interface{}{
				"model":    "gpt-4",
				"messages": []interface{}{},
			},
			expectedBody: map[string]interface{}{
				"model":    "gpt-4",
				"messages": []interface{}{},
			},
		},
		{
			name:  "apply model parameters",
			model: "thinking-model",
			providerConfig: ProviderConfig{
				Models: &[]string{"thinking-model"},
				ModelParams: map[string]map[string]interface{}{
					"thinking-model": {
						"think":       true,
						"temperature": 0.7,
					},
				},
			},
			requestBody: map[string]interface{}{
				"model":    "thinking-model",
				"messages": []interface{}{},
			},
			expectedBody: map[string]interface{}{
				"model":       "thinking-model",
				"messages":    []interface{}{},
				"think":       true,
				"temperature": 0.7,
			},
		},
		{
			name:  "override existing parameters",
			model: "thinking-model",
			providerConfig: ProviderConfig{
				Models: &[]string{"thinking-model"},
				ModelParams: map[string]map[string]interface{}{
					"thinking-model": {
						"temperature": 0.7,
					},
				},
			},
			requestBody: map[string]interface{}{
				"model":       "thinking-model",
				"messages":    []interface{}{},
				"temperature": 0.5, // This should be overridden
			},
			expectedBody: map[string]interface{}{
				"model":       "thinking-model",
				"messages":    []interface{}{},
				"temperature": 0.7, // Overridden by model config
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the model parameter application logic from handleProxy
			bodyJSON := make(map[string]interface{})
			for k, v := range tt.requestBody {
				bodyJSON[k] = v
			}

			// Apply model-specific parameters if configured
			if tt.providerConfig.Models != nil && tt.providerConfig.ModelParams != nil {
				// Check if the model is in the models list and has parameters
				for _, modelName := range *tt.providerConfig.Models {
					if modelName == tt.model {
						if params, exists := tt.providerConfig.ModelParams[tt.model]; exists {
							// Apply model parameters to the request body
							for k, v := range params {
								bodyJSON[k] = v
							}
						}
						break
					}
				}
			}

			if !reflect.DeepEqual(bodyJSON, tt.expectedBody) {
				t.Errorf("body after parameter application = %v, want %v", bodyJSON, tt.expectedBody)
			}
		})
	}
}

func TestHandleCORS(t *testing.T) {
	server := &ProxyServer{}

	tests := []struct {
		name           string
		method         string
		expectedReturn bool
		checkHeaders   bool
	}{
		{
			name:           "OPTIONS request",
			method:         "OPTIONS",
			expectedReturn: true,
			checkHeaders:   true,
		},
		{
			name:           "GET request",
			method:         "GET",
			expectedReturn: false,
			checkHeaders:   true,
		},
		{
			name:           "POST request",
			method:         "POST",
			expectedReturn: false,
			checkHeaders:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			w := httptest.NewRecorder()

			result := server.handleCORS(w, req)

			if result != tt.expectedReturn {
				t.Errorf("handleCORS() = %v, want %v", result, tt.expectedReturn)
			}

			if tt.checkHeaders {
				expectedHeaders := map[string]string{
					"Access-Control-Allow-Origin":      "*",
					"Access-Control-Allow-Credentials": "true",
					"Access-Control-Allow-Methods":     "GET, POST, PUT, DELETE, OPTIONS",
					"Access-Control-Allow-Headers":     "*",
				}

				for header, expectedValue := range expectedHeaders {
					actualValue := w.Header().Get(header)
					if actualValue != expectedValue {
						t.Errorf("Header %s = %s, want %s", header, actualValue, expectedValue)
					}
				}
			}

			if tt.method == "OPTIONS" && w.Code != http.StatusNoContent {
				t.Errorf("OPTIONS request should return 204, got %d", w.Code)
			}
		})
	}
}

func TestDecompressBody(t *testing.T) {
	testData := "Hello, World!"
	
	// Helper function to compress data with different encodings
	compressData := func(data string, encoding string) ([]byte, error) {
		switch encoding {
		case "gzip":
			var buf bytes.Buffer
			w := gzip.NewWriter(&buf)
			_, err := w.Write([]byte(data))
			if err != nil {
				return nil, err
			}
			err = w.Close()
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		case "deflate":
			var buf bytes.Buffer
			w := zlib.NewWriter(&buf)
			_, err := w.Write([]byte(data))
			if err != nil {
				return nil, err
			}
			err = w.Close()
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		case "br":
			var buf bytes.Buffer
			w := brotli.NewWriter(&buf)
			_, err := w.Write([]byte(data))
			if err != nil {
				return nil, err
			}
			err = w.Close()
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		case "zstd":
			var buf bytes.Buffer
			w, err := zstd.NewWriter(&buf)
			if err != nil {
				return nil, err
			}
			_, err = w.Write([]byte(data))
			if err != nil {
				return nil, err
			}
			err = w.Close()
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		case "snappy":
			var buf bytes.Buffer
			w := snappy.NewBufferedWriter(&buf)
			_, err := w.Write([]byte(data))
			if err != nil {
				return nil, err
			}
			err = w.Close()
			if err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		default:
			return []byte(data), nil
		}
	}
	
	tests := []struct {
		name     string
		encoding string
		wantErr  bool
	}{
		{
			name:     "no encoding",
			encoding: "",
			wantErr:  false,
		},
		{
			name:     "gzip encoding",
			encoding: "gzip",
			wantErr:  false,
		},
		{
			name:     "deflate encoding", 
			encoding: "deflate",
			wantErr:  false,
		},
		{
			name:     "brotli encoding",
			encoding: "br",
			wantErr:  false,
		},
		{
			name:     "zstd encoding",
			encoding: "zstd",
			wantErr:  false,
		},
		{
			name:     "snappy encoding",
			encoding: "snappy",
			wantErr:  false,
		},
		{
			name:     "unknown encoding",
			encoding: "unknown",
			wantErr:  false, // Should pass through unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compress the test data appropriately for the encoding
			compressedData, err := compressData(testData, tt.encoding)
			if err != nil {
				t.Fatalf("failed to compress test data: %v", err)
			}
			
			reader := bytes.NewReader(compressedData)
			result, err := decompressBody(reader, tt.encoding)

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result == nil {
				t.Error("expected non-nil reader")
			}
			
			// For compressed encodings, verify we can read back the original data
			if tt.encoding != "" && tt.encoding != "unknown" && result != nil {
				decompressedData, err := io.ReadAll(result)
				if err != nil {
					t.Errorf("failed to read decompressed data: %v", err)
				} else if string(decompressedData) != testData {
					t.Errorf("decompressed data = %q, want %q", string(decompressedData), testData)
				}
			}
		})
	}
}