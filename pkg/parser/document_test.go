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

func TestExtractDocumentText_PlainText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "letter.txt")
	const want = "Dear Hiring Manager,\n\nI build automation.\n"
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	got, err := ExtractDocumentText(path)
	if err != nil {
		t.Fatalf("ExtractDocumentText failed: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractDocumentText_MissingFile(t *testing.T) {
	if _, err := ExtractDocumentText("/nonexistent/letter.txt"); err == nil {
		t.Error("expected an error for a missing file, got nil")
	}
}
