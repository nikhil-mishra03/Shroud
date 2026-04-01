package extract

import (
	"testing"
)

func TestSummarize_AnthropicFormat(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4-6",
		"system": "You are a helpful assistant.",
		"messages": [
			{"role": "user", "content": "Hello, my name is [EMAIL_1]. Help me."},
			{"role": "assistant", "content": "Sure, I can help."},
			{"role": "user", "content": "What is the answer?"}
		],
		"tools": [{"name": "read_file"}, {"name": "write_file"}]
	}`

	s := Summarize(body)

	if s.Model != "claude-sonnet-4-6" {
		t.Errorf("model: got %q, want %q", s.Model, "claude-sonnet-4-6")
	}
	if s.ToolCount != 2 {
		t.Errorf("tool_count: got %d, want 2", s.ToolCount)
	}
	if s.MessageCount != 3 {
		t.Errorf("message_count: got %d, want 3", s.MessageCount)
	}
	if s.SystemLen == 0 {
		t.Error("system_len: expected > 0")
	}
	// Only the LAST user turn should be shown — "What is the answer?"
	if s.UserContent != "What is the answer?" {
		t.Errorf("user_content: got %q, want last user turn only", s.UserContent)
	}
	if s.UserLen == 0 {
		t.Error("user_len: expected > 0")
	}
}

func TestSummarize_OpenAIFormat(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "Be concise."},
			{"role": "user", "content": "Summarize this for me."},
			{"role": "assistant", "content": "Here is a summary."},
			{"role": "user", "content": "Thanks, now do [KEY_1] check."}
		]
	}`

	s := Summarize(body)

	if s.Model != "gpt-4o" {
		t.Errorf("model: got %q, want %q", s.Model, "gpt-4o")
	}
	if s.ToolCount != 0 {
		t.Errorf("tool_count: got %d, want 0", s.ToolCount)
	}
	if s.MessageCount != 4 {
		t.Errorf("message_count: got %d, want 4", s.MessageCount)
	}
	if s.SystemLen == 0 {
		t.Error("system_len: expected > 0 (from system role message)")
	}
	if s.UserLen == 0 {
		t.Error("user_len: expected > 0")
	}
}

func TestSummarize_ContentBlocks(t *testing.T) {
	// Anthropic-style content blocks
	body := `{
		"model": "claude-haiku-4-5-20251001",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "Here is my code:"},
				{"type": "text", "text": "print([EMAIL_1])"}
			]}
		]
	}`

	s := Summarize(body)

	if s.UserLen == 0 {
		t.Error("user_len: expected > 0 for content blocks")
	}
	if s.UserContent == "" {
		t.Error("user_content: expected non-empty for content blocks")
	}
}

func TestSummarize_InvalidJSON(t *testing.T) {
	s := Summarize("not json at all")
	if s.Model != "" || s.UserContent != "" || s.ToolCount != 0 {
		t.Error("expected zero-value summary for invalid JSON")
	}
}

func TestSummarize_EmptyBody(t *testing.T) {
	s := Summarize("")
	if s.Model != "" {
		t.Errorf("expected empty model for empty body, got %q", s.Model)
	}
}

func TestSummarize_NoMessages(t *testing.T) {
	body := `{"model": "claude-sonnet-4-6"}`
	s := Summarize(body)
	if s.Model != "claude-sonnet-4-6" {
		t.Errorf("model: got %q", s.Model)
	}
	if s.MessageCount != 0 || s.UserLen != 0 {
		t.Error("expected zero counts for payload with no messages")
	}
}

func TestSummarize_ImageBlocksSkipped(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4-6",
		"messages": [
			{"role": "user", "content": [
				{"type": "image", "source": {"type": "base64", "data": "abc123"}},
				{"type": "text", "text": "What is in this image?"}
			]}
		]
	}`

	s := Summarize(body)
	// Only text block should be in user content, not image data
	if s.UserContent != "What is in this image?" {
		t.Errorf("user_content: got %q, want %q", s.UserContent, "What is in this image?")
	}
}
