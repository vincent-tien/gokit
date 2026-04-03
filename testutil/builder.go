package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// Row represents a database row to insert via fluent builder.
type Row struct {
	table   string
	columns []string
	values  []any
}

// InsertRow starts building a row for the given table.
//
//	testutil.InsertRow("items").
//	    Set("id", testutil.UUID(1)).
//	    Set("title", "Test Item").
//	    Exec(t, db)
func InsertRow(table string) *Row {
	return &Row{table: table}
}

// Set adds a column-value pair to the row.
func (r *Row) Set(column string, value any) *Row {
	r.columns = append(r.columns, column)
	r.values = append(r.values, value)
	return r
}

// Exec inserts the row into the database. Fails the test on error.
// Uses ? placeholders — works with both PostgreSQL (via pgx/stdlib) and MySQL.
func (r *Row) Exec(t testing.TB, db *sql.DB) {
	t.Helper()
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		r.table,
		strings.Join(r.columns, ", "),
		questionMarks(len(r.columns)),
	)
	if _, err := db.ExecContext(context.Background(), query, r.values...); err != nil {
		t.Fatalf("InsertRow(%s): %v", r.table, err)
	}
}

// InsertRows inserts multiple rows from a slice of maps.
// All maps must have the same keys. Column order is sorted alphabetically
// for deterministic SQL generation.
func InsertRows(t testing.TB, db *sql.DB, table string, rows []map[string]any) {
	t.Helper()
	if len(rows) == 0 {
		return
	}

	// Extract and sort columns from first row for deterministic ordering.
	columns := make([]string, 0, len(rows[0]))
	for col := range rows[0] {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		questionMarks(len(columns)),
	)

	for _, row := range rows {
		values := make([]any, len(columns))
		for i, col := range columns {
			values[i] = row[col]
		}
		if _, err := db.ExecContext(context.Background(), query, values...); err != nil {
			t.Fatalf("InsertRows(%s): %v", table, err)
		}
	}
}

func questionMarks(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}
