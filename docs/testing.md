# Testing Guide

## Running Tests

### Quick Commands
```bash
make test           # Run all tests
make test-coverage  # Generate coverage report (coverage.html)
make test-unit      # Unit tests only
make test-integration # Integration tests only
make test-short     # Quick test run
```

### Manual Test Commands
```bash
go test -v ./...                    # All tests with verbose output
go test -v -run TestSpecificTest    # Run specific test
go test -v -short ./...             # Skip long-running tests
```

## Test Structure

### Test Files Organization
- `config_test.go` - Configuration parsing and CLI argument tests
- `handler_proxy_test.go` - Proxy functionality and model parameter tests
- `database_test.go` - Database operations and logging tests
- `integration_test.go` - End-to-end integration tests

### Test Categories

#### 1. Configuration Tests (`config_test.go`)
- **`TestModelConfig_UnmarshalJSON`** - String vs object model parsing
- **`TestModelConfig_MarshalJSON`** - JSON serialization
- **`TestProviderConfig_IsEnabled`** - Provider enable/disable logic
- **`TestConfigParsing`** - Complete config file parsing
- **`TestDefaultConfig`** - Default configuration behavior
- **`TestCLIPortOverride`** - CLI port override functionality
- **`TestParseCLIArgs`** - CLI argument parsing

#### 2. Proxy Handler Tests (`handler_proxy_test.go`)
- **`TestParseProvider`** - URL path parsing
- **`TestProxyHeaders`** - Header manipulation and API key injection
- **`TestGetModels_CustomModels`** - Model listing with enhanced parameters
- **`TestWriteModels`** - Model endpoint response generation
- **`TestModelParameterApplication`** - Automatic parameter injection
- **`TestHandleCORS`** - CORS header handling
- **`TestDecompressBody`** - Response decompression

#### 3. Database Tests (`database_test.go`)
- **`TestGetDb`** - Database initialization and schema creation
- **`TestLogEntryOperations`** - CRUD operations for log entries
- **`TestLogEntriesPagination`** - Pagination of log results
- **`TestGetLogDetail_NotFound`** - Error handling for missing records
- **`TestDatabaseSchema`** - Database schema integrity

#### 4. Integration Tests (`integration_test.go`)
- **`TestIntegration_ModelParameterApplication`** - End-to-end parameter injection
- **`TestIntegration_V1ModelsEndpoint`** - `/v1/models` endpoint testing
- **`TestIntegration_ProviderModelsEndpoint`** - Provider-specific model endpoints
- **`TestIntegration_BackwardCompatibility`** - Legacy config compatibility
- **`TestIntegration_RequestLogging`** - Request logging to database

## Key Test Scenarios

### Enhanced Model Configuration Testing

#### Backward Compatibility
```go
// Legacy format still works
"models": ["model1", "model2"]
```

#### Enhanced Format
```go
// New object format with parameters
"models": [
  "simple-model",
  {
    "name": "thinking-model",
    "think": true,
    "temperature": 0.7
  }
]
```

#### Parameter Application Verification
Tests verify that when a request is made to `thinking-model`, the parameters `think: true` and `temperature: 0.7` are automatically added to the request body.

### CLI Argument Testing
- Port override functionality (`-p` flag)
- Config file specification (`-c` flag)
- Precedence order validation

## Test Data Management

### Isolation Strategies
- Tests use `t.TempDir()` for temporary directories and files
- Database tests use isolated SQLite files
- Configuration tests use temporary config files
- Integration tests use mock HTTP servers

### Mock Servers
Integration tests create mock upstream servers to simulate provider responses without external dependencies.

## Coverage Reporting

### Generate Coverage Report
```bash
make test-coverage
```

This creates:
- `coverage.out` - Raw coverage data
- `coverage.html` - HTML coverage report

### View Coverage
```bash
open coverage.html  # macOS
xdg-open coverage.html  # Linux
```

## Writing New Tests

### Test File Naming
- Place test files in same directory as source code
- Use `*_test.go` naming convention
- Group related tests in same file

### Test Function Naming
```go
func TestFeatureName_Scenario(t *testing.T) {
    // Test implementation
}
```

### Table-Driven Tests
```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "valid input",
            input:    "test",
            expected: "result",
            wantErr:  false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test logic
        })
    }
}
```

### Test Helpers
```go
// Helper functions for common test setup
func setupTestConfig(t *testing.T) *Config {
    // Setup logic
}

func createTestServer(t *testing.T) *ProxyServer {
    // Server creation logic
}
```

## Debugging Tests

### Verbose Output
```bash
go test -v ./...
```

### Run Specific Test
```bash
go test -v -run TestModelConfig_UnmarshalJSON
```

### Debug Logging in Tests
```go
func TestSomething(t *testing.T) {
    t.Logf("Debug info: %v", someValue)
    // Test logic
}
```

### Check Temporary Files
Tests create temporary files that can be inspected for debugging:
```go
tmpDir := t.TempDir()
t.Logf("Temp directory: %s", tmpDir)
```

## Continuous Integration

### CI-Friendly Features
- No external dependencies (uses embedded SQLite)
- Deterministic tests (no random behavior)
- Automatic cleanup of temporary resources
- Parallel-safe tests (where appropriate)

### Environment Setup
Tests work in CI environments without special setup:
```bash
go test ./...  # Works in any CI system
```

## Performance Considerations

### Fast Test Execution
- Database tests create fresh databases for isolation
- Integration tests use mock servers (no network calls)
- Most tests complete in milliseconds
- Use `make test-short` for faster feedback during development

### Resource Management
- Tests automatically clean up temporary files
- Database connections are properly closed
- Mock servers are shut down after tests

## Best Practices

### Test Organization
1. **Unit Tests**: Test individual functions and components
2. **Integration Tests**: Test end-to-end request flows
3. **Mock Dependencies**: Use mock HTTP servers and temporary files
4. **Test Both Paths**: Happy path and error conditions
5. **Maintain Coverage**: Aim for high test coverage

### Test Maintenance
1. **Keep Tests Simple**: Each test should verify one thing
2. **Use Descriptive Names**: Test names should explain what they verify
3. **Clean Up Resources**: Ensure tests don't leave artifacts
4. **Update Tests**: When changing code, update corresponding tests
5. **Document Complex Tests**: Add comments for complex test scenarios