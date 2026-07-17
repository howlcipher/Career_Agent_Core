package parser

import (
	"path/filepath"
	"testing"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func TestChunkMarkdown(t *testing.T) {
	input := "# Header 1\nContent 1\n## Header 2\nContent 2"
	chunks := ChunkMarkdown(input)
	
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0] != "# Header 1\nContent 1" {
		t.Errorf("unexpected chunk 0: %q", chunks[0])
	}
	if chunks[1] != "## Header 2\nContent 2" {
		t.Errorf("unexpected chunk 1: %q", chunks[1])
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	score := CosineSimilarity(a, b)
	if score != 1.0 {
		t.Errorf("expected 1.0, got %f", score)
	}

	c := []float32{0, 1, 0}
	score = CosineSimilarity(a, c)
	if score != 0.0 {
		t.Errorf("expected 0.0, got %f", score)
	}
}

func TestRetrieveTopK(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	err := storage.InitDBWithPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer storage.CloseDB()

	storage.ClearCareerChunks()
	storage.SaveCareerChunk("chunk 1", []float32{1, 0, 0})
	storage.SaveCareerChunk("chunk 2", []float32{0, 1, 0})

	results, err := RetrieveTopK([]float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatalf("RetrieveTopK error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Text != "chunk 1" {
		t.Errorf("expected chunk 1, got %q", results[0].Text)
	}
}

func TestCosineSimilarityMismatchedSize(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0}
	
	// This should not panic
	score := CosineSimilarity(a, b)
	if score != 0.0 {
		t.Errorf("expected 0.0 for mismatched sizes, got %f", score)
	}
}
