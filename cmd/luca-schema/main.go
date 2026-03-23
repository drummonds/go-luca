// luca-schema creates an SQLite database with the go-luca schema and sample
// data. Designed for use with documentation tools like tbls.
//
// Usage:
//
//	luca-schema [-db path] [-sql]
//
// Flags:
//
//	-db   Path for the SQLite database (default: /tmp/go-luca-schema.db)
//	-sql  Print the schema DDL to stdout instead of creating a database
package main

import (
	"flag"
	"fmt"
	"os"

	luca "codeberg.org/hum3/go-luca"
)

func main() {
	dbPath := flag.String("db", "/tmp/go-luca-schema.db", "SQLite database path")
	sqlOnly := flag.Bool("sql", false, "print schema DDL to stdout")
	flag.Parse()

	if *sqlOnly {
		fmt.Print(luca.SchemaSQL)
		return
	}

	// Remove existing file so we get a clean schema
	os.Remove(*dbPath)

	db, err := luca.CreateSchemaDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	db.Close()
	fmt.Printf("Schema database created at %s\n", *dbPath)
}
