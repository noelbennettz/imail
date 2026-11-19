package http_server

import (
	"testing"
	"time"

	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
)

// insertQueueEntry writes one queue row for the logged-in outbox tests.
func (f *authFixture) insertQueueEntry(t *testing.T, state int8, reason string) int64 {
	t.Helper()
	now := time.Now().UTC()
	entry, err := queue.Entry(1, "msg-id@example.com", "sender@example.com", []string{"remote@example.net"}, now)
	if err != nil {
		t.Fatal(err)
	}
	entry.State = state
	if _, err := f.db.Insert(&entry); err != nil {
		t.Fatal(err)
	}
	if reason != "" {
		if _, err := f.db.Exec("update email_queue set last_error=? where id=?", reason, entry.Id); err != nil {
			t.Fatal(err)
		}
	}
	return entry.Id
}

func TestOutboxManagementIsAdminOnly(t *testing.T) {
	f := newAuthFixture(t)
	failedID := f.insertQueueEntry(t, models.EmailQueueFailed, "550 no such user")

	member := f.login(t, "member", "initial-password")
	for _, path := range []string{
		"/api/admin/outbox/list?state=failed",
		"/api/admin/outbox",
	} {
		if result := f.call(t, member, path, nil); result.ErrorNo != response.NoAccessPrivileges {
			t.Fatalf("%s for a member = %+v, want no access", path, result)
		}
	}
	for _, path := range []string{"/api/admin/outbox/retry", "/api/admin/outbox/delete"} {
		if result := f.call(t, member, path, map[string]any{"id": failedID}); result.ErrorNo != response.NoAccessPrivileges {
			t.Fatalf("%s for a member = %+v, want no access", path, result)
		}
	}
}

func TestOutboxAdminCanListRetryAndDelete(t *testing.T) {
	f := newAuthFixture(t)
	failedID := f.insertQueueEntry(t, models.EmailQueueFailed, "550 no such user")
	deliveredID := f.insertQueueEntry(t, models.EmailQueueDelivered, "")
	pendingID := f.insertQueueEntry(t, models.EmailQueuePending, "")

	admin := f.login(t, "admin", "initial-password")

	result := f.call(t, admin, "/api/admin/outbox/list?state=failed", nil)
	if result.ErrorNo != 0 {
		t.Fatalf("list failed: %+v", result)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["total"].(float64) != 1 {
		t.Fatalf("failed list = %+v, want one entry", result.Data)
	}
	entries := data["entries"].([]any)
	entry := entries[0].(map[string]any)
	if entry["last_error"] != "550 no such user" || entry["state"] != "failed" {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	if result := f.call(t, admin, "/api/admin/outbox/retry", map[string]any{"id": failedID}); result.ErrorNo != 0 {
		t.Fatalf("retry failed: %+v", result)
	}
	if result := f.call(t, admin, "/api/admin/outbox/list?state=pending", nil); result.ErrorNo != 0 {
		t.Fatalf("pending list failed: %+v", result)
	}

	if result := f.call(t, admin, "/api/admin/outbox/delete", map[string]any{"id": deliveredID}); result.ErrorNo != 0 {
		t.Fatalf("delete failed: %+v", result)
	}
	// A pending row is still owned by the worker and must not be deleted.
	if result := f.call(t, admin, "/api/admin/outbox/delete", map[string]any{"id": pendingID}); result.ErrorNo == 0 {
		t.Fatalf("deleting a pending row was accepted: %+v", result)
	}
}
