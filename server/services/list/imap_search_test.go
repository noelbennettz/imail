package list

import (
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/dto"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
)

func TestHasFlagUsesStoredStatus(t *testing.T) {
	item := &response.UserEmailUIDData{UserEmail: models.UserEmail{Status: 3}}
	if !hasFlag(item, imap.FlagDeleted) {
		t.Fatal("\\Deleted did not match the deleted status")
	}
	if hasFlag(item, imap.FlagDraft) || hasFlag(item, imap.FlagJunk) {
		t.Fatal("a deleted message matched an unrelated flag")
	}
	item.Status = 4
	if !hasFlag(item, imap.FlagDraft) || hasFlag(item, imap.FlagDeleted) {
		t.Fatal("draft flag did not follow the stored status")
	}
	item.Status = 5
	if !hasFlag(item, imap.FlagJunk) {
		t.Fatal("junk flag did not follow the stored status")
	}
}

func TestCountQueryHasNoOrderOrLimit(t *testing.T) {
	ctx := &context.Context{UserID: 1}
	sql, _ := genSQL(ctx, true, dto.SearchTag{Type: -1, Status: -1, GroupId: -1}, "", false, 0, 15)
	if strings.Contains(strings.ToLower(sql), "order by") || strings.Contains(strings.ToLower(sql), " limit ") {
		t.Fatalf("count query contains pagination: %q", sql)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(sql)), "select count(1)") {
		t.Fatalf("count query = %q", sql)
	}
}

func TestUserEmailRowsSelectStatus(t *testing.T) {
	// hasFlag reads status, so the IMAP list query must select it.
	sql, _, ok := userEmailRowsSQL(&context.Context{}, "INBOX", true, ImapListReq{Star: 1})
	if !ok || !strings.Contains(sql, "status") {
		t.Fatalf("query does not select status: %q", sql)
	}
}
