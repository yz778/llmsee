package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// System configuration
const (
	defaultConfigFile         = "llmsee.yml"
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
	Host         string                    `yaml:"host"`
	Port         int                       `yaml:"port"`
	DatabaseFile string                    `yaml:"databasefile"`
	PageSize     int                       `yaml:"pagesize"`
	Providers    map[string]ProviderConfig `yaml:"providers"`
}

type ProviderConfig struct {
	BaseURL       string                            `yaml:"baseurl"`
	ApiKey        string                            `yaml:"apikey"`
	HeaderMapping map[string]string                 `yaml:"headermapping"`
	Models        *[]string                         `yaml:"models"`
	ModelParams   map[string]map[string]interface{} `yaml:"modelparams"`
	Enabled       *bool                             `yaml:"enabled"`
	IsGemini      bool                              `yaml:"-"`
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
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
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
	var cliArgs CLIArgs

	// Use a separate flag set to avoid conflicts with test flags
	fs := flag.NewFlagSet("llmsee", flag.ContinueOnError)
	fs.StringVar(&cliArgs.ConfigFile, "c", "", "Path to YAML configuration file")
	fs.IntVar(&cliArgs.Port, "p", 0, "Port number to override config file setting")

	// Parse using os.Args[1:] to skip the program name
	if len(os.Args) > 1 {
		fs.Parse(os.Args[1:])
	}

	return cliArgs
}

func getConfig(configPath string) (config *Config, err error) {
	config = &Config{}

	if configPath != "" {
		fileConfig, _ := os.ReadFile(configPath)
		if err = yaml.Unmarshal(fileConfig, &config); err != nil {
			log.Printf("Config file %s is not valid, skipping", configPath)
			config = &Config{}
		} else {
			log.Printf("Config file %s", configPath)
		}
	}

	// Apply defaults if not set by config file
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

	// if there are no providers, assign the defaults
	if config.Providers == nil {
		config.Providers = make(map[string]ProviderConfig)
	}

	// Ensure default providers exist if not configured
	for provider, initialProviderConfig := range defaultConfig.Providers {
		if _, exists := config.Providers[provider]; !exists {
			config.Providers[provider] = initialProviderConfig
		}
		// Special handling for pre-configured Gemini API
		if strings.HasPrefix(initialProviderConfig.BaseURL, geminiBaseURL) {
			p := config.Providers[provider]
			p.IsGemini = true
			config.Providers[provider] = p
		}
	}

	// Iterate over the providers again to handle enabled/disabled logic.
	// This separate loop is necessary because initial merging of defaults needs to happen first.
	for providerName, providerConfig := range config.Providers {
		if !providerConfig.IsEnabled() {
			log.Printf("Provider %s is disabled and will be removed.", providerName)
			delete(config.Providers, providerName)
		}
	}

	return config, err
}

func findConfigFile(cliConfigFile string) string {
	// CLI argument
	if cliConfigFile != "" {
		return cliConfigFile
	}

	// Environment variable
	if configFile := os.Getenv("LLMSEE_CONFIGFILE"); configFile != "" {
		return configFile
	}

	// Standard locations
	homeDir, _ := os.UserHomeDir()
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	appData := os.Getenv("APPDATA")

	pathsToCheck := []string{
		"llmsee.yml", // Current directory
	}
	if homeDir != "" {
		pathsToCheck = append(pathsToCheck,
			filepath.Join(homeDir, "llmsee.yml"),
			filepath.Join(homeDir, ".config", "llmsee", "llmsee.yml"),
			filepath.Join(homeDir, "Library", "Application Support", "llmsee", "llmsee.yml"),
		)
	}
	if xdgConfigHome != "" {
		pathsToCheck = append(pathsToCheck,
			filepath.Join(xdgConfigHome, "llmsee", "llmsee.yml"),
		)
	}
	if appData != "" {
		pathsToCheck = append(pathsToCheck,
			filepath.Join(appData, "llmsee", "llmsee.yml"),
		)
	}

	for _, path := range pathsToCheck {
		if fileExists(path) {
			return path
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
