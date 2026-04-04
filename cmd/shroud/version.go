package main

// These variables are overwritten at build time via ldflags.
// Example:
//
//	go build -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o shroud ./cmd/shroud
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)
