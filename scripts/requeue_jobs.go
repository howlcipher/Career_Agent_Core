//go:build ignore

package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// 1. Parse log file for unsupported ATS URLs
	file, err := os.Open("career_agent.log")
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	defer file.Close()

	unsupportedURLs := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "unsupported Applicant Tracking System at ") {
			parts := strings.Split(line, "unsupported Applicant Tracking System at ")
			if len(parts) == 2 {
				url := strings.TrimSpace(parts[1])
				unsupportedURLs[url] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("error reading log file: %v", err)
	}

	fmt.Printf("Found %d unique unsupported URLs in logs.\n", len(unsupportedURLs))

	// 2. Connect to database
	db, err := sql.Open("sqlite3", "./applications.db")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// 3. Delete garbage Yahoo URLs
	res, err := db.Exec("DELETE FROM job_funnel WHERE url LIKE '%yahoo.com%'")
	if err != nil {
		log.Fatalf("failed to delete garbage URLs: %v", err)
	}
	deletedCount, _ := res.RowsAffected()
	fmt.Printf("Deleted %d garbage Yahoo URLs.\n", deletedCount)

	// 4. Requeue valid false positives
	// We will first get all FAILED_SUBMIT and FAILED_SCORE jobs that aren't unsupported
	rows, err := db.Query("SELECT id, url, status FROM job_funnel WHERE status IN ('FAILED_SUBMIT', 'FAILED_SCORE')")
	if err != nil {
		log.Fatalf("failed to query failed jobs: %v", err)
	}
	
	var toRequeue []int
	var trueFailures []int

	for rows.Next() {
		var id int
		var url, status string
		if err := rows.Scan(&id, &url, &status); err != nil {
			log.Fatalf("error scanning row: %v", err)
		}
		
		if unsupportedURLs[url] {
			trueFailures = append(trueFailures, id)
		} else {
			toRequeue = append(toRequeue, id)
		}
	}
	rows.Close()

	// Perform the requeue updates
	if len(toRequeue) > 0 {
		// Create placeholders for the query
		placeholders := make([]string, len(toRequeue))
		args := make([]interface{}, len(toRequeue))
		for i, id := range toRequeue {
			placeholders[i] = "?"
			args[i] = id
		}
		
		query := fmt.Sprintf("UPDATE job_funnel SET status = 'DISCOVERED' WHERE id IN (%s)", strings.Join(placeholders, ","))
		res, err = db.Exec(query, args...)
		if err != nil {
			log.Fatalf("failed to requeue jobs: %v", err)
		}
		requeuedCount, _ := res.RowsAffected()
		fmt.Printf("Successfully requeued %d jobs back to DISCOVERED.\n", requeuedCount)
	} else {
		fmt.Println("No jobs to requeue.")
	}

	fmt.Printf("Left %d jobs as FAILED_SUBMIT (True Negatives - Unsupported ATS).\n", len(trueFailures))
}
