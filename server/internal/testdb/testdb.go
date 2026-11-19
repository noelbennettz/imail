// Package testdb opens the database the package tests run against.
//
// The default is a throwaway SQLite file, which is what a developer has. The CI
// database matrix sets IMAIL_TEST_DB=mysql or postgres and IMAIL_TEST_DSN to a
// service container, so the same tests run against the two server databases the
// project supports. On a server backend the tables are emptied when a test
// opens them, because there is no cheap way to give each test its own database.
package testdb

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

// synced remembers which databases already have the test schema. The server
// backends are shared by every test in the package, and xorm's Sync2 tries to
// recreate an index that already exists (MySQL answers duplicate key name), so
// the schema is created once per process and later tests only empty it.
var synced = struct {
	sync.Mutex
	seen map[string]bool
}{seen: map[string]bool{}}

// Backend returns the configured test backend and DSN. An empty backend means
// SQLite at the path the caller passes in.
func Backend() (string, string) {
	backend := os.Getenv("IMAIL_TEST_DB")
	switch backend {
	case "mysql", "postgres":
		return backend, os.Getenv("IMAIL_TEST_DSN")
	default:
		return "", ""
	}
}

// Open returns an engine for the test. On a server backend the tables of the
// given models are emptied first; on SQLite the path is opened (and created) as
// before.
func Open(t *testing.T, sqlitePath string, models ...any) *xorm.Engine {
	t.Helper()
	return open(t, sqlitePath, true, models...)
}

// Reopen opens the same database again without emptying it. It is what a
// restart test needs after closing the engine.
func Reopen(t *testing.T, sqlitePath string, models ...any) *xorm.Engine {
	t.Helper()
	return open(t, sqlitePath, false, models...)
}

func open(t *testing.T, sqlitePath string, reset bool, models ...any) *xorm.Engine {
	t.Helper()
	backend, dsn := Backend()
	if backend == "" {
		backend, dsn = "sqlite", sqlitePath
	}
	if dsn == "" {
		t.Fatalf("IMAIL_TEST_DB=%s but IMAIL_TEST_DSN is empty", backend)
	}
	engine, err := xorm.NewEngine(backend, dsn)
	if err != nil {
		t.Fatalf("open %s test database: %v", backend, err)
	}
	engine.SetMaxOpenConns(1)
	engine.SetMaxIdleConns(1)
	// xorm converts values with DatabaseTZ on the way in and TZLocation on the
	// way out. The server pins time.Local to UTC (config.Init), so the tests do
	// it too and set both sides explicitly: an engine built while the process
	// was still on the local zone would otherwise round-trip in that zone.
	engine.DatabaseTZ = time.UTC
	engine.TZLocation = time.UTC
	if err := syncOnce(backend, dsn, engine, models...); err != nil {
		_ = engine.Close()
		t.Fatalf("sync %s test schema: %v", backend, err)
	}
	if reset {
		for _, model := range models {
			table := engine.TableName(model)
			if table == "" {
				t.Fatalf("cannot resolve the table name of %T", model)
			}
			if _, err := engine.Exec(fmt.Sprintf("delete from %s", engine.Quote(table))); err != nil {
				_ = engine.Close()
				t.Fatalf("empty %s in the %s test database: %v", table, backend, err)
			}
		}
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

// syncOnce creates the schema for the first test of this process and leaves the
// tables alone afterwards.
func syncOnce(backend, dsn string, engine *xorm.Engine, models ...any) error {
	if backend == "sqlite" {
		return engine.Sync2(models...)
	}
	synced.Lock()
	defer synced.Unlock()
	key := backend + " " + dsn
	if synced.seen[key] {
		return nil
	}
	// Rebuild the schema instead of migrating it: a leftover table from an
	// interrupted run, or from another package that used the same database,
	// makes xorm try to add an index that already exists. The CI matrix runs the
	// packages one at a time (go test -p 1), so nothing else is using the
	// database while it is rebuilt.
	if err := engine.DropTables(models...); err != nil {
		return err
	}
	if err := engine.Sync2(models...); err != nil {
		return err
	}
	synced.seen[key] = true
	return nil
}

// IsSQLite reports whether the tests run against SQLite. Tests that depend on
// SQLite-specific SQL (triggers, for example) use it to skip on a server
// backend.
func IsSQLite() bool {
	backend, _ := Backend()
	return backend == ""
}
