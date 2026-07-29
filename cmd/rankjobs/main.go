// cmd/rankjobs backfills job_funnel.fit_similarity for DISCOVERED rows
// (improvements.md #22), so GetDiscoveredJobs's ORDER BY can push jobs whose
// title/company most closely matches the resume toward the front of the
// queue, as a tie-break within each existing sourcePriorityCASE reachability
// tier (pkg/storage/manager.go). Embeds "<company> <title>" per job via the
// same GetEmbedding path cmd/agent already uses for RAG retrieval, then
// scores it against the resume's existing career_chunks with
// parser.BestSimilarity — no fresh resume ingestion needed as long as
// cmd/agent has run at least once (it seeds career_chunks from
// USER_PROFILE.md on first run).
//
// Deliberately a separate CLI, not folded into cmd/agent's own startup: this
// backfill shares the same local Ollama instance as any live cmd/agent run,
// so it needs its own bounded -limit rather than blocking or competing
// unboundedly with a run already in progress.
//
// Usage:
//
//	go run ./cmd/rankjobs                # backfill up to 200 missing rows (default)
//	go run ./cmd/rankjobs -limit 500      # backfill up to 500
//	go run ./cmd/rankjobs -limit 0        # backfill everything missing, no cap
package main

import (
	"flag"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
)

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	limit := flag.Int("limit", 200, "max rows to backfill this run (0 = unlimited); kept modest by default since embedding calls share the local Ollama instance a live cmd/agent run may also be using")
	flag.Parse()

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	chunks, err := storage.GetAllCareerChunks()
	if err != nil {
		log.Fatalf("Failed to load career chunks: %v", err)
	}
	if len(chunks) == 0 {
		log.Fatal("No career chunks found in the database — run cmd/agent at least once first so it can ingest USER_PROFILE.md into career_chunks.")
	}

	jobs, err := storage.GetJobsMissingFitSimilarity(*limit)
	if err != nil {
		log.Fatalf("Failed to query jobs missing fit_similarity: %v", err)
	}
	if len(jobs) == 0 {
		log.Println("Nothing to backfill — every DISCOVERED job already has a fit_similarity score.")
		return
	}
	log.Printf("Backfilling fit_similarity for %d job(s) against %d resume chunk(s)...", len(jobs), len(chunks))

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	var done, failed atomic.Int32
	var g errgroup.Group
	g.SetLimit(4) // Bounded concurrency for embeddings

	for _, j := range jobs {
		j := j
		g.Go(func() error {
			text := strings.TrimSpace(j.CompanyName + " " + j.JobTitle)
			if text == "" {
				failed.Add(1)
				return nil
			}

			var emb []float32
			var embErr error
			for attempt := 1; attempt <= 3; attempt++ {
				emb, embErr = client.GetEmbedding(text)
				if embErr == nil {
					break
				}
				if strings.Contains(embErr.Error(), "connect:") || strings.Contains(embErr.Error(), "no route to host") || strings.Contains(embErr.Error(), "429") || strings.Contains(embErr.Error(), "deadline exceeded") {
					log.Printf("Network or rate-limit error embedding %q (attempt %d/3). Sleeping 60s...", j.JobTitle, attempt)
					time.Sleep(60 * time.Second)
				} else {
					break
				}
			}
			if embErr != nil {
				log.Printf("Embedding failed for %s at %s: %v", j.JobTitle, j.URL, embErr)
				failed.Add(1)
				return nil
			}

			score := parser.BestSimilarity(emb, chunks)
			if err := storage.UpdateFitSimilarity(j.URL, score); err != nil {
				log.Printf("Failed to save fit_similarity for %s: %v", j.URL, err)
				failed.Add(1)
				return nil
			}
			
			done.Add(1)
			d := done.Load()
			if d%25 == 0 {
				log.Printf("Progress: %d/%d done", d, len(jobs))
			}
			return nil
		})
	}
	_ = g.Wait()
	log.Printf("Done. %d succeeded, %d failed.", done.Load(), failed.Load())
}
