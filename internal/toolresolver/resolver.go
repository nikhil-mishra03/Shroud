// Package toolresolver maps friendly tool names (e.g. "claude") to actual
// binaries on the user's PATH. It provides install hints when a known tool
// is missing and falls back to exec.LookPath for unknown names.
package toolresolver

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ToolInfo describes a known AI coding tool.
type ToolInfo struct {
	FriendlyName string   // human-readable name shown in messages
	BinaryNames  []string // candidate binary names, checked in order
	InstallHint  string   // shown when the tool is not found
}

// KnownTools maps lowercase friendly names to their resolution metadata.
// Adding a new tool is a single map entry — no protocol logic here.
var KnownTools = map[string]ToolInfo{
	"claude": {
		FriendlyName: "Claude Code",
		BinaryNames:  []string{"claude"},
		InstallHint:  "Install Claude Code: npm install -g @anthropic-ai/claude-code",
	},
	"aider": {
		FriendlyName: "Aider",
		BinaryNames:  []string{"aider"},
		InstallHint:  "Install Aider: pip install aider-chat",
	},
}

// Resolve looks up a tool by friendly name or raw binary name/path.
//
// Resolution order:
//  1. If name matches a KnownTools entry, try each BinaryNames via exec.LookPath.
//  2. Otherwise, treat name as a literal binary name or path and call exec.LookPath directly.
//  3. If nothing is found, return an error with an install hint (for known tools)
//     or a generic message (for unknown tools).
func Resolve(name string) (binPath string, info ToolInfo, err error) {
	lower := strings.ToLower(name)

	// Check known tools first.
	if ti, ok := KnownTools[lower]; ok {
		for _, bin := range ti.BinaryNames {
			if p, lookErr := exec.LookPath(bin); lookErr == nil {
				return p, ti, nil
			}
		}
		return "", ti, fmt.Errorf("%s not found on PATH.\n  %s", ti.FriendlyName, ti.InstallHint)
	}

	// Unknown tool — try as-is (supports absolute paths and arbitrary binaries).
	if p, lookErr := exec.LookPath(name); lookErr == nil {
		return p, ToolInfo{FriendlyName: filepath.Base(name)}, nil
	}

	return "", ToolInfo{}, fmt.Errorf("%q not found on PATH", name)
}
