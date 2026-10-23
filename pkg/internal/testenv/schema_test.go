package testenv

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	createRe = regexp.MustCompile("(?is)CREATE TABLE\\s+`?(\\w+)`?\\s*\\((.*?)\\)\\s*ENGINE")
	colRe    = regexp.MustCompile("^`?(\\w+)`?\\s+\\w+")
)

// ibexColumns parses table -> column names from sql/ibex.sql.
func ibexColumns(t *testing.T) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "sql", "ibex.sql"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, m := range createRe.FindAllStringSubmatch(string(b), -1) {
		var cols []string
		for _, line := range strings.Split(m[2], "\n") {
			line = strings.TrimSpace(line)
			upper := strings.ToUpper(line)
			if line == "" || strings.HasPrefix(upper, "PRIMARY") || strings.HasPrefix(upper, "UNIQUE") || strings.HasPrefix(upper, "KEY") {
				continue
			}
			if c := colRe.FindStringSubmatch(line); c != nil {
				cols = append(cols, c[1])
			}
		}
		sort.Strings(cols)
		out[m[1]] = cols
	}
	return out
}

func TestSchemaMatchesIbexSQL(t *testing.T) {
	want := ibexColumns(t)
	if len(want) != 5+taskHostShards {
		t.Fatalf("parsed %d tables from ibex.sql, want %d", len(want), 5+taskHostShards)
	}

	// Always inspect the sqlite DDL, even when OPS_MYSQL_DSN selects MySQL for New().
	db, cleanup := newSQLite(t)
	defer cleanup()
	for table, cols := range want {
		var got []string
		rows, err := db.Raw("SELECT name FROM pragma_table_info(?)", table).Rows()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			got = append(got, n)
		}
		rows.Close()
		sort.Strings(got)
		if !reflect.DeepEqual(got, cols) {
			t.Errorf("table %s: sqlite columns %v, ibex.sql columns %v", table, got, cols)
		}
	}
}
