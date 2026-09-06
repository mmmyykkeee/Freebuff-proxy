package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestRegistryAgainstLiveSources hits GitHub raw; skipped without network.
func TestRegistryAgainstLiveSources(t *testing.T) {
	client := &http.Client{Timeout: 30 * time.Second}
	r := NewModelRegistry(client, log.New(os.Stderr, "", 0))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := r.refresh(ctx); err != nil {
		t.Skipf("no network or sources moved: %v", err)
	}
	models := r.Models()
	if len(models) < 5 {
		t.Fatalf("expected >=5 models, got %d: %v", len(models), models)
	}
	t.Logf("parsed %d models", len(models))
	for _, m := range models {
		a, _ := r.AgentForModel(m)
		t.Logf("  %-40s -> %s", m, a)
	}

	// Paused models must NOT be served.
	for _, paused := range []string{"minimax/minimax-m3", "deepseek/deepseek-v4-pro", "stealth/ox-alpha", "z-ai/glm-5.2"} {
		if r.HasModel(paused) {
			t.Errorf("paused model %s must not be in registry", paused)
		}
	}
	// Aliases must resolve.
	if _, ok := r.AgentForModel("mimo-v2.5"); !ok {
		t.Error("alias mimo-v2.5 did not resolve")
	}
	if _, ok := r.AgentForModel("google/gemini-3.1-flash-lite"); !ok {
		t.Error("gemini helper model missing")
	}
}
