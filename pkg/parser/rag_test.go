package parser

import (
	"errors"
	"os"
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

func TestBestSimilarity(t *testing.T) {
	chunks := []storage.CareerChunk{
		{Text: "unrelated", Embedding: []float32{0, 1, 0}},
		{Text: "best match", Embedding: []float32{1, 0, 0}},
		{Text: "partial match", Embedding: []float32{0.5, 0.5, 0}},
	}

	got := BestSimilarity([]float32{1, 0, 0}, chunks)
	if got != 1.0 {
		t.Errorf("expected the best-matching chunk's score 1.0, got %f", got)
	}
}

func TestBestSimilarityEmptyChunks(t *testing.T) {
	got := BestSimilarity([]float32{1, 0, 0}, nil)
	if got != 0.0 {
		t.Errorf("expected 0.0 for no chunks, got %f", got)
	}
}

func TestCareerChunksNeedReingest(t *testing.T) {
	matching := []storage.CareerChunk{{Text: "a", Embedding: make([]float32, 768)}}
	if CareerChunksNeedReingest(matching, 768) {
		t.Error("expected no reingest needed when dimensions match")
	}

	stale := []storage.CareerChunk{{Text: "a", Embedding: make([]float32, 3072)}}
	if !CareerChunksNeedReingest(stale, 768) {
		t.Error("expected reingest needed when stored dimension (3072) differs from the current model's (768)")
	}

	if CareerChunksNeedReingest(nil, 768) {
		t.Error("empty existing chunks is the 'never ingested' case, not a mismatch — should not report reingest needed")
	}
}

func TestIngestResumeChunks(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer storage.CloseDB()

	profilePath := filepath.Join(tempDir, "USER_PROFILE.md")
	content := "# Section One\nSome content.\n## Section Two\nMore content."
	if err := os.WriteFile(profilePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test profile: %v", err)
	}

	// Pre-seed a stale chunk to confirm ingestion clears before rebuilding.
	storage.SaveCareerChunk("stale leftover", []float32{9, 9, 9})

	calls := 0
	stubEmbed := func(text string) ([]float32, error) {
		calls++
		return []float32{1, 0, 0}, nil
	}

	n, err := IngestResumeChunks(stubEmbed, profilePath)
	if err != nil {
		t.Fatalf("IngestResumeChunks failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 chunks embedded, got %d", n)
	}
	if calls != 2 {
		t.Errorf("expected embed to be called once per chunk (2), got %d", calls)
	}

	chunks, err := storage.GetAllCareerChunks()
	if err != nil {
		t.Fatalf("GetAllCareerChunks failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks in storage after ingest, got %d", len(chunks))
	}
	for _, c := range chunks {
		if c.Text == "stale leftover" {
			t.Error("expected the pre-existing stale chunk to be cleared before re-ingestion, but it survived")
		}
	}
}

func TestIngestResumeChunksSkipsFailedEmbeddings(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	if err := storage.InitDBWithPath(dbPath); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer storage.CloseDB()

	profilePath := filepath.Join(tempDir, "USER_PROFILE.md")
	content := "# Section One\nContent."
	if err := os.WriteFile(profilePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test profile: %v", err)
	}

	failingEmbed := func(text string) ([]float32, error) {
		return nil, errors.New("embedding service unavailable")
	}

	n, err := IngestResumeChunks(failingEmbed, profilePath)
	if err != nil {
		t.Fatalf("IngestResumeChunks should not error just because embedding fails per-chunk: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 chunks saved when every embed call fails, got %d", n)
	}
}
