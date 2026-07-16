package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Job struct {
	ID          int
	CompanyName string
	JobTitle    string
	URL         string
	Status      string
}

func main() {
	db, err := sql.Open("sqlite3", "applications.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, ifnull(company_name, ''), ifnull(job_title, ''), ifnull(url, ''), status FROM job_funnel WHERE status IN ('DISCOVERED', 'PROCESSING');")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var discoveredCount, processingCount int
	var badJobs []Job

	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.CompanyName, &j.JobTitle, &j.URL, &j.Status); err != nil {
			log.Fatal(err)
		}

		if j.Status == "DISCOVERED" {
			discoveredCount++
		} else if j.Status == "PROCESSING" {
			processingCount++
		}

		isBad := false
		if strings.TrimSpace(j.CompanyName) == "" {
			isBad = true
		}
		if strings.TrimSpace(j.JobTitle) == "" {
			isBad = true
		}
		
		u, err := url.Parse(j.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			isBad = true
		} else {
			host := u.Hostname()
			if host == "localhost" || host == "127.0.0.1" {
				isBad = true
			}
		}

		if isBad {
			badJobs = append(badJobs, j)
		}
	}

	fmt.Printf("Total DISCOVERED: %d\n", discoveredCount)
	fmt.Printf("Total PROCESSING: %d\n", processingCount)
	fmt.Printf("Total Bad Jobs: %d\n", len(badJobs))

	for i, j := range badJobs {
		if i < 10 {
			fmt.Printf("Bad Job - ID: %d, Company: %s, Title: %s, URL: %s, Status: %s\n", j.ID, j.CompanyName, j.JobTitle, j.URL, j.Status)
		}
	}
}
