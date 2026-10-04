// luca-schema creates a database with the go-luca schema and sample data.
// Designed for use with documentation tools like tbls.
//
// Usage:
//
//	luca-schema [-db path | -pg dsn] [-sql]
//
// Flags:
//
//	-db   Path for a pglike (SQLite) database (default: /tmp/go-luca-schema.db)
//	-pg   PostgreSQL DSN instead; its public schema is dropped and recreated
//	-sql  Print the schema DDL to stdout instead of creating a database
//
// Schema documentation is generated from PostgreSQL, not from the pglike
// file: the contract views use ::numeric, which pglike evaluates with
// functions registered on its own connections, so another SQLite library
// (tbls, the sqlite3 shell) cannot compile those views.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dbPath := flag.String("db", "/tmp/go-luca-schema.db", "pglike (SQLite) database path")
	pgDSN := flag.String("pg", "", "PostgreSQL DSN; the public schema is dropped and recreated")
	sqlOnly := flag.Bool("sql", false, "print schema DDL to stdout")
	flag.Parse()

	if *sqlOnly {
		fmt.Print(luca.SchemaSQL)
		return
	}

	if *pgDSN != "" {
		if err := createPostgres(*pgDSN); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Schema created in PostgreSQL")
		return
	}

	// Remove existing file so we get a clean schema
	_ = os.Remove(*dbPath)

	db, err := luca.CreateSchemaDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	_ = db.Close()
	fmt.Printf("Schema database created at %s\n", *dbPath)
}

// createPostgres wipes the public schema of the database at dsn and
// recreates it with the go-luca schema and sample data.
func createPostgres(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		return fmt.Errorf("reset public schema: %w", err)
	}
	return luca.CreateSchema(db)
}
