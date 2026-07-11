//go:build ignore

// Command gen regenerates fixture.json from demo.BuildDataset(). Run it via
// `go generate ./internal/demo` (cwd = the package dir) whenever BuildDataset
// changes; TestFixtureMatchesBuilder fails until the committed fixture is fresh.
package main

import (
	"encoding/json"
	"log"
	"os"

	"help-my-run/backend/internal/demo"
)

func main() {
	b, err := json.MarshalIndent(demo.BuildDataset(), "", "  ")
	if err != nil {
		log.Fatalf("marshal dataset: %v", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile("fixture.json", b, 0o644); err != nil {
		log.Fatalf("write fixture.json: %v", err)
	}
	log.Printf("wrote fixture.json (%d bytes)", len(b))
}
