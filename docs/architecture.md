# Shroud Architecture

## Current Shape

Shroud is a local launcher plus proxy layer for AI coding tools.

The current runtime model is:

1. Start the CLI with `shroud run <tool>`.
2. Create a session logger and secret masker.
3. Optionally start the browser dashboard UI.
4. Start two local provider-bound proxy listeners:
   - OpenAI listener
   - Anthropic listener
5. Launch the child tool with provider-specific local base URLs injected into its environment.
6. Let the tool choose whichever provider-specific base URL it already understands.

This keeps user-facing UX simple:

- `./shroud run codex`
- `./shroud run claude`
- `./shroud run deepagents`
- `./shroud run aider`

No provider prefix or manual provider selection is required at launch time.

## Why The Routing Changed

Earlier, Shroud used one local proxy listener and injected the same local base URL into both:

- `OPENAI_BASE_URL`
- `ANTHROPIC_BASE_URL`

That forced the proxy to infer the upstream provider from request shape alone. This created ambiguity and broke down when:

- clients used newer endpoints such as `/responses`
- different providers exposed similar path structures
- tools hid provider information once redirected to `127.0.0.1`

The single-listener design was also a poor fit for debugging because the proxy had to guess provider identity after the client had already stripped away most context.

## Provider-Bound Listener Strategy

Shroud now uses deterministic provider routing without exposing that complexity to the user.

At runtime:

- one local port is reserved for OpenAI-compatible traffic
- one local port is reserved for Anthropic traffic

The child process receives both env vars:

```text
OPENAI_BASE_URL=http://127.0.0.1:<openai-port>
ANTHROPIC_BASE_URL=http://127.0.0.1:<anthropic-port>
```

The child tool then uses the provider env var it already understands.

Examples:

- Codex should use `OPENAI_BASE_URL`
- Claude Code should use `ANTHROPIC_BASE_URL`
- tools like Deep Agents or Aider can use whichever provider they are configured to talk to

This means Shroud does not need to infer provider from the tool name.

## Proxy Forwarding Model

Each proxy listener is created with a default provider identity.

That means:

- the OpenAI listener always forwards to the saved OpenAI upstream
- the Anthropic listener always forwards to the saved Anthropic upstream

Path-based inference still exists as a fallback, but it is no longer the primary routing mechanism.

The proxy also preserves upstream base paths when forwarding. This matters for OpenAI-style upstreams that may already contain `/v1` in the saved base URL.

Example:

- saved upstream: `https://api.openai.com/v1`
- incoming local request: `/responses`
- forwarded upstream request: `/v1/responses`

This avoids broken requests caused by discarding or duplicating upstream base paths.

## Masking And Rehydration

For ordinary HTTP requests:

1. Read the request body.
2. Mask secrets using deterministic placeholders.
3. Forward the masked body upstream.
4. Read the upstream response.
5. Rehydrate placeholders back to original values.
6. Return the result to the local tool.

For streaming HTTP responses:

- read line-by-line
- keep trailing partial placeholder fragments across chunk boundaries
- rehydrate once the full placeholder is present

Session logs record metadata only. Original secrets are not written to disk.

## UI Model

The optional UI is separate from the provider proxies.

- the dashboard HTTP server listens on its own random local port
- the dashboard uses `/ws` to receive Shroud events from the hub

This UI websocket is unrelated to the upstream AI provider transport used by the child tool.

## Current Limitation

Shroud currently proxies ordinary HTTP request/response traffic and streaming HTTP text responses.

It does not currently implement full websocket upgrade proxying for upstream tool traffic.

This matters because Codex CLI appears to use the OpenAI Responses API and may use websocket-based flows in some cases. Deterministic provider routing fixes one major class of failure, but it does not by itself solve websocket transport support.

## Recommended Next Steps

1. Validate the new provider-bound listener flow against real tools:
   - Codex
   - Claude Code
   - Deep Agents
   - Aider

2. Confirm which tools use:
   - plain HTTP
   - SSE/streaming HTTP
   - websocket upgrades

3. Add websocket proxy support if required by Codex or other tools.

4. Expand proxy tests to cover:
   - provider-bound listeners
   - upstream base path preservation
   - OpenAI Responses API paths
   - transport-specific behavior
