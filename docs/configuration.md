# Configuration Guide

## Configuration File Locations

LLMSee looks for configuration files in the following order:

1. CLI argument: `-c /path/to/config.yml`
2. Environment variable: `LLMSEE_CONFIGFILE`
3. Platform-specific default locations:
   - **Linux**: `$XDG_CONFIG_HOME/llmsee.yml` or `~/.config/llmsee.yml`
   - **macOS**: `~/Library/Application Support/llmsee.yml`
   - **Windows**: `%APPDATA%/llmsee.yml`

## Full Configuration Example

```yaml
# LLMSee Configuration File (YAML format)
# This is an example configuration file showing all available options

host: localhost
port: 5050
databasefile: llmsee.db
pagesize: 20

providers:
  # Ollama (local LLM server)
  ollama:
    baseurl: http://localhost:11434/v1
    apikey: ""
    headermapping: {}
    enabled: true

  # OpenAI
  openai:
    baseurl: https://api.openai.com/v1
    apikey: sk-your-openai-api-key-here
    headermapping: {}
    models:
      - gpt-4
      - gpt-3.5-turbo
      - gpt-4-turbo
    modelparams:
      gpt-4:
        temperature: 0.7
        max_tokens: 2000
      gpt-3.5-turbo:
        temperature: 0.9
        max_tokens: 1500
    enabled: true

  # Anthropic Claude
  anthropic:
    baseurl: https://api.anthropic.com/v1
    apikey: your-anthropic-api-key-here
    headermapping:
      Authorization: Bearer
    models:
      - claude-3-opus-20240229
      - claude-3-sonnet-20240229
    modelparams:
      claude-3-opus-20240229:
        max_tokens: 4000
        temperature: 0.7
    enabled: true

  # Custom provider example
  custom_provider:
    baseurl: https://api.custom.com/v1
    apikey: your-custom-api-key
    enabled: false
    headermapping:
      Authorization: X-API-Key
      User-Agent: X-Client-Name
    models:
      - custom-model-1
      - custom-model-2
    modelparams:
      custom-model-2:
        think: true
        temperature: 0.8
        top_p: 0.9
```

## Validation and Troubleshooting

### Check Configuration Loading
LLMSee logs which configuration file it loads:
```
Config file /home/user/.config/llmsee.yml
```

### Validate YAML Syntax
Use a YAML validator or:
```bash
cat ~/.config/llmsee.yml | yq .
```

### Test Provider Connectivity
Check if your provider URLs are accessible:
```bash
curl -H "Authorization: Bearer your-key" https://api.openai.com/v1/models
```

### Debug Mode
Set `APP_ENV=dev` for additional debug logging.