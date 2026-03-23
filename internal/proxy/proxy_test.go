package proxy

import (
	"net/http/httptest"
	"testing"

	"github.com/nimishr2/shroud/internal/masker"
	"github.com/nimishr2/shroud/internal/session"
)

func TestResolveUpstreamUsesDefaultProvider(t *testing.T) {
	p := New(masker.New(), &session.Logger{}, nil, map[string]string{
		"openai":    "https://api.openai.com/v1",
		"anthropic": "https://api.anthropic.com",
	}, "openai", false)

	r := httptest.NewRequest("POST", "http://127.0.0.1/responses", nil)
	if got := p.resolveUpstream(r); got != "https://api.openai.com/v1" {
		t.Fatalf("resolveUpstream() = %q, want %q", got, "https://api.openai.com/v1")
	}
}

func TestResolveUpstreamFallsBackToPathMatching(t *testing.T) {
	p := New(masker.New(), &session.Logger{}, nil, map[string]string{
		"openai":    "https://api.openai.com/v1",
		"anthropic": "https://api.anthropic.com",
	}, "", false)

	r := httptest.NewRequest("POST", "http://127.0.0.1/v1/messages", nil)
	if got := p.resolveUpstream(r); got != "https://api.anthropic.com" {
		t.Fatalf("resolveUpstream() = %q, want %q", got, "https://api.anthropic.com")
	}
}

func TestJoinURLPath(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		req     string
		wantOut string
	}{
		{
			name:    "root base keeps request path",
			base:    "",
			req:     "/v1/messages",
			wantOut: "/v1/messages",
		},
		{
			name:    "base v1 prefixes bare responses path",
			base:    "/v1",
			req:     "/responses",
			wantOut: "/v1/responses",
		},
		{
			name:    "base v1 does not double prefix v1 path",
			base:    "/v1",
			req:     "/v1/chat/completions",
			wantOut: "/v1/chat/completions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinURLPath(tc.base, tc.req); got != tc.wantOut {
				t.Fatalf("joinURLPath(%q, %q) = %q, want %q", tc.base, tc.req, got, tc.wantOut)
			}
		})
	}
}
