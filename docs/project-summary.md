# Project Summary

## What is LLMSee?

LLMSee is a cross-platform lightweight proxy for logging OpenAI-compatible API calls. It sits between your application and LLM providers, transparently logging all requests and responses while providing real-time monitoring through a built-in web interface.

## Key Features

- **Transparent Proxying**: Works with any OpenAI-compatible API
- **Multiple Providers**: Support for OpenAI, Ollama, Anthropic, Gemini, Groq, and more
- **Real-time Monitoring**: Built-in web UI with live request updates
- **Enhanced Model Configuration**: Automatic parameter injection for models
- **Cross-platform**: Single binary for Linux, macOS, Windows, and Docker
- **No Dependencies**: Embedded SQLite database, no external services required

## Architecture Overview

### Core Components

- **Proxy Server** (`main.go`) - HTTP server with routing and graceful shutdown
- **Configuration System** (`config.go`) - Multi-source config with CLI overrides
- **Database Layer** (`database.go`) - SQLite operations and data models
- **Request Handlers** (`handler_*.go`) - Specialized handlers for different endpoints
- **Web UI** (`ui/`) - Real-time monitoring interface

### Technology Stack

- **Backend**: Go 1.24.2
- **Database**: SQLite (embedded via modernc.org/sqlite)
- **Frontend**: Vanilla HTML/CSS/JavaScript
- **Real-time Updates**: Server-Sent Events (SSE)
- **Build System**: Make + Docker
- **Testing**: Standard Go testing with comprehensive coverage

## Request Flow

1. **Client Request** → LLMSee proxy
2. **Provider Detection** → Parse URL path to identify provider
3. **Model Enhancement** → Apply model-specific parameters (if configured)
4. **Upstream Forwarding** → Send request to actual provider
5. **Response Streaming** → Stream response back to client
6. **Logging** → Store request/response in SQLite database
7. **Real-time Updates** → Broadcast to web UI via SSE

## Key Design Decisions

### Single Binary Architecture
- All components embedded in one executable
- No external dependencies or services required
- Embedded web UI assets for production
- Cross-platform compatibility

### SQLite for Persistence
- Embedded database, no external DB server
- Automatic schema creation and management
- Efficient storage of request/response data
- Simple backup and maintenance

### Server-Sent Events for Real-time
- Simpler than WebSockets for one-way updates
- Automatic reconnection handling
- Efficient for monitoring use case
- No bidirectional communication needed

### Handler-based Architecture
- Separate files for different HTTP endpoints
- Clear separation of concerns
- Easy to extend with new functionality
- Consistent error handling patterns

## Enhanced Model Configuration

### Basic Model Configuration
```json
"models": ["gpt-4", "gpt-3.5-turbo"]
```

### Models with Parameters
```json
"models": ["gpt-4", "gpt-4-thinking"],
"modelparams": {
  "gpt-4-thinking": {
    "think": true,
    "temperature": 0.7,
    "max_tokens": 2000
  }
}
```

### Automatic Parameter Injection
When a request is made to `gpt-4-thinking`, LLMSee automatically adds the configured parameters to the request body before forwarding to the provider.

## Provider Support

### Supported Providers
- **OpenAI** - GPT models
- **Ollama** - Local LLM hosting
- **Anthropic** - Claude models
- **Google Gemini** - Gemini models
- **Groq** - Fast inference
- **OpenRouter** - Multiple model access
- **Custom Providers** - Any OpenAI-compatible API

### Provider Features
- Dynamic routing based on URL paths
- Custom header mapping per provider
- API key injection and management
- Special handling for provider differences (e.g., Gemini)

## Development Workflow

### Building
```bash
make build          # Build binary
make docker         # Build Docker image
make clean          # Clean build artifacts
```

### Testing
```bash
make test           # Run all tests
make test-coverage  # Generate coverage report
make test-unit      # Unit tests only
make test-integration # Integration tests only
```

### Development Mode
```bash
APP_ENV=dev ./llmsee    # Serve UI from filesystem for live development
```

## File Organization

```
.
├── main.go                 # Server initialization and routing
├── config.go              # Configuration management
├── database.go            # SQLite operations
├── handler_proxy.go       # Main proxy logic
├── handler_ui.go          # Web interface serving
├── handler_log.go         # Log API endpoints
├── handler_sse.go         # Real-time updates
├── *_test.go              # Test files (co-located)
├── ui/                    # Web interface assets
├── docs/                  # Documentation
├── Makefile               # Build targets
├── Dockerfile             # Container build
└── go.mod                 # Go dependencies
```

## Performance Characteristics

### Optimizations
- Connection pooling for HTTP clients and database
- Streaming responses for large payloads
- Buffered channels for SSE events
- Compression support for responses
- Efficient SQLite queries with proper indexing

### Resource Usage
- Low memory footprint (single binary)
- Minimal CPU overhead (proxy pattern)
- Disk usage grows with logged requests
- Network overhead is minimal (transparent proxy)

## Future Roadmap

### Planned Features
- Search functionality for logged requests
- Database auto-trimming (configurable retention)
- Enhanced filtering and analytics
- Export capabilities for logged data

### Potential Refactoring
- Move to `cmd/` and `internal/` structure for larger codebase
- Separate handler packages
- Plugin system for custom providers
- Configuration validation and schema

## Use Cases

### Development and Debugging
- Monitor LLM API calls during development
- Debug request/response issues
- Analyze model performance and behavior
- Track API usage and costs

### Testing and QA
- Verify API integrations work correctly
- Test different model configurations
- Validate request/response formats
- Performance testing of LLM applications

### Production Monitoring
- Monitor production LLM usage
- Track API performance and reliability
- Analyze user interaction patterns
- Debug production issues

### Research and Analysis
- Collect data for model comparison
- Analyze prompt effectiveness
- Study response patterns
- Generate usage reports