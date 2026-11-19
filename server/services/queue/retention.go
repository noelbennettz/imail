package queue

import (
	"time"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
)

// Retention bounds how long a finished queue row is kept. Delivered rows are
// only history and are dropped quickly; failed rows stay long enough for an
// administrator to read the error before deleting them. Both are variables so a
// deployment (and the tests) can shorten them.
var (
	// DeliveredRetention keeps a delivered row for seven days.
	DeliveredRetention = 7 * 24 * time.Hour
	// FailedRetention keeps a finally failed row for thirty days.
	FailedRetention = 30 * 24 * time.Hour
)

// Purge deletes finished queue rows older than the retention window. Pending,
// delivering and leased rows are never touched: they are the messages still to
// be delivered. It returns how many rows of each state were removed.
func Purge(now time.Time) (delivered, failed int64, err error) {
	now = now.UTC()
	delivered, err = deleteOlderThan(models.EmailQueueDelivered, now.Add(-DeliveredRetention))
	if err != nil {
		return delivered, 0, err
	}
	failed, err = deleteOlderThan(models.EmailQueueFailed, now.Add(-FailedRetention))
	return delivered, failed, err
}

func deleteOlderThan(state int8, cutoff time.Time) (int64, error) {
	result, err := db.Instance.Exec(
		db.WithContext(nil, "delete from email_queue where state=? and update_time < ?"),
		state, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}
