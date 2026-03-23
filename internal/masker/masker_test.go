package masker

import (
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
