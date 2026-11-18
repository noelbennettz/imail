package group

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/utils/context"
)

func TestGetGroupStatus(t *testing.T) {
	oldConfig := config.Instance
	config.Instance = &config.Config{
		DbType: config.DBTypeSQLite,
		DbDSN:  filepath.Join(t.TempDir(), "test.db"),
	}
	t.Cleanup(func() { config.Instance = oldConfig })

	if err := db.Init("test"); err != nil {
		t.Fatalf("init database: %v", err)
	}
	db.Instance.ShowSQL(false)
	ctx := &context.Context{
		UserID:      1,
		UserName:    "admin",
		UserAccount: "admin",
	}
	ret, _ := GetGroupStatus(ctx, "INBOX", []string{"MESSAGES", "UIDNEXT", "UIDVALIDITY", "UNSEEN"})
	fmt.Println(ret)
}
