package data_processor

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/marcboeker/go-duckdb"
)

type DuckDB struct {
	DB *sql.DB
}

// NewDuckDB creates a new DuckDB connection.
// path: Path to the database file. If empty, uses in-memory database.
func NewDuckDB(path string) (*DuckDB, error) {
	connStr := path
	if connStr == "" {
		connStr = "" // Empty string or "?cache=shared" for in-memory
	}

	db, err := sql.Open("duckdb", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping duckdb: %w", err)
	}

	log.Printf("🦆 DuckDB Connected: %s", connStr)
	return &DuckDB{DB: db}, nil
}

func (d *DuckDB) Close() error {
	return d.DB.Close()
}
