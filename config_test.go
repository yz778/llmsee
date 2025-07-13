package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultString(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		defaultV string
		expected string
	}{
		{"non-empty value", "test", "default", "test"},
		{"empty value", "", "default", "default"},
		{"whitespace", "   ", "default", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaultString(tt.value, tt.defaultV)
			if result != tt.expected {
				t.Errorf("defaultString(%q, %q) = %q, want %q", tt.value, tt.defaultV, result, tt.expected)
			}
		})
	}
}

func TestDefaultInt(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		defaultV int
		expected int
	}{
		{"valid int", "123", 456, 123},
		{"empty value", "", 456, 456},
		{"invalid int", "abc", 456, 456},
		{"negative int", "-123", 456, -123},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaultInt(tt.value, tt.defaultV)
			if result != tt.expected {
				t.Errorf("defaultInt(%q, %d) = %d, want %d", tt.value, tt.defaultV, result, tt.expected)
			}
		})
	}
}

func TestProviderConfig_IsEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		enabled  *bool
		expected bool
	}{
		{"nil enabled", nil, true},
		{"true enabled", &trueVal, true},
		{"false enabled", &falseVal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := ProviderConfig{Enabled: tt.enabled}
			result := pc.IsEnabled()
			if result != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "test_file")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	t.Run("existing file", func(t *testing.T) {
		if !fileExists(tempFile.Name()) {
			t.Errorf("fileExists(%q) = false, want true", tempFile.Name())
		}
	})

	t.Run("non-existing file", func(t *testing.T) {
		if fileExists("/nonexistent/path/to/file.txt") {
			t.Errorf("fileExists(nonexistent) = true, want false")
		}
	})
}

func TestParseCLIArgs(t *testing.T) {
	// Save original os.Args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	tests := []struct {
		name     string
		args     []string
		expected CLIArgs
	}{
		{
			name:     "no args",
			args:     []string{"llmsee"},
			expected: CLIArgs{ConfigFile: "", Port: 0},
		},
		{
			name:     "config file only",
			args:     []string{"llmsee", "-c", "test.yml"},
			expected: CLIArgs{ConfigFile: "test.yml", Port: 0},
		},
		{
			name:     "port only",
			args:     []string{"llmsee", "-p", "8080"},
			expected: CLIArgs{ConfigFile: "", Port: 8080},
		},
		{
			name:     "both config and port",
			args:     []string{"llmsee", "-c", "test.yml", "-p", "9000"},
			expected: CLIArgs{ConfigFile: "test.yml", Port: 9000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			result := parseCLIArgs()
			if result.ConfigFile != tt.expected.ConfigFile {
				t.Errorf("parseCLIArgs().ConfigFile = %q, want %q", result.ConfigFile, tt.expected.ConfigFile)
			}
			if result.Port != tt.expected.Port {
				t.Errorf("parseCLIArgs().Port = %d, want %d", result.Port, tt.expected.Port)
			}
		})
	}
}

func TestFindConfigFile(t *testing.T) {
	// Create temporary directory and files
	tempDir, err := os.MkdirTemp("", "test_config")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "llmsee.yml")
	if err := os.WriteFile(configPath, []byte("test: config"), 0644); err != nil {
		t.Fatal(err)
	}

	// Save original environment variables
	originalHome := os.Getenv("HOME")
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	originalAppData := os.Getenv("APPDATA")

	defer func() {
		os.Setenv("HOME", originalHome)
		os.Setenv("XDG_CONFIG_HOME", originalXDG)
		os.Setenv("APPDATA", originalAppData)
	}()

	tests := []struct {
		name      string
		cliFile   string
		setupFunc func()
		expected  string
	}{
		{
			name:      "cli argument provided",
			cliFile:   configPath,
			setupFunc: func() {},
			expected:  configPath,
		},
		{
			name:    "environment variable",
			cliFile: "",
			setupFunc: func() {
				os.Setenv("LLMSEE_CONFIGFILE", configPath)
			},
			expected: configPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset environment
			os.Unsetenv("LLMSEE_CONFIGFILE")
			os.Setenv("HOME", tempDir)

			tt.setupFunc()

			result := findConfigFile(tt.cliFile)
			if result != tt.expected {
				t.Errorf("findConfigFile(%q) = %q, want %q", tt.cliFile, result, tt.expected)
			}
		})
	}
}

func TestGetConfig(t *testing.T) {
	// Create temporary config file
	tempFile, err := os.CreateTemp("", "test_config_*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	configContent := `
host: testhost
port: 3000
databasefile: test.db
pagesize: 50
providers:
  testprovider:
    baseurl: http://test.com
    apikey: testkey
    enabled: true
`
	if err := os.WriteFile(tempFile.Name(), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("valid config from file", func(t *testing.T) {
		config, err := getConfig(tempFile.Name())
		if err != nil {
			t.Fatalf("getConfig() error = %v", err)
		}

		if config.Host != "testhost" {
			t.Errorf("config.Host = %q, want %q", config.Host, "testhost")
		}
		if config.Port != 3000 {
			t.Errorf("config.Port = %d, want %d", config.Port, 3000)
		}
		if len(config.Providers) != 1 {
			t.Errorf("len(config.Providers) = %d, want %d", len(config.Providers), 1)
		}
	})

	t.Run("invalid config file", func(t *testing.T) {
		invalidFile := "invalid-config.yml"
		if err := os.WriteFile(invalidFile, []byte("invalid: yaml: content"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(invalidFile)

		config, err := getConfig(invalidFile)
		if err != nil {
			t.Fatalf("getConfig() error = %v", err)
		}
		if config == nil {
			t.Error("getConfig() returned nil config")
		}
	})

	t.Run("no config file", func(t *testing.T) {
		config, err := getConfig("")
		if err != nil {
			t.Fatalf("getConfig() error = %v", err)
		}
		if config.Host == "" {
			t.Error("config.Host should not be empty")
		}
		if config.Port == 0 {
			t.Error("config.Port should not be 0")
		}
	})
}
