package setup

import (
	"path/filepath"
	"testing"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/password"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestSetAdminPasswordIsIdempotent(t *testing.T) {
	engine, err := xorm.NewEngine("sqlite", filepath.Join(t.TempDir(), "setup.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Sync2(&models.User{}); err != nil {
		t.Fatal(err)
	}
	old := db.Instance
	db.Instance = engine
	t.Cleanup(func() {
		db.Instance = old
		engine.Close()
	})

	if err := SetAdminPassword(nil, "admin", "first-password"); err != nil {
		t.Fatal(err)
	}
	if err := SetAdminPassword(nil, "admin", "second-password"); err != nil {
		t.Fatal(err)
	}

	var users []models.User
	if err := engine.Find(&users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want one administrator after a retried step", len(users))
	}
	if ok, _ := password.Verify("second-password", users[0].Password); !ok {
		t.Fatal("the retried setup did not update the administrator password")
	}
}
