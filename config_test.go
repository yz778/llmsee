package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestNewModelStructure tests the new model configuration structure
func TestNewModelStructure(t *testing.T) {
	tests := []struct {
		name           string
		configJSON     string
		expectedModels []string
		expectedParams map[string]map[string]interface{}
	}{
		{
			name: "simple models without parameters",
			configJSON: `{
				"providers": {
					"test": {
						"baseurl": "http://test.com",
						"models": ["model1", "model2"]
					}
				}
			}`,
			expectedModels: []string{"model1", "model2"},
			expectedParams: nil,
		},
		{
			name: "models with parameters",
			configJSON: `{
				"providers": {
					"test": {
						"baseurl": "http://test.com",
						"models": ["thinking-model", "creative-model"],
						"modelparams": {
							"thinking-model": {
								"think": true,
								"temperature": 0.7
							},
							"creative-model": {
								"think": false,
								"temperature": 1.2,
								"top_p": 0.9
							}
						}
					}
				}
			}`,
			expectedModels: []string{"thinking-model", "creative-model"},
			expectedParams: map[string]map[string]interface{}{
				"thinking-model": {
					"think":       true,
					"temperature": 0.7,
				},
				"creative-model": {
					"think":       false,
					"temperature": 1.2,
					"top_p":       0.9,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config Config
			err := json.Unmarshal([]byte(tt.configJSON), &config)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			provider := config.Providers["test"]
			
			if tt.expectedModels != nil {
				if provider.Models == nil {
					t.Error("expected models to be set")
					return
				}
				
				models := *provider.Models
				if !reflect.DeepEqual(models, tt.expectedModels) {
					t.Errorf("Models = %v, want %v", models, tt.expectedModels)
				}
			}

			if tt.expectedParams != nil {
				if !reflect.DeepEqual(provider.ModelParams, tt.expectedParams) {
					t.Errorf("ModelParams = %v, want %v", provider.ModelParams, tt.expectedParams)
				}
			}
		})
	}
}

func TestProviderConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderConfig
		expected bool
	}{
		{
			name:     "enabled is nil (default enabled)",
			provider: ProviderConfig{Enabled: nil},
			expected: true,
		},
		{
			name:     "explicitly enabled",
			provider: ProviderConfig{Enabled: boolPtr(true)},
			expected: true,
		},
		{
			name:     "explicitly disabled",
			provider: ProviderConfig{Enabled: boolPtr(false)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.provider.IsEnabled()
			if result != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConfigParsing(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		wantErr    bool
		validate   func(*testing.T, *Config)
	}{
		{
			name: "mixed model configurations",
			configJSON: `{
				"host": "localhost",
				"port": 5050,
				"providers": {
					"test_provider": {
						"baseUrl": "http://localhost:11434/v1",
						"apikey": "test-key",
						"models": ["simple-model", "thinking-model"],
						"modelparams": {
							"thinking-model": {
								"think": true,
								"temperature": 0.7
							}
						}
					}
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				provider := config.Providers["test_provider"]
				if provider.Models == nil {
					t.Error("expected models to be set")
					return
				}

				models := *provider.Models
				if len(models) != 2 {
					t.Errorf("expected 2 models, got %d", len(models))
					return
				}

				// Check models are strings now
				if models[0] != "simple-model" {
					t.Errorf("expected first model to be 'simple-model', got %s", models[0])
				}
				if models[1] != "thinking-model" {
					t.Errorf("expected second model to be 'thinking-model', got %s", models[1])
				}

				// Check model parameters are in separate ModelParams map
				if provider.ModelParams == nil {
					t.Error("expected ModelParams to be set")
					return
				}
				if provider.ModelParams["thinking-model"]["think"] != true {
					t.Errorf("expected think parameter to be true, got %v", provider.ModelParams["thinking-model"]["think"])
				}
				if provider.ModelParams["thinking-model"]["temperature"] != 0.7 {
					t.Errorf("expected temperature parameter to be 0.7, got %v", provider.ModelParams["thinking-model"]["temperature"])
				}
			},
		},
		{
			name: "legacy string array models",
			configJSON: `{
				"providers": {
					"legacy_provider": {
						"baseUrl": "http://localhost:11434/v1",
						"models": ["model1", "model2", "model3"]
					}
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				provider := config.Providers["legacy_provider"]
				if provider.Models == nil {
					t.Error("expected models to be set")
					return
				}

				models := *provider.Models
				if len(models) != 3 {
					t.Errorf("expected 3 models, got %d", len(models))
					return
				}

				expectedNames := []string{"model1", "model2", "model3"}
				for i, model := range models {
					if model != expectedNames[i] {
						t.Errorf("expected model %d to be %s, got %s", i, expectedNames[i], model)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "test_config.json")
			
			err := os.WriteFile(configFile, []byte(tt.configJSON), 0644)
			if err != nil {
				t.Fatalf("failed to write test config file: %v", err)
			}

			// Set environment variable to use our test config
			oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
			os.Setenv("LLMSEE_CONFIGFILE", configFile)
			defer os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)

			config, err := getConfig()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	// Clear environment variables that might affect config
	oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
	oldHost := os.Getenv("LLMSEE_HOST")
	oldPort := os.Getenv("LLMSEE_PORT")
	
	// Set to a non-existent path to ensure no config file is found
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "nonexistent.json")
	
	os.Setenv("LLMSEE_CONFIGFILE", nonExistentPath)
	os.Setenv("LLMSEE_HOST", "")
	os.Setenv("LLMSEE_PORT", "")
	
	defer func() {
		os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)
		os.Setenv("LLMSEE_HOST", oldHost)
		os.Setenv("LLMSEE_PORT", oldPort)
	}()

	// Reset CLI args for test
	cliArgs = CLIArgs{}

	config, err := getConfig()
	if err != nil {
		t.Errorf("unexpected error getting default config: %v", err)
		return
	}

	if config.Host != "localhost" {
		t.Errorf("expected default host to be 'localhost', got %s", config.Host)
	}

	if config.Port != 5050 {
		t.Errorf("expected default port to be 5050, got %d", config.Port)
	}

	if config.PageSize != 20 {
		t.Errorf("expected default page size to be 20, got %d", config.PageSize)
	}

	if len(config.Providers) == 0 {
		t.Error("expected default config to have at least one provider")
	}
}

func TestCLIPortOverride(t *testing.T) {
	tests := []struct {
		name         string
		configJSON   string
		cliArgs      []string
		expectedPort int
	}{
		{
			name: "CLI port overrides config file port",
			configJSON: `{
				"port": 8080,
				"providers": {}
			}`,
			cliArgs:      []string{"-p", "9090"},
			expectedPort: 9090,
		},
		{
			name: "CLI port overrides default port",
			configJSON: `{
				"providers": {}
			}`,
			cliArgs:      []string{"-p", "7070"},
			expectedPort: 7070,
		},
		{
			name: "No CLI port uses config file port",
			configJSON: `{
				"port": 6060,
				"providers": {}
			}`,
			cliArgs:      []string{},
			expectedPort: 6060,
		},
		{
			name: "No CLI port and no config port uses default",
			configJSON: `{
				"providers": {}
			}`,
			cliArgs:      []string{},
			expectedPort: 5050, // default port
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "test_config.json")
			
			err := os.WriteFile(configFile, []byte(tt.configJSON), 0644)
			if err != nil {
				t.Fatalf("failed to write test config file: %v", err)
			}

			// Clear environment variables
			oldConfigFile := os.Getenv("LLMSEE_CONFIGFILE")
			oldHost := os.Getenv("LLMSEE_HOST")
			oldPort := os.Getenv("LLMSEE_PORT")
			
			os.Unsetenv("LLMSEE_CONFIGFILE")
			os.Unsetenv("LLMSEE_HOST")
			os.Unsetenv("LLMSEE_PORT")
			
			defer func() {
				os.Setenv("LLMSEE_CONFIGFILE", oldConfigFile)
				os.Setenv("LLMSEE_HOST", oldHost)
				os.Setenv("LLMSEE_PORT", oldPort)
			}()

			// Reset CLI args for test
			cliArgs = CLIArgs{}
			
			// Simulate CLI arguments
			args := append([]string{"-c", configFile}, tt.cliArgs...)
			os.Args = append([]string{"llmsee"}, args...)

			config, err := getConfig()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if config.Port != tt.expectedPort {
				t.Errorf("expected port %d, got %d", tt.expectedPort, config.Port)
			}
		})
	}
}

func TestParseCLIArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected CLIArgs
	}{
		{
			name: "config file and port",
			args: []string{"-c", "/path/to/config.json", "-p", "8080"},
			expected: CLIArgs{
				ConfigFile: "/path/to/config.json",
				Port:       8080,
			},
		},
		{
			name: "only config file",
			args: []string{"-c", "/path/to/config.json"},
			expected: CLIArgs{
				ConfigFile: "/path/to/config.json",
				Port:       0,
			},
		},
		{
			name: "only port",
			args: []string{"-p", "9090"},
			expected: CLIArgs{
				ConfigFile: "",
				Port:       9090,
			},
		},
		{
			name: "no arguments",
			args: []string{},
			expected: CLIArgs{
				ConfigFile: "",
				Port:       0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset CLI args for test
			cliArgs = CLIArgs{}
			
			// Set up CLI arguments
			os.Args = append([]string{"llmsee"}, tt.args...)

			result := parseCLIArgs()

			if result.ConfigFile != tt.expected.ConfigFile {
				t.Errorf("ConfigFile = %s, want %s", result.ConfigFile, tt.expected.ConfigFile)
			}
			if result.Port != tt.expected.Port {
				t.Errorf("Port = %d, want %d", result.Port, tt.expected.Port)
			}
		})
	}
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}