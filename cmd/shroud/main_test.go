package main

import (
	"strings"
	"testing"
)

func TestIsCodexTool(t *testing.T) {
	tests := []struct {
		tool string
		want bool
	}{
		{"codex", true},
		{"Codex", true},
		{"CODEX", true},
		{"/usr/local/bin/codex", true},
		{"claude", false},
		{"aider", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isCodexTool(tc.tool); got != tc.want {
			t.Errorf("isCodexTool(%q) = %v, want %v", tc.tool, got, tc.want)
		}
	}
}

func TestWithoutEnv(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"OPENAI_BASE_URL=https://api.openai.com",
		"HOME=/Users/test",
		"ANTHROPIC_BASE_URL=https://api.anthropic.com",
	}

	got := withoutEnv(env, "OPENAI_BASE_URL")
	for _, entry := range got {
		if strings.HasPrefix(entry, "OPENAI_BASE_URL=") {
			t.Errorf("withoutEnv did not remove OPENAI_BASE_URL: %v", got)
		}
	}
	if len(got) != 3 {
		t.Errorf("withoutEnv() len = %d, want 3; result: %v", len(got), got)
	}

	// Removing a key that doesn't exist returns all entries unchanged
	got2 := withoutEnv(env, "NONEXISTENT")
	if len(got2) != len(env) {
		t.Errorf("withoutEnv with missing key changed len: got %d, want %d", len(got2), len(env))
	}
}

func TestCodexEnvInjection(t *testing.T) {
	// Verify that the codex path sets HTTPS_PROXY and strips provider base URLs.
	base := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_BASE_URL=https://api.anthropic.com",
		"OPENAI_BASE_URL=https://api.openai.com",
	}

	proxyAddr := "http://127.0.0.1:9999"

	env := withoutEnv(base, "OPENAI_BASE_URL")
	env = withoutEnv(env, "ANTHROPIC_BASE_URL")
	env = append(env,
		"HTTPS_PROXY="+proxyAddr,
		"HTTP_PROXY="+proxyAddr,
	)

	for _, entry := range env {
		if strings.HasPrefix(entry, "ANTHROPIC_BASE_URL=") {
			t.Errorf("codex env still contains ANTHROPIC_BASE_URL")
		}
		if strings.HasPrefix(entry, "OPENAI_BASE_URL=") {
			t.Errorf("codex env still contains OPENAI_BASE_URL")
		}
	}

	hasHTTPS := false
	for _, entry := range env {
		if entry == "HTTPS_PROXY="+proxyAddr {
			hasHTTPS = true
		}
	}
	if !hasHTTPS {
		t.Errorf("codex env missing HTTPS_PROXY=%s", proxyAddr)
	}
}
