//go:build integration

// Integration tests against a real MySQL loaded with sql/ibex.sql.
//
//	Run: OPS_MYSQL_DSN='user:pass@tcp(127.0.0.1:3306)/ibex?charset=utf8mb4&parseTime=True' \
//	     go test -tags integration ./pkg/models/ -run Integration
package models

import (
	"errors"
	"os"
	"testing"

	"ops-guarding/pkg/appframe/storage"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const itTaskID int64 = 990000001 // task_host_1

func openIT(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("OPS_MYSQL_DSN")
	if dsn == "" {
		t.Skip("OPS_MYSQL_DSN not set")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	return db
}

func seedIT(t *testing.T, db *gorm.DB) {
	t.Helper()
	cleanIT(db)
	if err := db.Exec("INSERT INTO "+tht(itTaskID)+" (id, host, status) VALUES (?, ?, 'running')", itTaskID, "it-host").Error; err != nil {
		t.Fatalf("seed host: %v", err)
	}
	if err := db.Create(&TaskHostDoing{Id: itTaskID, Host: "it-host", Clock: 100, Action: "start"}).Error; err != nil {
		t.Fatalf("seed doing: %v", err)
	}
	t.Cleanup(func() { cleanIT(db) })
}

func cleanIT(db *gorm.DB) {
	db.Exec("DELETE FROM "+tht(itTaskID)+" WHERE id=?", itTaskID)
	db.Exec("DELETE FROM task_host_doing WHERE id=?", itTaskID)
}

func hostStatusIT(t *testing.T, db *gorm.DB) string {
	var s string
	db.Raw("SELECT status FROM "+tht(itTaskID)+" WHERE id=? AND host=?", itTaskID, "it-host").Scan(&s)
	return s
}

func TestIntegrationMarkDoneStatusCommits(t *testing.T) {
	db := openIT(t)
	old := storage.DB
	storage.DB = db
	t.Cleanup(func() { storage.DB = old })
	seedIT(t, db)

	if err := MarkDoneStatus(itTaskID, 100, "it-host", "success", "o", "e"); err != nil {
		t.Fatal(err)
	}
	if s := hostStatusIT(t, db); s != "success" {
		t.Fatalf("status = %q", s)
	}
	if n, _ := DoingHostCount("id=?", itTaskID); n != 0 {
		t.Fatalf("doing rows = %d", n)
	}
}

// The doing-row delete fails, so the host row update must roll back too.
func TestIntegrationMarkDoneStatusRollsBack(t *testing.T) {
	db := openIT(t)
	seedIT(t, db)

	failing := openIT(t)
	_ = failing.Callback().Delete().Before("gorm:delete").Register("it:fail", func(d *gorm.DB) {
		_ = d.AddError(errors.New("boom"))
	})
	old := storage.DB
	storage.DB = failing
	t.Cleanup(func() { storage.DB = old })

	if err := MarkDoneStatus(itTaskID, 100, "it-host", "success", "o", "e"); err == nil {
		t.Fatal("expected error")
	}
	if s := hostStatusIT(t, db); s != "running" {
		t.Fatalf("status = %q, want running (rolled back)", s)
	}
}
