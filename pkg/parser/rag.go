package parser

import (
	"math"
	"sort"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

// IngestResumeChunks clears any existing career_chunks and re-chunks/re-embeds
// profilePath's markdown content via embed (typically client.GetEmbedding),
// one chunk per top-level/H2 section (ChunkMarkdown). Returns the number of
// chunks successfully embedded and saved.
//
// A full clear-and-rebuild, not an incremental update: partial/stale state
// (e.g. chunks embedded by a since-changed embedding model, see
// CareerChunksNeedReingest) is worse than an empty cache, which callers
// already handle as "ingest from scratch."
func IngestResumeChunks(embed func(text string) ([]float32, error), profilePath string) (int, error) {
	mdContent, err := ReadMarkdown(profilePath)
	if err != nil {
		return 0, err
	}
	if err := storage.ClearCareerChunks(); err != nil {
		return 0, err
	}

	saved := 0
	for _, text := range ChunkMarkdown(mdContent) {
		if strings.TrimSpace(text) == "" {
			continue
		}
		emb, err := embed(text)
		if err != nil {
			continue
		}
		if err := storage.SaveCareerChunk(text, emb); err != nil {
			continue
		}
		saved++
	}
	return saved, nil
}

// CareerChunksNeedReingest reports whether existing career chunks' embedding
// dimension no longer matches freshDim (the dimension the currently
// configured embed model actually produces). A mismatch means the embed
// model changed since the chunks were last ingested — CosineSimilarity
// silently scores every such comparison 0 (its own mismatched-size guard)
// rather than erroring, so retrieval quietly degrades to an arbitrary order
// instead of failing loudly. Confirmed live 2026-07-24: 8 stored chunks at
// 3072 dimensions while the configured OLLAMA_EMBED_MODEL (nomic-embed-text)
// actually produces 768 — see bugs.md's "stale RAG chunk dimension" entry.
// Empty existing chunks are not a mismatch; that's the "never ingested yet"
// case callers already handle separately.
func CareerChunksNeedReingest(existing []storage.CareerChunk, freshDim int) bool {
	if len(existing) == 0 {
		return false
	}
	for _, c := range existing {
		if len(c.Embedding) != freshDim {
			return true
		}
	}
	return false
}

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

// BestSimilarity returns the highest cosine similarity between queryEmbedding
// and any of the given chunks' embeddings, or 0 if chunks is empty. Used by
// cmd/rankjobs (improvements.md #22) to score a job's title/company against
// the resume as a whole via its best-matching chunk, rather than an average
// that would get diluted by resume sections irrelevant to any given job.
func BestSimilarity(queryEmbedding []float32, chunks []storage.CareerChunk) float32 {
	var best float32
	for _, c := range chunks {
		if s := CosineSimilarity(queryEmbedding, c.Embedding); s > best {
			best = s
		}
	}
	return best
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
