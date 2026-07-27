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
//	CAREER_PROFILE_PATH=/path/to/USER_PROFILE.md go run ./cmd/reingest
package main

import (
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	profileFlag := flag.String(
		"profile",
		"",
		"path to career profile markdown (overrides CAREER_PROFILE_PATH)",
	)
	flag.Parse()

	_ = godotenv.Load()
	profilePath, err := config.ResolveCareerProfilePath(
		*profileFlag,
		os.Getenv(config.CareerProfilePathEnv),
		".",
	)
	if err != nil {
		log.Fatalf("Career profile configuration error: %v", err)
	}

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	n, err := parser.IngestResumeChunks(client.GetEmbedding, profilePath)
	if err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}
	if n == 0 {
		log.Fatal("Ingestion failed: no grounded career chunks were created")
	}
	log.Printf("Re-ingested %d career chunk(s) from %s.", n, profilePath)
}
