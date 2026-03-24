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
	entityType EntityType
	pattern    *regexp.Regexp
}

func New() *Masker {
	m := &Masker{
		mappings:   make(map[string]string),
		reverseMap: make(map[string]string),
		counters:   make(map[EntityType]int),
	}
	m.rules = []*rule{
		{EntityEmail, regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)},
		{EntityKey, regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,}|Bearer\s+[a-zA-Z0-9\-._~+/]+=*|ghp_[a-zA-Z0-9]{36}|xox[baprs]-[a-zA-Z0-9\-]+)`)},
		{EntityToken, regexp.MustCompile(`eyJ[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+`)},
		{EntityIP, regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`)},
		{EntityEnvVar, regexp.MustCompile(`\b([A-Z_]{2,})=([^\s"'\]}` + "`" + `\\]+)`)},
		{EntityCred, regexp.MustCompile(`\b(password|passwd|secret|api_key|auth|token)=(\S+)`)},
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
