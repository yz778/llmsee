package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"
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

// Configuration structs
type Config struct {
	Host         string                    `json:"host"`
	Port         int                       `json:"port"`
	DatabaseFile string                    `json:"databasefile"`
	PageSize     int                       `json:"pagesize"`
	Providers    map[string]ProviderConfig `json:"providers"`
}

type ProviderConfig struct {
	BaseURL       string            `json:"baseurl"`
	ApiKey        string            `json:"apikey"`
	HeaderMapping map[string]string `json:"headermapping"`
}

var defaultConfig = Config{
	Host:         "localhost",
	Port:         5050,
	DatabaseFile: "llmsee.db",
	PageSize:     20,
	Providers: map[string]ProviderConfig{
		"ollama": {
			BaseURL:       "http://localhost:11434/v1",
			ApiKey:        "",
			HeaderMapping: map[string]string{},
		},
	},
}

func getConfig() (config *Config, err error) {
	config = &Config{}
	configFile := findConfigFile()

	if configFile == "" {
		log.Printf("Using default configuration\n")
	} else {
		log.Printf("%-10s %s\n", "Config:", configFile)
		fileConfig, _ := os.ReadFile(configFile)
		if err := json.Unmarshal(fileConfig, &config); err != nil {
			log.Fatalf("Invalid configuration in %s\n", configFile)
			config = &Config{}
		}
	}

	if config.Host == "" {
		config.Host = defaultConfig.Host
	}

	if config.Port == 0 {
		config.Port = defaultConfig.Port
	}

	if config.DatabaseFile == "" {
		config.DatabaseFile = getDatabaseFile()
	}

	if config.PageSize == 0 {
		config.PageSize = defaultConfig.PageSize
	}

	if config.Providers == nil {
		config.Providers = make(map[string]ProviderConfig)
	}

	for providerName, defaultProviderConfig := range defaultConfig.Providers {
		if _, exists := config.Providers[providerName]; !exists {
			config.Providers[providerName] = defaultProviderConfig
		}
	}

	return config, err
}

func findConfigFile() string {
	// if configfile in arguments (-c <configfile>), it must exist
	var configFile string
	flag.StringVar(&configFile, "c", "", "Path to JSON configuration file")
	flag.Parse()
	if configFile != "" {
		if fileExists(configFile) {
			return configFile
		} else {
			log.Fatalf("Configuration file %s not found", configFile)
		}
	}

	// check env variable
	configFile = os.Getenv("LLMSEE_CONFIGFILE")
	if fileExists(configFile) {
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
	if fileExists(dbFile) {
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
