package parser

import (
	"math"
	"sort"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// ChunkMarkdown splits a markdown document into logical chunks based on headers.
func ChunkMarkdown(content string) []string {
	// A naive split on Level 1 and Level 2 headers
	lines := strings.Split(content, "\n")
	var chunks []string
	var currentChunk []string

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") {
			if len(currentChunk) > 0 {
				chunks = append(chunks, strings.Join(currentChunk, "\n"))
				currentChunk = nil
			}
		}
		currentChunk = append(currentChunk, line)
	}
	
	if len(currentChunk) > 0 {
		chunks = append(chunks, strings.Join(currentChunk, "\n"))
	}
	
	return chunks
}

// CosineSimilarity calculates the similarity between two embedding vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

type ScoredChunk struct {
	Chunk storage.CareerChunk
	Score float32
}

// RetrieveTopK searches the local SQLite chunks and returns the Top K most semantically relevant chunks.
func RetrieveTopK(queryEmbedding []float32, k int) ([]storage.CareerChunk, error) {
	chunks, err := storage.GetAllCareerChunks()
	if err != nil {
		return nil, err
	}

	var scored []ScoredChunk
	for _, chunk := range chunks {
		score := CosineSimilarity(queryEmbedding, chunk.Embedding)
		scored = append(scored, ScoredChunk{Chunk: chunk, Score: score})
	}

	// Sort descending by score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	var topK []storage.CareerChunk
	for i := 0; i < len(scored) && i < k; i++ {
		topK = append(topK, scored[i].Chunk)
	}
	return topK, nil
}
