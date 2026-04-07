# Shroud

Use AI tools on real data without leaking secrets.

Shroud is a local launcher + proxy that masks sensitive values before they are sent to LLM APIs, then rehydrates placeholders in responses so your tool output remains readable.

## Purpose

Shroud helps you safely run AI coding tools against real logs, configs, and code by:

- masking secrets in outbound traffic (for supported HTTP flows)
- restoring placeholders in responses before they reach your tool
- showing a live dashboard (`--ui`) of what was masked
- writing local session logs with placeholder metadata (not raw secrets)

## Current Support

### Platforms

- macOS: `amd64`, `arm64`
- Linux: `amd64`, `arm64`

### AI tools

Known shorthands:

- `claude`
- `aider`
- `deepagents`

Also supported:

- any tool/binary that respects `OPENAI_BASE_URL` and/or `ANTHROPIC_BASE_URL`

### Providers

- OpenAI-compatible endpoints
- Anthropic-compatible endpoints
- custom upstream base URLs via env vars:
  - `OPENAI_BASE_URL`
  - `ANTHROPIC_BASE_URL`

### Transport behavior

- HTTP request/response flows: supported
- Streaming `text/event-stream` (SSE): supported
- `CONNECT` tunnels: proxied as raw tunnel; payloads inside TLS are not inspected or masked

## Install

### Option 1: Homebrew (recommended)

```bash
brew tap nikhil-mishra03/homebrew-tap
brew install shroud
```

### Option 2: Install script

```bash
curl -fsSL https://raw.githubusercontent.com/nikhil-mishra03/Shroud/main/install.sh | sh
```

### Option 3: Build from source

Requirements:

- Go 1.24+

```bash
git clone https://github.com/nikhil-mishra03/Shroud.git
cd Shroud
go build -o shroud ./cmd/shroud
```

## Setup and Run

### 1. Verify install

```bash
shroud --help
shroud --version
```

### 2. Run your tool through Shroud

```bash
shroud run claude
# or shorthand
shroud claude
```

With dashboard:

```bash
shroud run --ui claude
```

Run another binary/path:

```bash
shroud run /path/to/your-tool [args...]
```

### 3. Useful commands

```bash
shroud logs
shroud status
```

## How It Works (Short Version)

1. Shroud starts local provider-bound proxies (OpenAI + Anthropic).
2. It launches your AI tool with proxy/base-url env vars.
3. Outbound request bodies are masked using deterministic placeholders (for supported HTTP flows).
4. Upstream responses are rehydrated before returning to your tool.
5. Session events are logged locally.

## Logging and Privacy

By default, logs are written under:

- `./logs` (current working directory)

Override with:

- `SHROUD_LOG_DIR=/custom/path`

Log files include session events (mask/rehydrate/request metadata). Shroud is designed to avoid writing raw secret values to session logs.

For deep troubleshooting:

```bash
shroud run --debug-http-log claude
```

## Known Limitations

- If a tool uses HTTPS through `CONNECT` tunneling end-to-end, Shroud can proxy the tunnel but cannot inspect/decrypt inner payloads.
- Best results come from tools that use provider base URL env vars (`OPENAI_BASE_URL` / `ANTHROPIC_BASE_URL`) for direct HTTP API calls.

## Troubleshooting

### Tool not found

Install the tool first, then retry:

- Claude Code: `npm install -g @anthropic-ai/claude-code`
- Aider: `pip install aider-chat`
- DeepAgents: `npm install -g @langchain/deepagents`

### `shroud` command not found after script install

Add `~/.local/bin` to your `PATH` (if not already present), then restart your shell.

## Development

Run tests:

```bash
go test ./...
```

Release builds are managed via GoReleaser and tag-triggered GitHub Actions (`v*` tags).
