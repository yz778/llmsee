# Database Guide

## Overview

LLMSee uses SQLite as an embedded database to store request/response logs. No external database server is required.

## Database Schema

### Main Table: `logs`

```sql
CREATE TABLE logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,
    provider TEXT NOT NULL,
    method TEXT NOT NULL,
    model TEXT NOT NULL,
    target_url TEXT NOT NULL,
    request_headers TEXT DEFAULT '',
    request_body TEXT DEFAULT '',
    response_status INT DEFAULT -1,
    response_headers TEXT DEFAULT '',
    response_body TEXT DEFAULT '',
    useragent TEXT DEFAULT '',
    duration_ms INTEGER DEFAULT -1
);

CREATE INDEX idx_timestamp ON logs(timestamp);
```

### Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| `id` | INTEGER | Auto-incrementing primary key |
| `timestamp` | TEXT | Request timestamp in RFC3339 format |
| `provider` | TEXT | Provider name (e.g., "openai", "ollama") |
| `method` | TEXT | HTTP method (GET, POST, etc.) |
| `model` | TEXT | Model name used in the request |
| `target_url` | TEXT | Full URL of the upstream provider |
| `request_headers` | TEXT | JSON-encoded request headers |
| `request_body` | TEXT | Raw request body |
| `response_status` | INT | HTTP response status code |
| `response_headers` | TEXT | JSON-encoded response headers |
| `response_body` | TEXT | Raw response body |
| `useragent` | TEXT | Client User-Agent header |
| `duration_ms` | INTEGER | Request duration in milliseconds |

## Database Locations

### Default Locations by Platform

- **Linux**: `~/.local/share/llmsee/llmsee.db`
- **macOS**: `~/Library/Application Support/llmsee/llmsee.db`
- **Windows**: `%APPDATA%/llmsee/llmsee.db`

### Custom Location

Set via configuration file:
```json
{
  "databasefile": "/custom/path/to/llmsee.db"
}
```

Or environment variable:
```bash
export LLMSEE_DATABASEFILE="/custom/path/to/llmsee.db"
```

## Database Operations

### Initialization

The database is automatically created on first run with proper schema and indexes.

### Connection Management

- **Max Open Connections**: 5 (conservative for SQLite)
- **Max Idle Connections**: 5
- **Connection Lifetime**: 1 minute
- **Connection Pooling**: Enabled for concurrent access

### Performance Tuning

#### Constants (from config.go)
```go
dbConnMaxLifetime = 1 * time.Minute
dbMaxIdleConns    = 5
dbMaxOpenConns    = 5
```

#### Indexes
- `idx_timestamp` - Optimizes queries by timestamp (most common sort)

## Data Models

### LogEntry Structure

```go
type LogEntry struct {
    Id                int    `json:"id"`
    Timestamp         string `json:"timestamp"`
    Provider          string `json:"provider"`
    Method            string `json:"method"`
    Model             string `json:"model"`
    TargetURL         string `json:"target_url"`
    RequestHeaders    string `json:"request_headers"`
    RequestBody       string `json:"request_body"`
    RequestBodySize   int    `json:"request_body_size"`
    ResponseStatus    int    `json:"response_status"`
    ResponseHeaders   string `json:"response_headers"`
    ResponseBody      string `json:"response_body"`
    ResponseBodySize  int    `json:"response_body_size"`
    UserAgent         string `json:"useragent"`
    DurationMs        int    `json:"duration_ms"`
}
```

### LogResponse Structure

```go
type LogResponse struct {
    Logs       []LogEntry `json:"logs"`
    TotalCount int        `json:"total_count"`
    Page       int        `json:"page"`
    PageSize   int        `json:"page_size"`
}
```

## API Operations

### Insert Log Request

```go
func (s *ProxyServer) insertLogRequest(entry LogEntry) (int64, error)
```

Creates a new log entry and returns the auto-generated ID.

### Update Log Request

```go
func (s *ProxyServer) updateLogRequest(entry LogEntry) error
```

Updates an existing log entry with response data.

### Get Log Entries (Paginated)

```go
func (s *ProxyServer) getLogEntries(page, pageSize int) ([]LogEntry, int, error)
```

Retrieves paginated log entries, ordered by timestamp (newest first).

### Get Log Detail

```go
func (s *ProxyServer) getLogDetail(id int64) (LogEntry, error)
```

Retrieves a single log entry by ID with full request/response bodies.

## Query Patterns

### Pagination Query
```sql
SELECT * FROM logs 
ORDER BY timestamp DESC 
LIMIT ? OFFSET ?
```

### Count Query
```sql
SELECT COUNT(*) FROM logs
```

### Detail Query
```sql
SELECT * FROM logs WHERE id = ?
```

## Data Storage Considerations

### Size Management

- **Request Bodies**: Stored as TEXT (up to 10MB per request)
- **Response Bodies**: Stored as TEXT (unlimited, but consider disk space)
- **Headers**: JSON-encoded strings
- **Compression**: Response bodies may be compressed by providers

### Disk Space

Log entries can grow large depending on:
- Request/response body sizes
- Number of requests
- Retention period

Consider implementing log rotation or cleanup for production use.

## Backup and Maintenance

### Backup Database

```bash
# Simple file copy (when LLMSee is stopped)
cp ~/.local/share/llmsee/llmsee.db backup.db

# Online backup using SQLite
sqlite3 ~/.local/share/llmsee/llmsee.db ".backup backup.db"
```

### Database Inspection

```bash
# Open database in SQLite CLI
sqlite3 ~/.local/share/llmsee/llmsee.db

# Common queries
.tables                           # List tables
.schema logs                      # Show table schema
SELECT COUNT(*) FROM logs;        # Count entries
SELECT * FROM logs LIMIT 5;      # Show recent entries
```

### Cleanup Old Entries

```sql
-- Delete entries older than 30 days
DELETE FROM logs 
WHERE timestamp < datetime('now', '-30 days');

-- Vacuum to reclaim space
VACUUM;
```

## Development Notes

### Testing Database Operations

Tests use temporary databases:
```go
tmpDir := t.TempDir()
dbFile := filepath.Join(tmpDir, "test.db")
db, err := getDb(dbFile)
```

### Database Migrations

Currently, schema changes require manual database recreation. Future versions may include migration support.

### Concurrent Access

SQLite handles concurrent reads well, but writes are serialized. The connection pool manages this automatically.

## Troubleshooting

### Common Issues

1. **Permission Errors**: Ensure LLMSee can write to the database directory
2. **Disk Space**: Monitor disk usage for large log volumes
3. **Corruption**: SQLite is robust, but always backup before manual edits

### Debug Database Issues

```bash
# Check database integrity
sqlite3 llmsee.db "PRAGMA integrity_check;"

# Check database size
ls -lh llmsee.db

# Check recent entries
sqlite3 llmsee.db "SELECT timestamp, provider, model FROM logs ORDER BY timestamp DESC LIMIT 10;"
```

### Performance Monitoring

Monitor these metrics:
- Database file size growth
- Query response times
- Connection pool utilization
- Disk I/O patterns