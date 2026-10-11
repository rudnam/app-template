// Package testdb gives integration tests a migrated, empty PostgreSQL database.
package testdb

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/francis/app-template/internal/db"
)

// Open connects to TEST_DATABASE_URL, applies the migrations and empties
// every table. Without TEST_DATABASE_URL the test is skipped; `./verify.sh go`
// sets it. The database is wiped, so never point it at real data.
//
// Packages share the one database, so run tests with `go test -p 1`.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; ./verify.sh go runs this test")
	}
	ctx := context.Background()
	conn, err := db.Open(ctx, url, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}

	rows, err := conn.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, pq.QuoteIdentifier(name))
	}
	rows.Close()
	if len(tables) > 0 {
		if _, err := conn.ExecContext(ctx, "TRUNCATE "+strings.Join(tables, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
			t.Fatal(err)
		}
	}
	return conn
}
