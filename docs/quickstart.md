# Quick Start Guide

## Installation

### Option 1: Download Binary
Download the latest release from GitHub and run directly.

### Option 2: Build from Source
```bash
git clone https://github.com/yz778/llmsee.git
cd llmsee
make build
```

### Option 3: Docker
```bash
docker run -p 5050:5050 -v $(pwd)/data:/data llmsee
```

## Basic Setup

1. **Create Configuration File**
   
   Create `~/.config/llmsee.yml`:
   ```yaml
   host: localhost
   port: 5050
   providers:
     ollama:
       baseurl: http://localhost:11434/v1
       apikey: ""
   ```

2. **Start LLMSee**
   ```bash
   ./dist/llmsee
   ```

3. **Access Web UI**
   
   Open http://localhost:5050/ui/ in your browser

4. **Configure Your Application**
   
   Point your LLM application to:
   - **API Base URL**: `http://localhost:5050/v1`
   - **API Key**: `none` (or any value)

5. **Start Making Requests**
   
   Your requests will now be logged and visible in the web interface.

## Command Line Options

```bash
llmsee -c /path/to/config.yml     # Custom config file
llmsee -p 8080                    # Override port
llmsee -c config.yml -p 9090      # Both options
```

## Next Steps

- See [Configuration Guide](configuration.md) for advanced setup
- See [Developer Guide - Backend](developer-guide-backend.md) for code contributions
- See [Testing Guide](testing.md) for running tests