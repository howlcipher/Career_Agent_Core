package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "applications.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	tablesRows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table';")
	if err != nil {
		log.Fatal(err)
	}
	defer tablesRows.Close()

	for tablesRows.Next() {
		var tableName string
		tablesRows.Scan(&tableName)
		fmt.Printf("\nTable: %s\n", tableName)
		rows, _ := db.Query(fmt.Sprintf("PRAGMA table_info(%s);", tableName))
		for rows.Next() {
			var cid int
			var name, typeName string
			var notnull int
			var dfltValue *string
			var pk int
			rows.Scan(&cid, &name, &typeName, &notnull, &dfltValue, &pk)
			fmt.Printf("- %s %s\n", name, typeName)
		}
		rows.Close()
	}
}
