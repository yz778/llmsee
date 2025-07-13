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

func getConfig() (config *Config, err error) {
	config = &Config{}

	// Parse CLI arguments first
	cliArgs := parseCLIArgs()

	// Find and load config file
	configFile := findConfigFile(cliArgs.ConfigFile)

	if configFile != "" {
		fileConfig, _ := os.ReadFile(configFile)
		if err := yaml.Unmarshal(fileConfig, &config); err != nil {
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
	}

	// Ensure default providers exist if not configured
	for provider, defaultProviderConfig := range defaultConfig.Providers {
		if _, exists := config.Providers[provider]; !exists {
			config.Providers[provider] = defaultProviderConfig
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
