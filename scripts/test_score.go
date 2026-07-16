//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("No API key")
	}

	c := mcp.NewClient(apiKey)

	scrapedData := map[string]string{
		"title": "Software Engineer",
		"desc":  "We are looking for a software engineer with Go and Python.",
	}
	constraints := map[string]interface{}{
		"remote_only":  true,
		"salary_floor": 100000,
	}

	score, err := c.ScoreJob(scrapedData, constraints, "I am a senior engineer with Go and Python.")
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("SCORE: %d\n", score)
	}
}
