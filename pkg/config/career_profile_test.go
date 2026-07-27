package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCareerProfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("create profile directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("# Career Profile\nGrounded facts.\n"), 0600); err != nil {
		t.Fatalf("write career profile: %v", err)
	}
}

func TestResolveCareerProfilePathPrecedence(t *testing.T) {
	baseDir := t.TempDir()
	defaultPath := filepath.Join(baseDir, DefaultCareerProfilePath)
	envPath := filepath.Join(baseDir, "from-env.md")
	flagPath := filepath.Join(baseDir, "from-flag.md")
	writeCareerProfile(t, defaultPath)
	writeCareerProfile(t, envPath)
	writeCareerProfile(t, flagPath)

	got, err := ResolveCareerProfilePath(flagPath, envPath, baseDir)
	if err != nil {
		t.Fatalf("ResolveCareerProfilePath returned an error: %v", err)
	}
	if got != flagPath {
		t.Errorf("resolved path = %q, want explicit flag path %q", got, flagPath)
	}

	got, err = ResolveCareerProfilePath("", envPath, baseDir)
	if err != nil {
		t.Fatalf("ResolveCareerProfilePath returned an error: %v", err)
	}
	if got != envPath {
		t.Errorf("resolved path = %q, want environment path %q", got, envPath)
	}

	got, err = ResolveCareerProfilePath("", "", baseDir)
	if err != nil {
		t.Fatalf("ResolveCareerProfilePath returned an error: %v", err)
	}
	if got != defaultPath {
		t.Errorf("resolved path = %q, want repository default %q", got, defaultPath)
	}
}

func TestResolveCareerProfilePathUsesSiblingLibraryFallback(t *testing.T) {
	parentDir := t.TempDir()
	baseDir := filepath.Join(parentDir, "Career_Agent_Core")
	if err := os.Mkdir(baseDir, 0700); err != nil {
		t.Fatalf("create repository directory: %v", err)
	}
	siblingPath := filepath.Join(parentDir, "ai_knowledge_library", DefaultCareerProfilePath)
	writeCareerProfile(t, siblingPath)

	got, err := ResolveCareerProfilePath("", "", baseDir)
	if err != nil {
		t.Fatalf("ResolveCareerProfilePath returned an error: %v", err)
	}
	if got != siblingPath {
		t.Errorf("resolved path = %q, want sibling-library fallback %q", got, siblingPath)
	}
}

func TestResolveCareerProfilePathRejectsMissingConfiguredPath(t *testing.T) {
	baseDir := t.TempDir()
	writeCareerProfile(t, filepath.Join(baseDir, DefaultCareerProfilePath))
	missingPath := filepath.Join(baseDir, "missing.md")

	_, err := ResolveCareerProfilePath(missingPath, "", baseDir)
	if err == nil {
		t.Fatal("missing configured path must fail instead of falling back")
	}
	if !strings.Contains(err.Error(), "not readable") {
		t.Errorf("error = %q, want a readability failure", err)
	}
}

func TestValidateCareerProfilePathRejectsNonRegularFile(t *testing.T) {
	_, err := ValidateCareerProfilePath(t.TempDir())
	if err == nil {
		t.Fatal("directory must not be accepted as a career profile")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Errorf("error = %q, want regular-file validation", err)
	}
}

func TestResolveCareerProfilePathExplainsExplicitNoRAGEscapeHatch(t *testing.T) {
	_, err := ResolveCareerProfilePath("", "", t.TempDir())
	if err == nil {
		t.Fatal("missing defaults must return an error")
	}
	for _, guidance := range []string{"-profile", CareerProfilePathEnv, "-no-rag"} {
		if !strings.Contains(err.Error(), guidance) {
			t.Errorf("error = %q, want guidance containing %q", err, guidance)
		}
	}
}
