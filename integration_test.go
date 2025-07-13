package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestIntegration_ModelParameterApplication tests the end-to-end model parameter application
func TestIntegration_ModelParameterApplication(t *testing.T) {
	// Create temporary config and database
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test_config.yml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Create test configuration with enhanced models
	config := map[string]interface{}{
		"host":         "localhost",
		"port":         5050,
		"databasefile": dbFile,
		"pageSize":     15,
		"providers": map[string]interface{}{
			"test_provider": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"apikey":  "test-key",
				"models":  []string{"simple-model", "thinking-model"},
				"modelparams": map[string]interface{}{
					"thinking-model": map[string]interface{}{
						"think":       true,
						"temperature": 0.7,
					},
				},
			},
			"ollama": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"enabled": false,
			},
		},
	}

	configYAML, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	err = os.WriteFile(configFile, configYAML, 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set environment variable
	oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
	os.Setenv("LLMSEE_CONFIGFILE", configFile)
	defer os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)

	// Create server
	server, err := NewProxyServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.db.Close()

	// Create a mock upstream server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request body to verify parameters were applied
		var requestBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			return
		}

		// Verify that the model parameters were applied
		if requestBody["think"] != true {
			t.Errorf("expected think parameter to be true, got %v", requestBody["think"])
		}
		if requestBody["temperature"] != 0.7 {
			t.Errorf("expected temperature parameter to be 0.7, got %v", requestBody["temperature"])
		}

		// Return a mock response
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "Hello from thinking model!",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Update the provider config to point to our mock server
	providerConfig := server.config.Providers["test_provider"]
	providerConfig.BaseURL = mockServer.URL
	server.config.Providers["test_provider"] = providerConfig

	// Test request with thinking-model
	requestBody := map[string]interface{}{
		"model": "thinking-model",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Hello",
			},
		},
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	// Make request to the proxy
	req := httptest.NewRequest("POST", "/test_provider/v1/chat/completions", bytes.NewReader(requestJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleProxy(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify response
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		t.Error("expected choices in response")
	}
}

// TestIntegration_V1ModelsEndpoint tests the /v1/models endpoint with enhanced models
func TestIntegration_V1ModelsEndpoint(t *testing.T) {
	// Create temporary config and database
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test_config.yml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Create test configuration
	config := map[string]interface{}{
		"databasefile": dbFile,
		"providers": map[string]interface{}{
			"provider1": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"models":  []string{"simple-model", "enhanced-model"},
				"modelparams": map[string]interface{}{
					"enhanced-model": map[string]interface{}{
						"think":       true,
						"temperature": 0.8,
					},
				},
			},
			"provider2": map[string]interface{}{
				"baseurl": "http://localhost:11435/v1",
				"models":  []string{"another-model"},
			},
			"ollama": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"enabled": false,
			},
		},
	}

	configYAML, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	err = os.WriteFile(configFile, configYAML, 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set environment variable
	oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
	os.Setenv("LLMSEE_CONFIGFILE", configFile)
	defer os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)

	// Create server
	server, err := NewProxyServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.db.Close()

	// Test /v1/models endpoint
	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()

	server.handleProxy(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := response["data"].([]interface{})
	if !ok {
		t.Fatal("expected data array in response")
	}

	// Should have 3 models total
	if len(data) != 3 {
		t.Errorf("expected 3 models, got %d", len(data))
	}

	// Check that model IDs are prefixed with provider names
	expectedModels := map[string]bool{
		"provider1:simple-model":   false,
		"provider1:enhanced-model": false,
		"provider2:another-model":  false,
	}

	for _, item := range data {
		model, ok := item.(map[string]interface{})
		if !ok {
			t.Error("expected model to be an object")
			continue
		}

		id, ok := model["id"].(string)
		if !ok {
			t.Error("expected model to have string id")
			continue
		}

		if _, exists := expectedModels[id]; exists {
			expectedModels[id] = true
		} else {
			t.Errorf("unexpected model ID: %s", id)
		}

		// Check if enhanced model has additional parameters
		if id == "provider1:enhanced-model" {
			if model["think"] != true {
				t.Errorf("expected enhanced-model to have think=true, got %v", model["think"])
			}
			if model["temperature"] != 0.8 {
				t.Errorf("expected enhanced-model to have temperature=0.8, got %v", model["temperature"])
			}
		}
	}

	// Verify all expected models were found
	for modelID, found := range expectedModels {
		if !found {
			t.Errorf("expected model %s not found in response", modelID)
		}
	}
}

// TestIntegration_ProviderModelsEndpoint tests provider-specific models endpoint
func TestIntegration_ProviderModelsEndpoint(t *testing.T) {
	// Create temporary config and database
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test_config.yml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Create test configuration
	config := map[string]interface{}{
		"databasefile": dbFile,
		"providers": map[string]interface{}{
			"test_provider": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"models":  []string{"model1", "model2"},
				"modelparams": map[string]interface{}{
					"model2": map[string]interface{}{
						"think":  true,
						"custom": "value",
					},
				},
			},
			"ollama": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"enabled": false,
			},
		},
	}

	configYAML, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	err = os.WriteFile(configFile, configYAML, 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set environment variable
	oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
	os.Setenv("LLMSEE_CONFIGFILE", configFile)
	defer os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)

	// Create server
	server, err := NewProxyServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.db.Close()

	// Test provider-specific models endpoint
	req := httptest.NewRequest("GET", "/test_provider/models", nil)
	w := httptest.NewRecorder()

	server.handleProxy(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := response["data"].([]interface{})
	if !ok {
		t.Fatal("expected data array in response")
	}

	if len(data) != 2 {
		t.Errorf("expected 2 models, got %d", len(data))
	}

	// Check models
	for _, item := range data {
		model, ok := item.(map[string]interface{})
		if !ok {
			t.Error("expected model to be an object")
			continue
		}

		id, ok := model["id"].(string)
		if !ok {
			t.Error("expected model to have string id")
			continue
		}

		if id == "model2" {
			if model["think"] != true {
				t.Errorf("expected model2 to have think=true, got %v", model["think"])
			}
			if model["custom"] != "value" {
				t.Errorf("expected model2 to have custom=value, got %v", model["custom"])
			}
		}
	}
}

// TestIntegration_BackwardCompatibility tests that old string-based model configs still work
func TestIntegration_BackwardCompatibility(t *testing.T) {
	// Create temporary config and database
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test_config.yml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Create legacy configuration (string array models)
	configYAML := `
databasefile: ` + strings.ReplaceAll(dbFile, "\\", "\\\\") + `
providers:
  legacy_provider:
    baseurl: http://localhost:11434/v1
    models:
      - model1
      - model2
      - model3
  ollama:
    baseurl: http://localhost:11434/v1
    enabled: false
`

	err := os.WriteFile(configFile, []byte(configYAML), 0644)

	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set environment variable
	oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
	os.Setenv("LLMSEE_CONFIGFILE", configFile)
	defer os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)

	// Create server
	server, err := NewProxyServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.db.Close()

	// Verify that models were parsed correctly
	provider := server.config.Providers["legacy_provider"]
	if provider.Models == nil {
		t.Fatal("expected models to be set")
	}

	models := *provider.Models
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}

	expectedNames := []string{"model1", "model2", "model3"}
	for i, model := range models {
		if model != expectedNames[i] {
			t.Errorf("expected model %d to be %s, got %s", i, expectedNames[i], model)
		}
	}

	// Test the models endpoint
	req := httptest.NewRequest("GET", "/legacy_provider/models", nil)
	w := httptest.NewRecorder()

	server.handleProxy(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := response["data"].([]interface{})
	if !ok {
		t.Fatal("expected data array in response")
	}

	if len(data) != 3 {
		t.Errorf("expected 3 models in response, got %d", len(data))
	}
}

// TestIntegration_RequestLogging tests that requests are properly logged to the database
func TestIntegration_RequestLogging(t *testing.T) {
	// Create temporary config and database
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test_config.yml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Create test configuration
	config := map[string]interface{}{
		"databasefile": dbFile,
		"providers": map[string]interface{}{
			"test_provider": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"models":  []string{"test-model"},
				"modelparams": map[string]interface{}{
					"test-model": map[string]interface{}{
						"temperature": 0.5,
					},
				},
			},
			"ollama": map[string]interface{}{
				"baseurl": "http://localhost:11434/v1",
				"enabled": false,
			},
		},
	}

	configYAML, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	err = os.WriteFile(configFile, configYAML, 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set environment variable
	oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
	os.Setenv("LLMSEE_CONFIGFILE", configFile)
	defer os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)

	// Create server
	server, err := NewProxyServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer server.db.Close()

	// Create a mock upstream server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate some processing time
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": "test"}`))
	}))
	defer mockServer.Close()

	// Update the provider config to point to our mock server
	providerConfig := server.config.Providers["test_provider"]
	providerConfig.BaseURL = mockServer.URL
	server.config.Providers["test_provider"] = providerConfig

	// Make a request
	requestBody := map[string]interface{}{
		"model":    "test-model",
		"messages": []interface{}{},
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest("POST", "/test_provider/v1/chat/completions", bytes.NewReader(requestJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-client/1.0")
	w := httptest.NewRecorder()

	server.handleProxy(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Give a moment for the database write to complete
	time.Sleep(50 * time.Millisecond)

	// Check that the request was logged
	logs, totalCount, err := server.getLogEntries(1, 10)
	if err != nil {
		t.Fatalf("failed to get log entries: %v", err)
	}

	if totalCount != 1 {
		t.Errorf("expected 1 log entry, got %d", totalCount)
	}

	if len(logs) != 1 {
		t.Errorf("expected 1 log entry in results, got %d", len(logs))
	}

	logEntry := logs[0]
	if logEntry.Provider != "test_provider" {
		t.Errorf("expected provider 'test_provider', got %s", logEntry.Provider)
	}
	if logEntry.Model != "test-model" {
		t.Errorf("expected model 'test-model', got %s", logEntry.Model)
	}
	if logEntry.Method != "POST" {
		t.Errorf("expected method 'POST', got %s", logEntry.Method)
	}
	if logEntry.ResponseStatus != 200 {
		t.Errorf("expected response status 200, got %d", logEntry.ResponseStatus)
	}
	if logEntry.DurationMs <= 0 {
		t.Errorf("expected positive duration, got %d", logEntry.DurationMs)
	}
}
