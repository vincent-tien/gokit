// Package testutil provides test infrastructure helpers for database testing.
// Dialect-aware: works with both PostgreSQL and MySQL/MariaDB.
package testutil

import (
	"os"
	"strings"
)

// Dialect represents a database engine for SQL behavior differences.
type Dialect int

const (
	Postgres Dialect = iota // default
	MySQL                   // also covers MariaDB
)

// DialectFromEnv reads GOKIT_DIALECT env var.
// Values: "postgres" (default), "mysql", "mariadb".
func DialectFromEnv() Dialect {
	switch strings.ToLower(os.Getenv("GOKIT_DIALECT")) {
	case "mysql", "mariadb":
		return MySQL
	default:
		return Postgres
	}
}

// Driver returns the database/sql driver name for this dialect.
func (d Dialect) Driver() string {
	if d == MySQL {
		return "mysql"
	}
	return "pgx"
}

// DefaultDSN returns a localhost DSN suitable for local development/CI.
func (d Dialect) DefaultDSN() string {
	if d == MySQL {
		return "gokit:gokit@tcp(localhost:3306)/gokit_test?parseTime=true&loc=UTC"
	}
	return "postgres://gokit:gokit@localhost:5432/gokit_test?sslmode=disable"
}

// String returns a human-readable dialect name.
func (d Dialect) String() string {
	if d == MySQL {
		return "mysql"
	}
	return "postgres"
}
