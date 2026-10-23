//go:build integration

package testenv

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var dsnDBRe = regexp.MustCompile(`/([^/?]*)(\?|$)`)

func init() { mysqlFactory = newMySQL }

// newMySQL creates a unique database on the server given by OPS_MYSQL_DSN
// (its database part is only used to connect), loads sql/ibex.sql into it and
// drops it at cleanup. Returns ok=false (falling back to sqlite) when the
// variable is unset.
func newMySQL(t testing.TB) (*gorm.DB, func(), bool) {
	t.Helper()

	dsn := os.Getenv("OPS_MYSQL_DSN")
	if dsn == "" {
		return nil, nil, false
	}

	admin, err := sql.Open("mysql", dsnDBRe.ReplaceAllString(dsn, "/$2"))
	if err != nil {
		t.Fatalf("open mysql admin: %v", err)
	}

	name := fmt.Sprintf("og_test_%d_%d", time.Now().UnixNano()%1_000_000_000, seq.Add(1))
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatalf("create database: %v", err)
	}

	testDSN := dsnDBRe.ReplaceAllString(dsn, "/"+name+"$2")
	db, err := gorm.Open(mysql.Open(testDSN), &gorm.Config{Logger: quietLogger()})
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}

	script, err := os.ReadFile(filepath.Join(repoRoot(t), "sql", "ibex.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range splitSQL(string(script)) {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("load ibex.sql: %v\n%s", err, stmt)
		}
	}

	return db, func() {
		if sdb, err := db.DB(); err == nil {
			_ = sdb.Close()
		}
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`")
		_ = admin.Close()
	}, true
}

func repoRoot(t testing.TB) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "sql", "ibex.sql")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("sql/ibex.sql not found")
	return ""
}

// splitSQL drops the script's own database management statements (we use a
// unique database instead) and splits the rest on ';'.
func splitSQL(script string) []string {
	var out []string
	for _, s := range strings.Split(script, ";") {
		s = strings.TrimSpace(stripComments(s))
		l := strings.ToLower(s)
		if s == "" || strings.HasPrefix(l, "set names") || strings.HasPrefix(l, "drop database") ||
			strings.HasPrefix(l, "create database") || strings.HasPrefix(l, "use ") {
			continue
		}
		out = append(out, s)
	}
	return out
}

var blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

func stripComments(s string) string { return blockComment.ReplaceAllString(s, "") }
