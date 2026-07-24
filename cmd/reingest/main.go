// cmd/reingest clears and rebuilds the RAG career_chunks cache from
// USER_PROFILE.md — the same ingestion cmd/agent runs automatically on a
// fresh install or when it detects the stored chunks' embedding dimension no
// longer matches the configured embed model (see
// parser.CareerChunksNeedReingest). Exposed as its own command for two
// cases cmd/agent's own startup check can't cover without a restart: (1)
// fixing a stale cache against a database a live cmd/agent process is
// already using (career_chunks is read fresh per job via
// parser.RetrieveTopK, so a separate short-lived writer takes effect on that
// process's very next job, no restart needed — SQLite WAL mode supports
// concurrent readers/writers), and (2) manually refreshing after editing
// USER_PROFILE.md.
//
// Usage:
//
//	go run ./cmd/reingest
//	go run ./cmd/reingest -profile /path/to/USER_PROFILE.md
package main

import (
	"flag"
	"log"
	"os"

	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func main() {
	profilePath := flag.String("profile", "/var/home/howlcipher/dev/ai_knowledge_library/USER_PROFILE.md", "path to USER_PROFILE.md")
	flag.Parse()

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	n, err := parser.IngestResumeChunks(client.GetEmbedding, *profilePath)
	if err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}
	log.Printf("Re-ingested %d career chunk(s) from %s.", n, *profilePath)
}
