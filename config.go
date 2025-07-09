package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// System configuration
const (
	defaultConfigFile         = "llmsee.json"
	clientIdleConnTimeout     = 90 * time.Second // Timeout for idle HTTP client
	clientMaxIdleConns        = 100              // Max idle connections for HTTP client
	clientMaxIdleConnsPerHost = 10               // Max idle connections per host for HTTP client
	dbConnMaxLifetime         = 1 * time.Minute  // Maximum lifetime of database connections
	dbMaxIdleConns            = 5                // Maximum idle database connections
	dbMaxOpenConns            = 5                // Maximum open database connections
	httpIdleConnTimeout       = 90 * time.Second // Timeout for idle HTTP server
	httpMaxHeaderBytes        = 1 << 20          // 1MB for HTTP max header size
	httpMaxRequestBodySize    = 10 << 20         // 10MB max size for request body
	httpReadTimeout           = 1 * time.Hour    // Timeout for reading requests
	httpRequestTimeout        = 1 * time.Hour    // HTTP request timeout
	httpWriteTimeout          = 1 * time.Hour    // Timeout for writing responses
	shutdownTimeout           = 10 * time.Second // Timeout for graceful shutdown
)

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Configuration structs
type Config struct {
	Host         string                    `json:"host"`
	Port         int                       `json:"port"`
	DatabaseFile string                    `json:"databasefile"`
	PageSize     int                       `json:"pagesize"`
	Providers    map[string]ProviderConfig `json:"providers"`
}

// ModelConfig represents a model configuration that can be either a string or an object with additional parameters
type ModelConfig struct {
	Name       string                 `json:"name,omitempty"`
	Parameters map[string]interface{} `json:"-"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ModelConfig
// Supports both string format: "model-name" and object format: {"name": "model-name", "think": true, ...}
func (m *ModelConfig) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		m.Name = str
		m.Parameters = make(map[string]interface{})
		return nil
	}

	// Try to unmarshal as object
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	// Extract name field
	if name, ok := obj["name"].(string); ok {
		m.Name = name
		delete(obj, "name") // Remove name from parameters
	} else {
		return fmt.Errorf("model object must have a 'name' field")
	}

	// Store remaining fields as parameters
	m.Parameters = obj
	return nil
}

// MarshalJSON implements custom JSON marshaling for ModelConfig
func (m *ModelConfig) MarshalJSON() ([]byte, error) {
	if len(m.Parameters) == 0 {
		// If no parameters, marshal as simple string
		return json.Marshal(m.Name)
	}

	// Marshal as object with name and parameters
	obj := make(map[string]interface{})
	obj["name"] = m.Name
	for k, v := range m.Parameters {
		obj[k] = v
	}
	return json.Marshal(obj)
}

type ProviderConfig struct {
	BaseURL       string            `json:"baseurl"`
	ApiKey        string            `json:"apikey"`
	HeaderMapping map[string]string `json:"headermapping"`
	Models        *[]ModelConfig    `json:"models"`
	Enabled       *bool             `json:"enabled"`
	IsGemini      bool              `json:"-"`
}

func (p *ProviderConfig) IsEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

func defaultString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	} else {
		return value
	}
}

func defaultInt(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	} else {
		value, _ := strconv.Atoi(value)
		return value
	}
}

var defaultConfig = Config{
	Host:         defaultString(os.Getenv("LLMSEE_HOST"), "localhost"),
	Port:         defaultInt(os.Getenv("LLMSEE_PORT"), 5050),
	DatabaseFile: "llmsee.db",
	PageSize:     20,
	Providers: map[string]ProviderConfig{
		"ollama": {
			BaseURL:       "http://" + defaultString(os.Getenv("LLMSEE_LOCALHOST"), "localhost") + ":11434/v1",
			ApiKey:        "",
			HeaderMapping: map[string]string{},
		},
	},
}

// CLI arguments structure
type CLIArgs struct {
	ConfigFile string
	Port       int
}

// parseCLIArgs parses command line arguments
func parseCLIArgs() CLIArgs {
	var args CLIArgs
	flag.StringVar(&args.ConfigFile, "c", "", "Path to JSON configuration file")
	flag.IntVar(&args.Port, "p", 0, "Port number to override config file setting")
	flag.Parse()
	return args
}

func getConfig() (config *Config, err error) {
	config = &Config{}
	
	// Parse CLI arguments first
	cliArgs := parseCLIArgs()
	
	// Find and load config file
	configFile := findConfigFile(cliArgs.ConfigFile)

	if configFile != "" {
		fileConfig, _ := os.ReadFile(configFile)
		if err := json.Unmarshal(fileConfig, &config); err != nil {
			log.Printf("Config file %s is not valid, skipping", configFile)
			config = &Config{}
		} else {
			log.Printf("Config file %s", configFile)
		}
	}

	if config.Host == "" {
		config.Host = defaultConfig.Host
	}

	// Apply CLI port override if provided, otherwise use config or default
	if cliArgs.Port > 0 {
		config.Port = cliArgs.Port
		log.Printf("Port overridden by CLI argument: %d", cliArgs.Port)
	} else if config.Port == 0 {
		config.Port = defaultConfig.Port
	}

	if config.DatabaseFile == "" {
		config.DatabaseFile = getDatabaseFile()
	}

	if config.PageSize == 0 {
		config.PageSize = defaultConfig.PageSize
	}

	// if there are no providers, assign the defaults
	if config.Providers == nil {
		config.Providers = make(map[string]ProviderConfig)
		for provider, defaultProviderConfig := range defaultConfig.Providers {
			if _, exists := config.Providers[provider]; !exists {
				config.Providers[provider] = defaultProviderConfig
			}
		}
	}

	// process providers
	for provider, providerConfig := range config.Providers {
		if !providerConfig.IsEnabled() {
			delete(config.Providers, provider)
		}

		if strings.HasPrefix(providerConfig.BaseURL, geminiBaseURL) {
			providerConfig.IsGemini = true
			config.Providers[provider] = providerConfig
		}
	}

	return config, err
}

func findConfigFile(cliConfigFile string) string {
	// if configfile provided via CLI argument, use it
	if cliConfigFile != "" {
		return cliConfigFile
	}

	// check env variable
	configFile := os.Getenv("LLMSEE_CONFIGFILE")
	if configFile != "" {
		return configFile
	}

	// check default OS paths
	switch runtime.GOOS {
	case "darwin":
		// macOS uses ~/Library/Application Support for application data
		userLibrary := os.Getenv("HOME") + "/Library/Application Support"
		configFile := filepath.Join(userLibrary, defaultConfigFile)
		if fileExists(configFile) {
			return configFile
		}

		// Check HOME/.config (fallback for Linux/macOS)
		if home := os.Getenv("HOME"); home != "" {
			configFile := filepath.Join(home, ".config", defaultConfigFile)
			if fileExists(configFile) {
				return configFile
			}
		}

	case "linux":
		// Check XDG_CONFIG_HOME for Linux
		if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
			configFile := filepath.Join(xdgConfigHome, defaultConfigFile)
			if fileExists(configFile) {
				return configFile
			}
		}

		// Check HOME/.config (fallback for Linux/macOS)
		if home := os.Getenv("HOME"); home != "" {
			configFile := filepath.Join(home, ".config", defaultConfigFile)
			if fileExists(configFile) {
				return configFile
			}
		}

	case "windows":
		// Check APPDATA for Windows
		if appData := os.Getenv("APPDATA"); appData != "" {
			configFile := filepath.Join(appData, defaultConfigFile)
			if fileExists(configFile) {
				return configFile
			}
		}

		// Check HOME (Fallback for Windows)
		if home := os.Getenv("USERPROFILE"); home != "" {
			configFile := filepath.Join(home, defaultConfigFile)
			if fileExists(configFile) {
				return configFile
			}
		}
	}

	// Default if no config file found
	return ""
}

// fileExists checks if a given file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func getDatabaseFile() string {
	var dbFile string

	// check env variable
	dbFile = os.Getenv("LLMSEE_DATABASEFILE")
	if dbFile != "" {
		return dbFile
	}

	// check default OS paths
	switch runtime.GOOS {
	case "darwin": // macOS
		// macOS uses ~/Library/Application Support for application data
		userLibrary := os.Getenv("HOME") + "/Library/Application Support"
		dbFile = filepath.Join(userLibrary, "llmsee", defaultConfig.DatabaseFile)

	case "linux": // Linux
		// Linux uses ~/.local/share for user-specific data
		userLocal := os.Getenv("HOME") + "/.local/share"
		dbFile = filepath.Join(userLocal, "llmsee", defaultConfig.DatabaseFile)

	case "windows": // Windows
		// Windows uses %APPDATA% for application data
		appData := os.Getenv("APPDATA")
		dbFile = filepath.Join(appData, "llmsee", defaultConfig.DatabaseFile)

	default:
		currentDir, err := os.Getwd()
		if err != nil {
			log.Fatalf("failed to get current working directory: %v", err)
		}
		dbFile = filepath.Join(currentDir, defaultConfig.DatabaseFile)
	}

	// Ensure the directory exists
	dir := filepath.Dir(dbFile)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		log.Fatalf("failed to create directory: %v", err)
	}

	return dbFile
}
