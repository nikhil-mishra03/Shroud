// Package extract parses LLM API request payloads and returns a structured
// summary. It supports both Anthropic and OpenAI message formats without
// requiring a hard dependency on either SDK.
package extract

import (
	"encoding/json"
	"strings"
)

// RequestSummary holds the user-relevant parts of an outbound LLM request.
// All string fields contain already-masked text — no original secrets.
type RequestSummary struct {
	Model        string // model name from payload
	UserContent  string // concatenated user message content (masked)
	SystemLen    int    // character count of system prompt(s) — not sent to UI
	UserLen      int    // character count of user content
	ToolCount    int    // number of tool definitions
	MessageCount int    // total messages in the conversation
}

// Summarize parses a JSON request body (Anthropic or OpenAI format) and
// returns a RequestSummary. If the body is not parseable JSON or does not
// contain a known messages field, a zero-value summary is returned.
func Summarize(body string) RequestSummary {
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return RequestSummary{}
	}

	var s RequestSummary

	// model
	if m, ok := raw["model"].(string); ok {
		s.Model = m
	}

	// tools / functions count
	if tools, ok := raw["tools"].([]any); ok {
		s.ToolCount = len(tools)
	} else if fns, ok := raw["functions"].([]any); ok {
		s.ToolCount = len(fns)
	}

	// system prompt — Anthropic uses a top-level "system" string or array;
	// OpenAI embeds system messages inside "messages".
	if sys, ok := raw["system"]; ok {
		s.SystemLen += lenOf(sys)
	}

	// messages array — present in both formats.
	// We only show the LAST user message (the current turn). Prior user turns
	// are context history and tool results — not what the user wants to see.
	if msgs, ok := raw["messages"].([]any); ok {
		s.MessageCount = len(msgs)
		var lastUserContent string
		for _, m := range msgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			switch role {
			case "system":
				// OpenAI system role — measure but don't show
				s.SystemLen += lenOf(msg["content"])
			case "user":
				// Keep overwriting so we end up with the last user turn only.
				txt := extractText(msg["content"])
				if txt != "" {
					lastUserContent = txt
				}
			}
		}
		s.UserContent = lastUserContent
		s.UserLen = len(s.UserContent)
	}

	return s
}

// lenOf returns the character count of an arbitrary JSON value rendered as its
// string representation. Used to estimate system prompt size.
func lenOf(v any) int {
	switch val := v.(type) {
	case string:
		return len(val)
	case []any:
		total := 0
		for _, item := range val {
			total += lenOf(item)
		}
		return total
	case map[string]any:
		if text, ok := val["text"].(string); ok {
			return len(text)
		}
		b, _ := json.Marshal(val)
		return len(b)
	default:
		return 0
	}
}

// extractText pulls a plain string out of a message content field, which can
// be a string (OpenAI simple format) or an array of content blocks
// (Anthropic / OpenAI vision format).
func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, block := range v {
			b, ok := block.(map[string]any)
			if !ok {
				continue
			}
			// text block: {"type": "text", "text": "..."}
			if b["type"] == "text" {
				if text, ok := b["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
			// tool_result / image blocks are intentionally skipped
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}
