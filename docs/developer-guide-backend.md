# Backend Developer Guide

## Getting Started

### Prerequisites
- Go 1.24.2 or later
- Make (for build automation)
- SQLite (embedded, no separate installation needed)

### Development Setup
```bash
git clone https://github.com/yz778/llmsee.git
cd llmsee
make build
```

### Development Mode
```bash
APP_ENV=dev ./dist/llmsee
```
This serves UI files from the filesystem instead of embedded assets, enabling live UI development.

## Code Architecture

### Project Structure
```
├── main.go                 # Server initialization, routing, graceful shutdown
├── config.go              # Configuration management, CLI parsing
├── database.go            # SQLite operations, data models
├── handler_proxy.go       # Main proxy logic, provider routing
├── handler_ui.go          # Web interface serving
├── handler_log.go         # Log data API endpoints
├── handler_sse.go         # Server-Sent Events implementation
└── *_test.go              # Co-located test files
```

### Core Components

#### 1. Server Initialization (`main.go`)
```go
type ProxyServer struct {
    devMode        bool
    config         Config
    db             *sql.DB
    mu             sync.RWMutex
    client         *http.Client
    clientChannels map[string]*SSEClient
}
```

**Key Responsibilities:**
- HTTP server setup and routing
- Graceful shutdown handling
- SSE client management
- Development mode detection

#### 2. Configuration System (`config.go`)
```go
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
    Models        *[]string                         `json:"models"`
    ModelParams   map[string]map[string]interface{} `json:"modelparams"`
    Enabled       *bool             `json:"enabled"`
    IsGemini      bool              `json:"-"`
}
```

**Key Features:**
- Cross-platform config file discovery
- CLI argument parsing with precedence
- Enhanced model configuration support
- Environment variable overrides

#### 3. Database Layer (`database.go`)
```go
type LogEntry struct {
    Id                int    `json:"id"`
    Timestamp         string `json:"timestamp"`
    Provider          string `json:"provider"`
    Model             string `json:"model"`
    // ... other fields
}
```

**Key Operations:**
- `insertLogRequest()` - Create new log entry
- `updateLogRequest()` - Update with response data
- `getLogEntries()` - Paginated retrieval
- `getLogDetail()` - Single entry with full data

#### 4. Proxy Handler (`handler_proxy.go`)
**Key Functions:**
- `handleProxy()` - Main request routing
- `parseProvider()` - Extract provider from URL
- `proxyHeaders()` - Header manipulation
- `getModels()` - Model listing with enhancements

## Development Patterns

### Error Handling
```go
// Consistent error wrapping with context
if err != nil {
    return fmt.Errorf("failed to process request: %w", err)
}

// HTTP error responses
http.Error(w, `{"error":"descriptive message"}`, http.StatusBadRequest)
```

### Logging
```go
// Use log.Printf for important events
log.Printf("Config file %s", configFile)
log.Printf("Port overridden by CLI argument: %d", port)

// Use log.Printf for errors
log.Printf("failed to update request log: %v", err)
```

### Concurrency Patterns
```go
// SSE client management with proper cleanup
server.mu.Lock()
server.clientChannels[clientID] = client
server.mu.Unlock()

// Buffered channels to prevent blocking
eventChan := make(chan []byte, 100)

// Context cancellation for request timeouts
ctx, cancel := context.WithTimeout(r.Context(), httpRequestTimeout)
defer cancel()
```

### Configuration Precedence
1. CLI arguments (`-p`, `-c`)
2. Configuration file
3. Environment variables
4. Default values

## Adding New Features

### Adding a New Provider
1. **Update Configuration** - Add provider-specific handling in `config.go`
2. **Handle Special Cases** - Add any provider-specific logic in `handler_proxy.go`
3. **Test Integration** - Add tests in `integration_test.go`

Example:
```go
// In config.go - detect special provider
if strings.HasPrefix(providerConfig.BaseURL, specialProviderURL) {
    providerConfig.IsSpecial = true
    config.Providers[provider] = providerConfig
}

// In handler_proxy.go - handle special behavior
if providerConfig.IsSpecial {
    // Special handling logic
    delete(bodyJSON, "unsupported_param")
}
```

### Adding New CLI Arguments
1. **Update CLIArgs struct** in `config.go`
2. **Add flag parsing** in `parseCLIArgs()`
3. **Apply in getConfig()** with proper precedence
4. **Add tests** in `config_test.go`

Example:
```go
type CLIArgs struct {
    ConfigFile string
    Port       int
    NewOption  string  // Add new option
}

func parseCLIArgs() CLIArgs {
    var args CLIArgs
    flag.StringVar(&args.ConfigFile, "c", "", "Config file path")
    flag.IntVar(&args.Port, "p", 0, "Port override")
    flag.StringVar(&args.NewOption, "n", "", "New option")  // Add flag
    flag.Parse()
    return args
}
```

### Adding New API Endpoints
1. **Create handler function** following naming pattern
2. **Add route** in `main.go`
3. **Implement logic** with proper error handling
4. **Add tests** in appropriate test file

Example:
```go
// In handler_*.go
func (s *ProxyServer) handleNewEndpoint(w http.ResponseWriter, r *http.Request) {
    // Handle CORS
    if s.handleCORS(w, r) {
        return
    }
    
    // Implementation logic
    w.Header().Set("Content-Type", "application/json")
    // ... response logic
}

// In main.go
mux.HandleFunc("/new-endpoint", server.handleNewEndpoint)
```

## Testing Guidelines

### Test Organization
- Keep test files co-located with source files
- Use table-driven tests for comprehensive coverage
- Mock external dependencies (HTTP servers, file systems)
- Test both happy path and error conditions

### Test Categories
1. **Unit Tests** - Individual functions and components
2. **Integration Tests** - End-to-end request flows
3. **Configuration Tests** - Config parsing and CLI args
4. **Database Tests** - Data persistence and retrieval

### Writing Tests
```go
func TestNewFeature(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "valid case",
            input:    "test input",
            expected: "expected output",
            wantErr:  false,
        },
        // More test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := functionUnderTest(tt.input)
            
            if tt.wantErr && err == nil {
                t.Error("expected error but got none")
            }
            if !tt.wantErr && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            if result != tt.expected {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

## Performance Considerations

### Constants and Limits
```go
// From config.go - adjust these for your use case
const (
    httpRequestTimeout        = 1 * time.Hour    // Long timeout for LLM requests
    httpMaxRequestBodySize    = 10 << 20         // 10MB max request body
    dbMaxOpenConns           = 5                 // Conservative DB connection pool
    clientMaxIdleConns       = 100               // HTTP client connection pool
)
```

### Optimization Strategies
- **Streaming**: Large responses are streamed, not buffered
- **Connection Pooling**: Both HTTP client and database connections
- **Buffered Channels**: SSE events use buffered channels to prevent blocking
- **Context Cancellation**: Proper timeout handling for all requests

### Memory Management
- Request/response bodies are handled efficiently
- Database connections are pooled and reused
- SSE clients are properly cleaned up on disconnect
- Temporary files in tests are automatically cleaned up

## Debugging

### Development Mode
```bash
APP_ENV=dev ./llmsee
```
- Serves UI from filesystem for live development
- Additional debug logging
- Easier to test UI changes

### Logging Levels
```go
// Important configuration events
log.Printf("Config file %s", configFile)

// Error conditions
log.Printf("failed to process request: %v", err)

// Debug information (in dev mode)
if devMode {
    log.Printf("Debug: %v", debugInfo)
}
```

### Common Debugging Techniques
1. **Check Configuration Loading** - Look for config file log messages
2. **Verify Provider Connectivity** - Test upstream URLs manually
3. **Inspect Database** - Use SQLite CLI to examine logged requests
4. **Monitor SSE Events** - Check browser dev tools for real-time updates
5. **Test with curl** - Verify proxy behavior with direct HTTP calls

## Code Style and Conventions

### Go Best Practices
- Use `gofmt` for consistent formatting
- Follow effective Go guidelines
- Use meaningful variable and function names
- Keep functions focused and small
- Handle errors explicitly

### Project-Specific Conventions
- Co-locate test files with source code
- Use table-driven tests where appropriate
- Maintain backward compatibility for configuration
- Log important events and errors
- Use proper HTTP status codes
- Clean up resources in defer statements

### Documentation
- Add comments for exported functions
- Document complex logic and algorithms
- Keep README and docs up to date
- Include examples in documentation
- Document breaking changes