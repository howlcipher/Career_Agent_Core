package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPIITemplateParsesWithoutPersonalData(t *testing.T) {
	path := filepath.Join("..", "..", "pii.yaml.template")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	tmp, err := os.CreateTemp(t.TempDir(), "pii-template-*.yaml")
	if err != nil {
		t.Fatalf("create temporary PII file: %v", err)
	}
	defer tmp.Close()
	if _, err := tmp.Write(data); err != nil {
		t.Fatalf("write temporary PII file: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temporary PII file: %v", err)
	}

	got, err := LoadPII(tmp.Name())
	if err != nil {
		t.Fatalf("template must remain valid PII YAML: %v", err)
	}
	if got.FirstName != "Your first name" || got.Email != "you@example.com" {
		t.Fatalf("template no longer contains safe placeholder values: %+v", got)
	}
	if got.Work.AuthorizedToWorkUS != "" || got.Work.RequiresSponsorship != "" {
		t.Fatal("legal attestations in the template must remain blank")
	}
}

func TestREADMENamesCurrentSetupEntrypoints(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	text := string(readme)
	for _, required := range []string{
		"pii.yaml.template",
		"CAREER_PROFILE_PATH",
		"OLLAMA_TIMEOUT_MINUTES",
		"http://127.0.0.1:8080",
		"go run cmd/agent/main.go --daemon",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("README.md is missing current setup entrypoint %q", required)
		}
	}
}
