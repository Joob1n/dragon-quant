package data_processor

import (
	"testing"
)

func TestDuckDBConnection(t *testing.T) {
	// 1. Connect (In-Memory)
	duck, err := NewDuckDB("")
	if err != nil {
		t.Fatalf("Failed to connect to DuckDB: %v", err)
	}
	defer duck.Close()

	// 2. Create Table
	_, err = duck.DB.Exec("CREATE TABLE test_table (id INTEGER, name VARCHAR)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// 3. Insert Data
	_, err = duck.DB.Exec("INSERT INTO test_table VALUES (1, 'Dragon'), (2, 'Quant')")
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// 4. Query Data
	rows, err := duck.DB.Query("SELECT name FROM test_table ORDER BY id")
	if err != nil {
		t.Fatalf("Failed to query data: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("Failed to scan row: %v", err)
		}
		names = append(names, name)
	}

	// 5. Verify
	if len(names) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(names))
	}
	if names[0] != "Dragon" || names[1] != "Quant" {
		t.Errorf("Unexpected data: %v", names)
	}

	t.Logf("✅ DuckDB Test Passed! Data: %v", names)
}
