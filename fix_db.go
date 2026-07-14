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
		log.Fatalf("Failed to open db: %v", err)
	}
	defer db.Close()

	res, err := db.Exec("UPDATE job_funnel SET status = 'DISCOVERED' WHERE status = 'FAILED_SCORE'")
	if err != nil {
		log.Fatalf("Failed to update: %v", err)
	}

	rows, _ := res.RowsAffected()
	fmt.Printf("Successfully reset %d jobs from FAILED_SCORE back to DISCOVERED.\n", rows)
}
