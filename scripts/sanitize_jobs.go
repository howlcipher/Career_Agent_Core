//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "applications.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 1. Identify and remove/flag bad data in DISCOVERED and PROCESSING statuses
	rows, err := db.Query("SELECT id, ifnull(company_name, ''), ifnull(job_title, ''), ifnull(url, '') FROM job_funnel WHERE status IN ('DISCOVERED', 'PROCESSING')")
	if err != nil {
		log.Fatal(err)
	}
	
	var badJobIDs []int
	for rows.Next() {
		var id int
		var companyName, jobTitle, jobURL string
		if err := rows.Scan(&id, &companyName, &jobTitle, &jobURL); err != nil {
			log.Fatal(err)
		}

		isBad := false
		if strings.TrimSpace(companyName) == "" || strings.TrimSpace(companyName) == "null" || strings.TrimSpace(companyName) == "None" {
			isBad = true
		}
		if strings.TrimSpace(jobTitle) == "" || strings.TrimSpace(jobTitle) == "null" || strings.TrimSpace(jobTitle) == "None" {
			isBad = true
		}

		u, err := url.Parse(strings.TrimSpace(jobURL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			isBad = true
		} else {
			host := u.Hostname()
			if host == "localhost" || host == "127.0.0.1" {
				isBad = true
			}
		}

		if isBad {
			badJobIDs = append(badJobIDs, id)
		}
	}
	rows.Close()

	if len(badJobIDs) > 0 {
		fmt.Printf("Found %d jobs with bad data. Deleting them...\n", len(badJobIDs))
		for _, id := range badJobIDs {
			_, err = db.Exec("DELETE FROM job_funnel WHERE id = ?", id)
			if err != nil {
				log.Printf("Failed to delete job %d: %v", id, err)
			}
		}
	} else {
		fmt.Println("No bad data found in the queue.")
	}

	// 2. Reset jobs stuck in PROCESSING back to DISCOVERED
	res, err := db.Exec("UPDATE job_funnel SET status = 'DISCOVERED' WHERE status = 'PROCESSING'")
	if err != nil {
		log.Fatal(err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Successfully reset %d orphaned jobs from PROCESSING to DISCOVERED.\n", rowsAffected)
}
