package queue

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"xorm.io/xorm"
)

// Errors the administrator API maps to a response. They are separate so the UI
// can tell "that row is gone" from "that row cannot be retried or deleted".
var (
	// ErrEntryNotFound means no queue row has that id.
	ErrEntryNotFound = errors.New("outbox entry not found")
	// ErrEntryState means the row exists but is not in a state the action
	// allows.
	ErrEntryState = errors.New("outbox entry is in the wrong state")
)

const (
	// DefaultPageSize is how many rows one page of the administrator outbox
	// list holds.
	DefaultPageSize = 20
	// MaxPageSize caps what a request may ask for.
	MaxPageSize = 100
)

// EntryView is one queue row as the administrator UI shows it: the delivery
// history (attempts, last error, timestamps) plus the envelope it belongs to.
type EntryView struct {
	ID           int64     `json:"id"`
	EmailID      int       `json:"email_id"`
	MsgID        string    `json:"msg_id"`
	EnvelopeFrom string    `json:"envelope_from"`
	Recipients   []string  `json:"recipients"`
	State        string    `json:"state"`
	StateCode    int8      `json:"state_code"`
	Attempts     int       `json:"attempts"`
	NextAttempt  time.Time `json:"next_attempt"`
	LastError    string    `json:"last_error"`
	CreateTime   time.Time `json:"create_time"`
	UpdateTime   time.Time `json:"update_time"`
}

// StateName renders a queue state for logs and the API.
func StateName(state int8) string {
	switch state {
	case models.EmailQueuePending:
		return "pending"
	case models.EmailQueueDelivering:
		return "delivering"
	case models.EmailQueueDelivered:
		return "delivered"
	case models.EmailQueueFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

// List returns one page of the outbox. filter is "pending" (waiting or leased
// rows) or "failed" (given up). The total counts every row of that filter, so
// the UI can page through all of them.
func List(filter string, page, pageSize int) ([]EntryView, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	var where string
	var args []any
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "pending":
		where = "state in (?, ?)"
		args = []any{models.EmailQueuePending, models.EmailQueueDelivering}
	case "failed":
		where = "state = ?"
		args = []any{models.EmailQueueFailed}
	default:
		return nil, 0, fmt.Errorf("unknown outbox filter %q", filter)
	}

	total, err := db.Instance.Table(&models.EmailQueue{}).Where(where, args...).Count()
	if err != nil {
		return nil, 0, err
	}
	var rows []models.EmailQueue
	if err := db.Instance.Table(&models.EmailQueue{}).Where(where, args...).
		OrderBy("id desc").Limit(pageSize, (page-1)*pageSize).Find(&rows); err != nil {
		return nil, 0, err
	}
	entries := make([]EntryView, 0, len(rows))
	for i := range rows {
		entries = append(entries, viewOf(&rows[i]))
	}
	return entries, total, nil
}

func viewOf(entry *models.EmailQueue) EntryView {
	recipients, err := Recipients(entry)
	if err != nil {
		// A malformed row is still worth showing: without this the operator
		// cannot find the row that needs deleting.
		recipients = nil
	}
	return EntryView{
		ID:           entry.Id,
		EmailID:      entry.EmailID,
		MsgID:        entry.MsgID,
		EnvelopeFrom: entry.EnvelopeFrom,
		Recipients:   recipients,
		State:        StateName(entry.State),
		StateCode:    entry.State,
		Attempts:     entry.Attempts,
		NextAttempt:  entry.NextAttempt,
		LastError:    LastError(entry),
		CreateTime:   entry.CreateTime,
		UpdateTime:   entry.UpdateTime,
	}
}

// Retry puts a finally failed row back in the queue immediately. The retry
// window (MaxRetryAge) restarts, so a row an administrator retries after days
// is not given up on its first attempt; the attempt counter restarts with it so
// the backoff starts at one minute again.
func Retry(id int64, now time.Time) error {
	now = now.UTC()
	return db.Transaction(db.Instance, func(session *xorm.Session) error {
		var entry models.EmailQueue
		found, err := session.ID(id).Get(&entry)
		if err != nil {
			return err
		}
		if !found {
			return ErrEntryNotFound
		}
		if entry.State != models.EmailQueueFailed {
			return fmt.Errorf("%w: only a failed row can be retried, this one is %s", ErrEntryState, StateName(entry.State))
		}
		affected, err := session.Table(&models.EmailQueue{}).
			Where("id = ? and state = ?", id, models.EmailQueueFailed).
			Update(map[string]any{
				"state":        models.EmailQueuePending,
				"attempts":     0,
				"next_attempt": now,
				"lease_until":  noLease,
				"last_error":   nil,
				"create_time":  now,
				"update_time":  now,
			})
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("%w: the row changed while it was being retried", ErrEntryState)
		}
		return setEmailResult(session, entry.EmailID, consts.EmailStatusDeliveryPending, nil)
	})
}

// Delete removes one finished queue row. Pending, delivering and leased rows
// are never deleted here: the worker still owns them, and deleting a pending
// row would silently drop mail. A delivered or failed row is history, so the
// email audit row keeps its own status.
func Delete(id int64) error {
	result, err := db.Instance.Exec(
		db.WithContext(nil, "delete from email_queue where id=? and state in (?, ?)"),
		id, models.EmailQueueDelivered, models.EmailQueueFailed)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	var entry models.EmailQueue
	found, err := db.Instance.ID(id).Get(&entry)
	if err != nil {
		return err
	}
	if !found {
		return ErrEntryNotFound
	}
	return fmt.Errorf("%w: only a delivered or failed row can be deleted, this one is %s", ErrEntryState, StateName(entry.State))
}
