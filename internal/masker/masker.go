package masker

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

type EntityType string

const (
	EntityEmail  EntityType = "EMAIL"
	EntityIP     EntityType = "IP"
	EntityKey    EntityType = "KEY"
	EntityToken  EntityType = "TOKEN"
	EntityEnvVar EntityType = "ENV"
	EntityCred   EntityType = "CRED"
)

type MaskEvent struct {
	Entity      EntityType
	Placeholder string
	Original    string
}

type Masker struct {
	mu         sync.Mutex
	mappings   map[string]string // placeholder -> original
	reverseMap map[string]string // original -> placeholder
	counters   map[EntityType]int
	rules      []*rule
}

type rule struct {
	entityType    EntityType
	pattern       *regexp.Regexp
	// contextFilter, when non-nil, is called for each regex match with the full
	// text and the [start, end) byte offsets of the match. If it returns true,
	// the match is suppressed (treated as a false positive).
	contextFilter func(text string, start, end int) bool
}

func New() *Masker {
	m := &Masker{
		mappings:   make(map[string]string),
		reverseMap: make(map[string]string),
		counters:   make(map[EntityType]int),
	}
	m.rules = []*rule{
		{EntityEmail, regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`), nil},
		{EntityKey, regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,}|Bearer\s+[a-zA-Z0-9\-._~+/]+=*|ghp_[a-zA-Z0-9]{36}|xox[baprs]-[a-zA-Z0-9\-]+)`), nil},
		{EntityToken, regexp.MustCompile(`eyJ[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+`), nil},
		{EntityIP, regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`), isVersionStringContext},
		{EntityEnvVar, regexp.MustCompile(`\b([A-Z_]{2,})=([^\s"'\]}` + "`" + `\\]+)`), nil},
		{EntityCred, regexp.MustCompile(`\b(password|passwd|secret|api_key|auth|token)=([^\s"'\\,\]}\[]+)`), nil},
	}
	return m
}

// Mask replaces sensitive values in text with placeholders.
// Returns the masked text and a list of mask events.
func (m *Masker) Mask(text string) (string, []MaskEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	events := []MaskEvent{}
	result := text

	// Track originals → placeholder within this call to avoid double-masking
	seen := make(map[string]string)

	for _, r := range m.rules {
		// Rules with two capture groups (key=value format): mask only the value,
		// preserving the key name in the output.
		if r.entityType == EntityEnvVar || r.entityType == EntityCred {
			et := r.entityType
			result = r.pattern.ReplaceAllStringFunc(result, func(match string) string {
				subs := r.pattern.FindStringSubmatch(match)
				if len(subs) < 3 {
					return match
				}
				val := subs[2]
				// Skip if the value is already a Shroud placeholder to avoid
				// double-masking when rules run sequentially on the same string.
				if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
					return match
				}
				if ph, ok := seen[val]; ok {
					return subs[1] + "=" + ph
				}
				ph := m.placeholderLocked(et, val)
				seen[val] = ph
				events = append(events, MaskEvent{et, ph, val})
				return subs[1] + "=" + ph
			})
			continue
		}

		// Rules with a contextFilter use index-based replacement so we can inspect
		// the surrounding text at each match position.
		if r.contextFilter != nil {
			locs := r.pattern.FindAllStringIndex(result, -1)
			if len(locs) == 0 {
				continue
			}
			var b strings.Builder
			prev := 0
			for _, loc := range locs {
				start, end := loc[0], loc[1]
				b.WriteString(result[prev:start])
				match := result[start:end]
				if r.contextFilter(result, start, end) {
					// Suppressed by context filter — emit as-is.
					b.WriteString(match)
				} else if ph, ok := seen[match]; ok {
					b.WriteString(ph)
				} else {
					ph := m.placeholderLocked(r.entityType, match)
					seen[match] = ph
					events = append(events, MaskEvent{r.entityType, ph, match})
					b.WriteString(ph)
				}
				prev = end
			}
			b.WriteString(result[prev:])
			result = b.String()
			continue
		}

		result = r.pattern.ReplaceAllStringFunc(result, func(match string) string {
			if ph, ok := seen[match]; ok {
				return ph
			}
			ph := m.placeholderLocked(r.entityType, match)
			seen[match] = ph
			events = append(events, MaskEvent{r.entityType, ph, match})
			return ph
		})
	}

	return result, events
}

// Rehydrate replaces placeholders in text with their original values.
func (m *Masker) Rehydrate(text string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := text
	for ph, original := range m.mappings {
		result = strings.ReplaceAll(result, ph, original)
	}
	return result
}

// Mappings returns a snapshot of placeholder → original mappings.
func (m *Masker) Mappings() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := make(map[string]string, len(m.mappings))
	for k, v := range m.mappings {
		snap[k] = v
	}
	return snap
}

// Severity returns the risk tier for an entity type.
// "critical" — actual credentials (keys, tokens, passwords)
// "moderate" — PII or config values (email, env vars)
// "low"      — ambient values unlikely to be sensitive (IPs)
func Severity(t EntityType) string {
	switch t {
	case EntityKey, EntityToken, EntityCred:
		return "critical"
	case EntityEmail, EntityEnvVar:
		return "moderate"
	default:
		return "low"
	}
}

// isVersionStringContext returns true when an IP-shaped match (e.g. "1.2.3.4")
// appears to be a software version string rather than a real IP address.
// It suppresses masking for common version string patterns like "v1.2.3.4",
// "go1.21.3", or anything immediately preceded by a letter or digit-dot context.
func isVersionStringContext(text string, start, _ int) bool {
	if start == 0 {
		return false
	}
	prev := text[start-1]
	// Preceded by a letter (e.g. "go1.", "v1.") — version string
	if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') {
		return true
	}
	return false
}

func (m *Masker) placeholderLocked(t EntityType, original string) string {
	if ph, ok := m.reverseMap[original]; ok {
		return ph
	}
	m.counters[t]++
	ph := fmt.Sprintf("[%s_%d]", t, m.counters[t])
	m.mappings[ph] = original
	m.reverseMap[original] = ph
	return ph
}
