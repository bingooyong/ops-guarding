//go:build integration

package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The MySQL loader must run every CREATE TABLE of sql/ibex.sql and skip the
// script's own database management statements. Needs no MySQL server.
func TestSplitSQLOfIbex(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "sql", "ibex.sql"))
	if err != nil {
		t.Fatal(err)
	}
	stmts := splitSQL(string(b))
	if len(stmts) != 5+taskHostShards {
		t.Fatalf("got %d statements, want %d", len(stmts), 5+taskHostShards)
	}
	for _, s := range stmts {
		if !strings.HasPrefix(strings.ToUpper(s), "CREATE TABLE") {
			t.Fatalf("unexpected statement: %.60s", s)
		}
	}
}
