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

	rows, err := db.Query("SELECT url FROM job_funnel WHERE status = 'FAILED_SCORE' LIMIT 5")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("FAILED_SCORE URLs:")
	for rows.Next() {
		var url string
		err = rows.Scan(&url)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(url)
	}
}
