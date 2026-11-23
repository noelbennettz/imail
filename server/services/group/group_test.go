package group

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto"
	"github.com/noelbennettz/imail/models"
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

func TestUnreadForTag(t *testing.T) {
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

	rows := []models.UserEmail{
		{UserID: 1, EmailID: 1, IsRead: 0, GroupId: 0, Status: 0},  // inbox, unread
		{UserID: 1, EmailID: 2, IsRead: 1, GroupId: 0, Status: 0},  // inbox, read
		{UserID: 1, EmailID: 3, IsRead: 0, GroupId: 0, Status: 5},  // junk, unread
		{UserID: 2, EmailID: 4, IsRead: 0, GroupId: 0, Status: 0},  // another user
		{UserID: 1, EmailID: 5, IsRead: 0, GroupId: 77, Status: 0}, // custom, unread
		{UserID: 2, EmailID: 6, IsRead: 0, GroupId: 77, Status: 0},
	}
	for i := range rows {
		if _, err := db.Instance.Insert(&rows[i]); err != nil {
			t.Fatalf("insert user_email: %v", err)
		}
	}

	ctx := &context.Context{UserID: 1, UserName: "u", UserAccount: "u"}
	cases := []struct {
		name string
		tag  dto.SearchTag
		want int
	}{
		{"inbox", dto.SearchTag{Type: 0, Status: -1}, 1},
		{"junk", dto.SearchTag{Type: -1, Status: 5}, 1},
		{"custom", dto.SearchTag{GroupId: 77, Status: -1, Type: -1}, 1},
		{"sent has no badge", dto.SearchTag{Type: 1, Status: -1}, 0},
		{"drafts have no badge", dto.SearchTag{Type: 0, Status: 4}, 0},
	}
	for _, tc := range cases {
		if got := UnreadForTag(ctx, tc.tag); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
