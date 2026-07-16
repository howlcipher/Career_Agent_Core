//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "./applications.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT count(*) FROM job_funnel WHERE status = 'FAILED_SUBMIT'").Scan(&count)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("FAILED_SUBMIT count: %d\n", count)

	err = db.QueryRow("SELECT count(*) FROM job_funnel WHERE status = 'FAILED_SCORE'").Scan(&count)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("FAILED_SCORE count: %d\n", count)

	rows, err := db.Query("SELECT id, company_name, job_title, url, status FROM job_funnel WHERE status IN ('FAILED_SUBMIT', 'FAILED_SCORE') LIMIT 20")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var comp, title, url, status string
		err = rows.Scan(&id, &comp, &title, &url, &status)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("[%s] ID: %d | %s | %s | %s\n", status, id, comp, title, url)
	}
}
