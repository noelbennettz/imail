// Package quota enforces per-user mailbox capacity.
package quota

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
)

// ErrExceeded is returned when storing a message would exceed a mailbox quota.
var ErrExceeded = errors.New("mailbox quota exceeded")

// UsedBytes returns the storage currently owned by a user. Messages in the
// trash (status 3) are excluded so emptying the trash frees capacity.
func UsedBytes(userID int) (int64, error) {
	rows, err := db.Instance.QueryString(
		`SELECT COALESCE(SUM(e.size), 0) AS used
		 FROM email e INNER JOIN user_email ue ON ue.email_id = e.id
		 WHERE ue.user_id = ? AND ue.status <> ?`, userID, models.DeletedStatus)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	used, err := strconv.ParseInt(rows[0]["used"], 10, 64)
	if err != nil {
		return 0, err
	}
	return used, nil
}

// Check verifies every recipient can accept size additional bytes.
// A quota of 0 means unlimited. The returned error wraps ErrExceeded so
// callers can map it to an SMTP 552 response.
func Check(users []*models.User, size int) error {
	if size < 0 {
		size = 0
	}
	for _, user := range users {
		if user == nil || user.Quota <= 0 {
			continue
		}
		used, err := UsedBytes(user.ID)
		if err != nil {
			return err
		}
		if used+int64(size) > user.Quota {
			return fmt.Errorf("%w: %s (%d/%d bytes)", ErrExceeded, user.Account, used, user.Quota)
		}
	}
	return nil
}
