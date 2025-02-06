# LLMSee

LLMSee is a cross-platform lightweight proxy for logging OpenAI-compatible API calls.

## Features

- **Platform Compatibility**: A single executable that runs on Linux, MacOS, Windows, or via Docker.
- **Built-in Web Interface**: Offers simple web UI to monitor logged requests in real time.
- **Supports Multiple Providers**: Supports transparent proxying to multiple LLM providers.

## Quick Start

1. Create a `~/.config/llmsee.json` [configuration](#configuration) file with your provider details.

2. Run `llmsee` to start the server.

3. Access the LLMSee Web UI at: http://localhost:5050/ui/.

4. Configure your application:
* `API_BASE_URL=http://localhost:5050/v1`
* `OPENAPI_API_KEY=none`

6. Start using your app and see real-time updates in the LLMSee Web UI

## Configuration

The configuration file should be set up as follows:

```json
{
	"host": "localhost",
	"port": 5050,
	"pageSize": 15,
	"providers": {
		"ollama": {
			"baseUrl": "http://localhost:11434/v1",
			"apikey": ""
		},
		"openai": {
			"baseUrl": "https://api.openai.com/v1",
			"apikey": "<OpenAI API Key>"
		},
		"deepseek": {
			"baseUrl": "https://api.deepseek.com",
			"apikey": "<Deepseek API Key>"
		},
		"gemini": {
			"baseUrl": "https://generativelanguage.googleapis.com/v1beta/openai",
			"apikey": "<Gemini API Key>"
		},
		"groq": {
			"baseUrl": "https://api.groq.com/openai/v1",
			"apikey": "<Groq API Key>"
		},
		"openrouter": {
			"baseUrl": "https://openrouter.ai/api/v1",
			"apikey": "<openrouter API key>"
		},
		"provider...": ...etc...
	}
}
```

## Building

To build the project, you will need Go installed:
```sh
make
```

To build the Docker image, run this command:
```sh
make docker
```

## TODOs

- [ ] Update documentation
- [ ] Clean up and refactor (cmd/, internal/)
- [ ] Add search
- [ ] Add database auto-trimming (max N records)

## Contributing

Contributions are welcome! Please fork the repository and submit a pull request.

## License

LLMSee is made by [@yz778](https://github.com/yz778) and licensed under the MIT License. See [LICENSE](LICENSE) for more details.
