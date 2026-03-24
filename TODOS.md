# TODOS

## Open

### IP regex false positives on version strings
**What:** The IPv4 masker regex (`masker.go:47`) matches any 4-octet dotted-decimal number, including version strings like `1.2.3.4` or `0.11.15.0`.

**Why:** Developers routinely mention software versions in prompts. If a version string is masked, the LLM sees `[IP_1]` instead and may produce unhelpful or confusing answers. The user also sees `[IP_1]` in the session summary, which is misleading.

**Pros:** Fixing this eliminates a class of false positives that will confuse users once Shroud is used on real developer workflows.

**Cons:** Go's RE2 engine doesn't support lookarounds, so the fix requires post-match context filtering (not just a regex change). Adds a small amount of complexity to the masker hot path.

**Context:** The current regex is `\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`. A version string like `1.2.3.4` or `0.11.15.0` satisfies this. A reasonable heuristic: if the matched string is preceded by `v` or `/` (e.g., `v1.2.3.4`, `go@1.21.0`), treat it as a version, not an IP.

**Depends on:** Nothing — standalone fix in `internal/masker/masker.go`.

---

### NO_PROXY values in Codex environment contain placeholder strings
**What:** `cmd/shroud/main.go` lines 155-156 set `NO_PROXY` and `no_proxy` in the Codex child environment with literal placeholder-like values (e.g. `[ENV_N]]`) instead of real IP/hostname values. These need to be valid addresses (e.g. `127.0.0.1,localhost`) so Codex doesn't try to route loopback traffic through itself.

**Why:** Codex uses a forward-proxy model (HTTPS_PROXY) to reach the LLM API through Shroud. The NO_PROXY value is supposed to exclude localhost so the proxy doesn't try to proxy its own loopback calls — with broken placeholder values, this exclusion is silently non-functional.

**Pros:** Fixing ensures Codex's proxy exclusion list works correctly; prevents potential loopback routing loops.

**Cons:** Need to confirm the intended value (127.0.0.1? 0.0.0.0/0? both?) before changing.

**Context:** The placeholder strings appear to be masking artifacts from when the code was developed through Shroud itself. The correct value is almost certainly `127.0.0.1,localhost` or `[::1],localhost` for IPv6 support.

**Depends on:** Codex integration testing — fix together with websocket/Responses API support work.
