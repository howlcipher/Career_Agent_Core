//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "applications.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, company_name, job_title, url, status FROM job_funnel WHERE status IN ('DISCOVERED', 'PROCESSING')")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var companyName, jobTitle, url, status sql.NullString
		err := rows.Scan(&id, &companyName, &jobTitle, &url, &status)
		if err != nil {
			log.Fatal(err)
		}
		
		isBad := false
		if !companyName.Valid || companyName.String == "" || companyName.String == "null" || companyName.String == "None" {
			isBad = true
		}
		if !jobTitle.Valid || jobTitle.String == "" || jobTitle.String == "null" || jobTitle.String == "None" {
			isBad = true
		}
		if !url.Valid || url.String == "" || !strings.HasPrefix(url.String, "http") || strings.Contains(url.String, "localhost") || strings.Contains(url.String, "127.0.0.1") {
			isBad = true
		}

		if isBad {
			fmt.Printf("BAD ID: %d | Company: '%s' | Title: '%s' | URL: '%s' | Status: '%s'\n", id, companyName.String, jobTitle.String, url.String, status.String)
		}
	}
}
