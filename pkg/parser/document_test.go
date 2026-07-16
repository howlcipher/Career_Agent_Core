package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMarkdown(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.md")
	content := "# Hello\nThis is a test."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	got, err := ReadMarkdown(path)
	if err != nil {
		t.Fatalf("ReadMarkdown error: %v", err)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}
}
