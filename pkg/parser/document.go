package parser

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

func ReadMarkdown(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read markdown file: %w", err)
	}
	return string(content), nil
}

// ExtractDocumentText returns a document's plain text, transparently handling
// both formats a master document can realistically be supplied in: a PDF (text
// extracted) or a plain-text/markdown file (read as-is). Callers that need to
// paste a document's contents into a form field must go through this rather
// than os.ReadFile, or a PDF's raw binary ends up in the field.
func ExtractDocumentText(path string) (string, error) {
	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		return ExtractFromFallbackPDF(path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read document: %w", err)
	}
	return string(content), nil
}

func ExtractFromFallbackPDF(path string) (string, error) {
	if path == "" {
		path = "__William_Elias_Resume__.pdf"
	}

	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open pdf: %w", err)
	}
	defer f.Close()

	b, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("failed to extract text from pdf: %w", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(b); err != nil {
		return "", fmt.Errorf("failed to read from pdf reader: %w", err)
	}

	return buf.String(), nil
}
