# Configuration Guide

## Configuration File Locations

LLMSee looks for configuration files in the following order:

1. CLI argument: `-c /path/to/config.json`
2. Environment variable: `LLMSEE_CONFIGFILE`
3. Platform-specific default locations:
   - **Linux**: `$XDG_CONFIG_HOME/llmsee.json` or `~/.config/llmsee.json`
   - **macOS**: `~/Library/Application Support/llmsee.json`
   - **Windows**: `%APPDATA%/llmsee.json`

## Basic Configuration

```json
{
  "host": "localhost",
  "port": 5050,
  "pageSize": 15,
  "databasefile": "/path/to/custom/database.db",
  "providers": {
    "provider_name": {
      "baseUrl": "https://api.provider.com/v1",
      "apikey": "your-api-key"
    }
  }
}
```

## Provider Configuration

### Basic Provider Setup

```json
"providers": {
  "openai": {
    "baseUrl": "https://api.openai.com/v1",
    "apikey": "sk-your-openai-key"
  },
  "ollama": {
    "baseUrl": "http://localhost:11434/v1",
    "apikey": ""
  }
}
```

### Advanced Provider Options

```json
"providers": {
  "custom_provider": {
    "baseUrl": "https://api.custom.com/v1",
    "apikey": "your-key",
    "enabled": true,
    "headerMapping": {
      "Authorization": "X-API-Key",
      "User-Agent": "X-Client-Name"
    },
    "models": [
      "simple-model",
      {
        "name": "enhanced-model",
        "think": true,
        "temperature": 0.7,
        "max_tokens": 2000
      }
    ]
  }
}
```

## Enhanced Model Configuration

LLMSee supports two model definition formats:

### Simple Format (Legacy)
```json
"models": ["model1", "model2", "model3"]
```

### Enhanced Format (Recommended)
```json
"models": [
  "simple-model",
  {
    "name": "thinking-model",
    "think": true,
    "temperature": 0.7
  },
  {
    "name": "creative-model",
    "think": false,
    "temperature": 1.2,
    "top_p": 0.9,
    "max_tokens": 4000
  }
]
```

### How Enhanced Models Work

When you make a request to an enhanced model, LLMSee automatically injects the configured parameters into your request body:

**Your Request:**
```json
{
  "model": "thinking-model",
  "messages": [{"role": "user", "content": "Hello"}]
}
```

**Actual Request Sent:**
```json
{
  "model": "thinking-model", 
  "messages": [{"role": "user", "content": "Hello"}],
  "think": true,
  "temperature": 0.7
}
```

## Environment Variables

Override configuration with environment variables:

- `LLMSEE_HOST` - Server host (default: localhost)
- `LLMSEE_PORT` - Server port (default: 5050)
- `LLMSEE_CONFIGFILE` - Path to config file
- `LLMSEE_DATABASEFILE` - Path to database file
- `LLMSEE_LOCALHOST` - Localhost override for Docker

## Database Configuration

### Default Database Locations

- **Linux**: `~/.local/share/llmsee/llmsee.db`
- **macOS**: `~/Library/Application Support/llmsee/llmsee.db`
- **Windows**: `%APPDATA%/llmsee/llmsee.db`

### Custom Database Location

```json
{
  "databasefile": "/custom/path/to/llmsee.db"
}
```

Or via environment variable:
```bash
export LLMSEE_DATABASEFILE="/custom/path/to/llmsee.db"
```

## Provider Examples

### OpenAI
```json
"openai": {
  "baseUrl": "https://api.openai.com/v1",
  "apikey": "sk-your-openai-api-key"
}
```

### Ollama (Local)
```json
"ollama": {
  "baseUrl": "http://localhost:11434/v1",
  "apikey": ""
}
```

### Anthropic Claude
```json
"anthropic": {
  "baseUrl": "https://api.anthropic.com/v1",
  "apikey": "your-anthropic-key",
  "headerMapping": {
    "Authorization": "x-api-key"
  }
}
```

### Google Gemini
```json
"gemini": {
  "baseUrl": "https://generativelanguage.googleapis.com/v1beta/openai",
  "apikey": "your-gemini-api-key"
}
```

### Groq
```json
"groq": {
  "baseUrl": "https://api.groq.com/openai/v1",
  "apikey": "your-groq-api-key"
}
```

### OpenRouter
```json
"openrouter": {
  "baseUrl": "https://openrouter.ai/api/v1",
  "apikey": "your-openrouter-key"
}
```

## Configuration Precedence

Settings are applied in this order (highest to lowest priority):

1. **CLI Arguments** (`-p 8080`, `-c config.json`)
2. **Configuration File** (`llmsee.json`)
3. **Environment Variables** (`LLMSEE_*`)
4. **Default Values**

## Validation and Troubleshooting

### Check Configuration Loading
LLMSee logs which configuration file it loads:
```
Config file /home/user/.config/llmsee.json
```

### Validate JSON Syntax
Use a JSON validator or:
```bash
cat ~/.config/llmsee.json | jq .
```

### Test Provider Connectivity
Check if your provider URLs are accessible:
```bash
curl -H "Authorization: Bearer your-key" https://api.openai.com/v1/models
```

### Debug Mode
Set `APP_ENV=dev` for additional debug logging.