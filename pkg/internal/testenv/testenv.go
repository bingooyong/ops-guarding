// Package testenv provides an isolated database + cache for tests that need
// the server-side models (task chain, scheduler, rpc).
//
// By default it uses an in-memory sqlite database (pure Go driver, no cgo),
// so `go test ./...` stays hermetic. Built with `-tags integration` and with
// OPS_MYSQL_DSN set, New uses a disposable MySQL database loaded from
// sql/ibex.sql instead, so the same tests run against the real schema.
package testenv

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"ops-guarding/pkg/appframe/cache"
	"ops-guarding/pkg/appframe/storage"
	"ops-guarding/pkg/models"
	"ops-guarding/pkg/server/config"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Env is a ready-to-use test environment. Package-level globals
// (storage.DB, cache.Instance, config.C, doing cache) are restored by Cleanup.
type Env struct {
	DB      *gorm.DB
	Backend string // "sqlite" or "mysql"
}

// Scheduler is the scheduler identity configured in config.C.Heartbeat.LocalAddr.
const Scheduler = "127.0.0.1:20090"

// BasicAuthUser / BasicAuthPass are the credentials accepted by the router in tests.
const (
	BasicAuthUser = "ibex"
	BasicAuthPass = "test-secret"
)

// mysqlFactory is set by mysql_env.go (build tag integration).
var mysqlFactory func(t testing.TB) (*gorm.DB, func(), bool)

var seq atomic.Int64

// New creates an isolated environment. It never touches a developer database.
func New(t testing.TB) *Env {
	t.Helper()

	var (
		db      *gorm.DB
		cleanup func()
		backend = "sqlite"
	)

	if mysqlFactory != nil {
		if mdb, mclean, ok := mysqlFactory(t); ok {
			db, cleanup, backend = mdb, mclean, "mysql"
		}
	}

	if db == nil {
		db, cleanup = newSQLite(t)
	}

	oldDB, oldCache, oldCfg := storage.DB, cache.Instance, config.C
	storage.DB = db
	cache.InitMemoryCache(time.Minute)

	cfg := new(config.Config)
	cfg.RunMode = gin.TestMode
	cfg.BasicAuth = gin.Accounts{BasicAuthUser: BasicAuthPass}
	cfg.Heartbeat.LocalAddr = Scheduler
	config.C = cfg

	models.SetDoingCache(nil)

	t.Cleanup(func() {
		storage.DB, cache.Instance, config.C = oldDB, oldCache, oldCfg
		models.SetDoingCache(nil)
		cleanup()
	})

	return &Env{DB: db, Backend: backend}
}

func newSQLite(t testing.TB) (*gorm.DB, func()) {
	t.Helper()

	dsn := fmt.Sprintf("file:testenv%d?mode=memory&cache=shared", seq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: quietLogger()})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	// One transaction: ~100 separate DDL commits make each New() 10x slower.
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range SQLiteSchema() {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("%w\n%s", err, stmt)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create sqlite schema: %v", err)
	}

	return db, func() { _ = sqlDB.Close() }
}

// quietLogger silences GORM's per-query logging ("record not found" is
// routine for the models' nil-on-missing lookups).
func quietLogger() gormlogger.Interface {
	return gormlogger.Default.LogMode(gormlogger.Silent)
}
