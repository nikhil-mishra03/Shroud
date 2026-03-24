package masker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskEmail(t *testing.T) {
	m := New()
	out, events := m.Mask("Contact john@company.com for info")
	if strings.Contains(out, "john@company.com") {
		t.Error("email not masked")
	}
	if len(events) != 1 || events[0].Entity != EntityEmail {
		t.Errorf("expected 1 email event, got %+v", events)
	}
	if !strings.Contains(out, "[EMAIL_1]") {
		t.Errorf("expected [EMAIL_1] placeholder, got: %s", out)
	}
}

func TestMaskAPIKey(t *testing.T) {
	m := New()
	out, events := m.Mask("Using sk-abc123def456ghi789jkl012mno345pqr")
	if strings.Contains(out, "sk-abc") {
		t.Error("API key not masked")
	}
	if len(events) != 1 || events[0].Entity != EntityKey {
		t.Errorf("expected 1 key event, got %+v", events)
	}
}

func TestMaskIP(t *testing.T) {
	m := New()
	out, events := m.Mask("Server at 10.2.3.4 is down")
	if strings.Contains(out, "10.2.3.4") {
		t.Error("IP not masked")
	}
	if len(events) != 1 || events[0].Entity != EntityIP {
		t.Errorf("expected 1 IP event, got %+v", events)
	}
	if !strings.Contains(out, "[IP_1]") {
		t.Errorf("expected [IP_1] placeholder, got: %s", out)
	}
}

func TestMaskJWT(t *testing.T) {
	m := New()
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	out, events := m.Mask("Token: " + jwt)
	if strings.Contains(out, jwt) {
		t.Error("JWT not masked")
	}
	if len(events) != 1 || events[0].Entity != EntityToken {
		t.Errorf("expected 1 token event, got %+v", events)
	}
}

func TestDeterministicMapping(t *testing.T) {
	m := New()
	out1, _ := m.Mask("user john@company.com logged in")
	out2, _ := m.Mask("user john@company.com logged out")
	// Same email should get same placeholder both times
	if !strings.Contains(out1, "[EMAIL_1]") || !strings.Contains(out2, "[EMAIL_1]") {
		t.Error("same email should get same placeholder")
	}
}

func TestMultipleEntities(t *testing.T) {
	m := New()
	input := "User john@company.com accessed 10.2.3.4 using sk-abc123def456ghi789jkl012mno345pqr"
	out, events := m.Mask(input)
	if strings.Contains(out, "john@company.com") {
		t.Error("email not masked in multi-entity")
	}
	if strings.Contains(out, "10.2.3.4") {
		t.Error("IP not masked in multi-entity")
	}
	if strings.Contains(out, "sk-abc") {
		t.Error("key not masked in multi-entity")
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d: %+v", len(events), events)
	}
}

func TestRehydrate(t *testing.T) {
	m := New()
	masked, _ := m.Mask("Contact john@company.com about 10.2.3.4")
	rehydrated := m.Rehydrate(masked)
	if !strings.Contains(rehydrated, "john@company.com") {
		t.Error("email not rehydrated")
	}
	if !strings.Contains(rehydrated, "10.2.3.4") {
		t.Error("IP not rehydrated")
	}
}

func TestNoFalsePositives(t *testing.T) {
	m := New()
	safe := "The quick brown fox jumps over the lazy dog"
	out, events := m.Mask(safe)
	if out != safe {
		t.Errorf("safe text was modified: %s", out)
	}
	if len(events) != 0 {
		t.Errorf("unexpected mask events: %+v", events)
	}
}

func TestMaskLowercaseCredentials(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"password", "password=ghostrider1221"},
		{"passwd", "passwd=hunter2"},
		{"secret", "secret=abc123"},
		{"api_key", "api_key=xyz789"},
		{"token", "token=mytoken"},
		{"auth", "auth=mysecrettoken"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New()
			out, events := m.Mask(tc.input)
			if strings.Contains(out, tc.input) {
				t.Errorf("credential not masked: %s", out)
			}
			if len(events) != 1 || events[0].Entity != EntityCred {
				t.Errorf("expected 1 CRED event, got %+v", events)
			}
			// Key name must be preserved
			key := strings.SplitN(tc.input, "=", 2)[0]
			if !strings.HasPrefix(out, key+"=") {
				t.Errorf("key name not preserved in output: %s", out)
			}
		})
	}
}

func TestMaskEnvVarInsideEscapedJSONPreservesValidJSON(t *testing.T) {
	m := New()
	input := `{"text":"set DEBUG=1\\\", \\\"when claude stops show X"}`

	out, events := m.Mask(input)

	if len(events) != 1 || events[0].Entity != EntityEnvVar {
		t.Fatalf("expected 1 env var event, got %+v", events)
	}
	if !strings.Contains(out, `DEBUG=[ENV_1]\\\", \\\"when`) {
		t.Fatalf("masked output did not preserve escaped quote boundary: %s", out)
	}

	var decoded map[string]string
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("masked output is not valid JSON: %v\n%s", err, out)
	}
	if !strings.Contains(decoded["text"], `DEBUG=[ENV_1]`) {
		t.Fatalf("decoded text missing masked env var: %q", decoded["text"])
	}
}
