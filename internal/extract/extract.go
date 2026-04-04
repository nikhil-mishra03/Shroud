// Package extract parses LLM API request payloads and returns a structured
// summary. It supports both Anthropic and OpenAI message formats without
// requiring a hard dependency on either SDK.
package extract

import (
	"encoding/json"
	"strings"
)

// OutboundBlock represents a single piece of outbound content from the system to the LLM.
type OutboundBlock struct {
	Role    string `json:"role"`    // "user" or "tool_result"
	Label   string `json:"label"`   // e.g. "User" or tool_use_id
	Content string `json:"content"` // the actual text content (already masked)
}

// RequestSummary holds the user-relevant parts of an outbound LLM request.
// All string fields contain already-masked text — no original secrets.
type RequestSummary struct {
	Model          string          // model name from payload
	UserContent    string          // last user turn text (for meta line)
	SystemLen      int             // character count of system prompt(s) — not sent to UI
	UserLen        int             // character count of user content
	ToolCount      int             // number of tool definitions
	MessageCount   int             // total messages in the conversation
	OutboundBlocks []OutboundBlock // all user-side content blocks (text + tool_results)
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
	// Capture ALL user-side content: user text turns and tool_result blocks.
	// Skip assistant messages (those are Claude's responses, not outbound from us).
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
				s.SystemLen += lenOf(msg["content"])
			case "user":
				blocks := extractOutboundBlocks(msg["content"])
				s.OutboundBlocks = append(s.OutboundBlocks, blocks...)
				// Keep last user text for the meta summary line
				for _, b := range blocks {
					if b.Role == "user" && b.Content != "" {
						lastUserContent = b.Content
					}
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

// extractOutboundBlocks returns all user-side content blocks from a message
// content field. It handles plain strings, text blocks, and tool_result blocks.
// Image blocks are skipped. Assistant messages must not be passed here.
func extractOutboundBlocks(content any) []OutboundBlock {
	switch v := content.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []OutboundBlock{{Role: "user", Label: "User", Content: v}}
	case []any:
		var blocks []OutboundBlock
		for _, item := range v {
			b, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch b["type"] {
			case "text":
				if text, ok := b["text"].(string); ok && text != "" {
					blocks = append(blocks, OutboundBlock{Role: "user", Label: "User", Content: text})
				}
			case "tool_result":
				label, _ := b["tool_use_id"].(string)
				if label == "" {
					label = "tool_result"
				}
				// tool_result content can be a string or an array of text blocks
				text := extractToolResultText(b["content"])
				if text != "" {
					blocks = append(blocks, OutboundBlock{Role: "tool_result", Label: label, Content: text})
				}
			// image blocks intentionally skipped
			}
		}
		return blocks
	default:
		return nil
	}
}

// extractToolResultText extracts plain text from a tool_result content field,
// which can be a string or an array of text blocks.
func extractToolResultText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			b, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if b["type"] == "text" {
				if text, ok := b["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// extractText pulls a plain string out of a message content field.
// Kept for backward compatibility with existing callers.
func extractText(content any) string {
	blocks := extractOutboundBlocks(content)
	var parts []string
	for _, b := range blocks {
		if b.Content != "" {
			parts = append(parts, b.Content)
		}
	}
	return strings.Join(parts, " ")
}
