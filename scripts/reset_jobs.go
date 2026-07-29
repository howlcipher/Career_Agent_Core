//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "./applications.db")
	if err != nil {
		fmt.Println("Error opening db:", err)
		return
	}
	defer db.Close()

	res, err := db.Exec("UPDATE job_funnel SET status = 'DISCOVERED' WHERE status = 'FAILED_SCORE'")
	if err != nil {
		fmt.Println("Error updating:", err)
		return
	}
	rows, _ := res.RowsAffected()
	fmt.Printf("Successfully reset %d jobs back to DISCOVERED!\n", rows)
}
