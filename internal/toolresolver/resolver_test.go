package toolresolver

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// createFakeBin creates a dummy executable in dir with the given name.
func createFakeBin(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		p += ".exe"
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolve_KnownTool(t *testing.T) {
	dir := t.TempDir()
	createFakeBin(t, dir, "claude")

	// Prepend temp dir to PATH so LookPath finds our fake binary.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	binPath, info, err := Resolve("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.FriendlyName != "Claude Code" {
		t.Errorf("friendly name: got %q, want %q", info.FriendlyName, "Claude Code")
	}
	if !strings.HasSuffix(binPath, "claude") {
		t.Errorf("binPath %q should end with 'claude'", binPath)
	}
}

func TestResolve_KnownToolCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	createFakeBin(t, dir, "claude")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, info, err := Resolve("Claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.FriendlyName != "Claude Code" {
		t.Errorf("case-insensitive lookup failed: got %q", info.FriendlyName)
	}
}

func TestResolve_KnownToolNotInstalled(t *testing.T) {
	// Empty PATH so nothing is found.
	t.Setenv("PATH", t.TempDir())

	_, _, err := Resolve("claude")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
	if !strings.Contains(err.Error(), "npm install") {
		t.Errorf("error should contain install hint, got: %v", err)
	}
}

func TestResolve_UnknownToolOnPath(t *testing.T) {
	dir := t.TempDir()
	createFakeBin(t, dir, "my-custom-tool")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	binPath, info, err := Resolve("my-custom-tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.FriendlyName != "my-custom-tool" {
		t.Errorf("friendly name: got %q, want %q", info.FriendlyName, "my-custom-tool")
	}
	if binPath == "" {
		t.Error("binPath should not be empty")
	}
}

func TestResolve_UnknownToolNotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, _, err := Resolve("nonexistent-tool-xyz")
	if err == nil {
		t.Fatal("expected error for unknown missing tool")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("error should say 'not found on PATH', got: %v", err)
	}
}

func TestResolve_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	bin := createFakeBin(t, dir, "sometool")

	// Absolute path should resolve directly, bypassing the registry.
	binPath, _, err := Resolve(bin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if binPath != bin {
		t.Errorf("expected exact path %q, got %q", bin, binPath)
	}
}

func TestResolve_AiderKnown(t *testing.T) {
	dir := t.TempDir()
	createFakeBin(t, dir, "aider")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, info, err := Resolve("aider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.FriendlyName != "Aider" {
		t.Errorf("friendly name: got %q, want %q", info.FriendlyName, "Aider")
	}
}

// Verify exec.LookPath is used (not just string matching).
func TestResolve_UsesLookPath(t *testing.T) {
	// This test ensures we're actually calling exec.LookPath, not just
	// checking if the file exists. LookPath checks executability.
	dir := t.TempDir()

	// Create a non-executable file — LookPath should skip it on Unix.
	p := filepath.Join(dir, "claude")
	os.WriteFile(p, []byte("not executable"), 0o644)

	t.Setenv("PATH", dir)

	_, _, err := Resolve("claude")
	if runtime.GOOS != "windows" {
		// On Unix, non-executable files should not resolve.
		if err == nil {
			// Check if exec.LookPath actually respects permissions on this OS.
			// Some systems (e.g., CI containers) may run as root where this doesn't apply.
			if _, lookErr := exec.LookPath(p); lookErr != nil {
				t.Fatal("expected error for non-executable file")
			}
		}
	}
}
