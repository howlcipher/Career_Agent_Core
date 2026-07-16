//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "./applications.db")
	if err != nil {
		fmt.Println("Error opening db:", err)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT status, count(*) FROM job_funnel WHERE status IN ('FAILED_SCORE', 'FAILED_SUBMIT') GROUP BY status;")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		fmt.Printf("%s: %d\n", status, count)
	}
}
