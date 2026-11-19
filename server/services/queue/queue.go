// Package queue stores outbound mail for remote recipients and delivers it in
// the background.
//
// The SMTP server writes the email row and the queue row in one transaction, so
// once the client has its 250 the message is durable. A worker started with the
// server claims due rows with a short lease, sends the stored MIME with the
// existing MX/TLS logic, and records the final result in the queue row and in
// the email audit columns. Temporary failures are retried with exponential
// backoff up to a fixed age; a permanent 5xx reply fails the row at once.
//
// There is no external dependency: the queue is a database table, and a restart
// resumes whatever is still pending.
package queue

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

const (
	// MaxRetryAge bounds how long a row may keep being retried. It counts from
	// the moment the email was accepted, not from the last attempt.
	MaxRetryAge = 72 * time.Hour
	// ClaimBatch is the number of rows one claim query may return.
	ClaimBatch = 10
	// DefaultLease is how long a claimed row stays owned by one worker. A row
	// whose lease expired is claimed again, which is what makes a delivery that
	// was interrupted by a crash get retried.
	DefaultLease = 10 * time.Minute
)

// Backoff lists the delay before the next attempt, indexed by the number of
// attempts already made: 1 minute, 5, 15, 1 hour, 6 hours, then 24 hours.
var Backoff = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

// noLease marks a row that no worker owns. It is deliberately not the zero time:
// MySQL rejects year 1, so a sentinel inside every supported dialect's range is
// used instead.
var noLease = time.Unix(0, 0).UTC()

// RetryDelay returns the delay after the given number of attempts (1 or more).
func RetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	index := attempts - 1
	if index >= len(Backoff) {
		index = len(Backoff) - 1
	}
	return Backoff[index]
}

// NextAttempt returns when the next attempt is due and whether the entry is
// still inside MaxRetryAge. attempts is the number of attempts already made.
func NextAttempt(now, created time.Time, attempts int) (time.Time, bool) {
	next := now.Add(RetryDelay(attempts))
	if next.After(created.Add(MaxRetryAge)) {
		return next, false
	}
	return next, true
}

// Entry builds the queue row for one stored email. recipients must not be
// empty; local recipients are delivered inline by the caller and are not part
// of the row.
func Entry(emailID int, msgID, envelopeFrom string, recipients []string, now time.Time) (models.EmailQueue, error) {
	// Times are stored in UTC: the SQLite driver writes a bare wall clock and
	// reads it back as UTC, so a local-zone value would drift.
	now = now.UTC()
	encoded, err := json.Marshal(recipients)
	if err != nil {
		return models.EmailQueue{}, err
	}
	return models.EmailQueue{
		EmailID:      emailID,
		MsgID:        msgID,
		EnvelopeFrom: envelopeFrom,
		Recipients:   string(encoded),
		State:        models.EmailQueuePending,
		LeaseUntil:   noLease,
		NextAttempt:  now,
		CreateTime:   now,
		UpdateTime:   now,
	}, nil
}

// Recipients decodes the stored recipient list.
func Recipients(entry *models.EmailQueue) ([]string, error) {
	var recipients []string
	if strings.TrimSpace(entry.Recipients) == "" {
		return nil, fmt.Errorf("queue entry %d has no recipients", entry.Id)
	}
	if err := json.Unmarshal([]byte(entry.Recipients), &recipients); err != nil {
		return nil, fmt.Errorf("queue entry %d has malformed recipients: %w", entry.Id, err)
	}
	return recipients, nil
}

// Entries builds one queue row per remote recipient domain. Retrying per domain
// is what keeps a domain that already accepted the message from receiving it
// again when another domain is still failing.
func Entries(emailID int, msgID, envelopeFrom string, recipients []string, now time.Time) ([]models.EmailQueue, error) {
	byDomain := make(map[string][]string)
	var domains []string
	for _, address := range recipients {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		_, domain, found := strings.Cut(strings.ToLower(address), "@")
		if !found {
			// Addresses without a domain cannot be grouped; they get their own
			// row so the failure is visible instead of being dropped.
			domain = address
		}
		if _, seen := byDomain[domain]; !seen {
			domains = append(domains, domain)
		}
		byDomain[domain] = append(byDomain[domain], address)
	}
	sort.Strings(domains)
	entries := make([]models.EmailQueue, 0, len(domains))
	for _, domain := range domains {
		entry, err := Entry(emailID, msgID, envelopeFrom, byDomain[domain], now)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Claim leases one due entry, or returns nil when nothing is due. The lease and
// the state change happen in one conditional UPDATE, so two workers racing for
// the same row cannot both win.
func Claim(now time.Time, lease time.Duration) (*models.EmailQueue, error) {
	now = now.UTC()
	session := db.Instance.NewSession()
	defer session.Close()

	var candidate models.EmailQueue
	due := "(state = ? and next_attempt <= ?) or (state = ? and lease_until <= ?)"
	args := []any{models.EmailQueuePending, now, models.EmailQueueDelivering, now}
	found, err := session.Where(due, args...).
		OrderBy("next_attempt asc, id asc").
		Limit(1).
		Get(&candidate)
	if err != nil || !found {
		return nil, err
	}

	leaseUntil := now.Add(lease)
	affected, err := session.Table(&models.EmailQueue{}).
		Where("id = ? and ("+due+")", append([]any{candidate.Id}, args...)...).
		Update(map[string]any{
			"state":        models.EmailQueueDelivering,
			"lease_until":  leaseUntil,
			"attempts":     candidate.Attempts + 1,
			"update_time":  now,
			"next_attempt": candidate.NextAttempt,
		})
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		// Another worker claimed this row between the select and the update.
		return nil, nil
	}
	candidate.State = models.EmailQueueDelivering
	candidate.LeaseUntil = leaseUntil
	candidate.Attempts++
	return &candidate, nil
}

// MarkDelivered records a successful delivery and marks the email as sent.
func MarkDelivered(ctx *context.Context, entry *models.EmailQueue, now time.Time) error {
	now = now.UTC()
	return db.Transaction(db.Instance, func(session *xorm.Session) error {
		if err := updateEntry(session, entry.Id, map[string]any{
			"state":        models.EmailQueueDelivered,
			"lease_until":  noLease,
			"last_error":   nil,
			"next_attempt": now,
			"update_time":  now,
		}); err != nil {
			return err
		}
		return finalizeEmail(session, entry.EmailID)
	})
}

// MarkFailed records the final failure of an entry and marks the email as
// failed. The error text is kept on both rows.
func MarkFailed(ctx *context.Context, entry *models.EmailQueue, reason string, now time.Time) error {
	now = now.UTC()
	return db.Transaction(db.Instance, func(session *xorm.Session) error {
		if err := updateEntry(session, entry.Id, map[string]any{
			"state":        models.EmailQueueFailed,
			"lease_until":  noLease,
			"last_error":   reason,
			"next_attempt": now,
			"update_time":  now,
		}); err != nil {
			return err
		}
		return finalizeEmail(session, entry.EmailID)
	})
}

// MarkRetry schedules the next attempt and keeps the email in the pending
// state, with the reason recorded so an administrator can see what is wrong.
func MarkRetry(ctx *context.Context, entry *models.EmailQueue, reason string, next time.Time, now time.Time) error {
	now = now.UTC()
	next = next.UTC()
	return db.Transaction(db.Instance, func(session *xorm.Session) error {
		if err := updateEntry(session, entry.Id, map[string]any{
			"state":        models.EmailQueuePending,
			"lease_until":  noLease,
			"last_error":   reason,
			"next_attempt": next,
			"update_time":  now,
		}); err != nil {
			return err
		}
		return setEmailResult(session, entry.EmailID, consts.EmailStatusDeliveryPending, reason)
	})
}

// Counts returns how many entries are waiting and how many have finally failed.
func Counts() (pending, failed int64, err error) {
	pending, err = db.Instance.Table(&models.EmailQueue{}).
		Where("state in (?, ?)", models.EmailQueuePending, models.EmailQueueDelivering).
		Count()
	if err != nil {
		return 0, 0, err
	}
	failed, err = db.Instance.Table(&models.EmailQueue{}).
		Where("state = ?", models.EmailQueueFailed).
		Count()
	if err != nil {
		return 0, 0, err
	}
	return pending, failed, nil
}

// updateEntry applies a state change to one queue row.
func updateEntry(session *xorm.Session, id int64, values map[string]any) error {
	affected, err := session.Table(&models.EmailQueue{}).Where("id = ?", id).Update(values)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("update queue entry %d: affected %d rows, want 1", id, affected)
	}
	return nil
}

// setEmailResult mirrors the delivery outcome into the email audit columns the
// web client reads.
func setEmailResult(session *xorm.Session, emailID int, status int8, errorText any) error {
	if _, err := session.Exec(db.WithContext(nil, `update email
		set status=?, error=?, update_time=CURRENT_TIMESTAMP
		where id=?`), status, errorText, emailID); err != nil {
		return err
	}
	// The message may already be deleted by its owner; the row still exists and
	// the update is best effort.
	if _, err := session.Exec(db.WithContext(nil, `update user_email
		set status=? where email_id=?`), status, emailID); err != nil {
		return err
	}
	return nil
}

// finalizeEmail writes the aggregate result once no queue row of the email is
// outstanding any more. One remote domain per row means a domain that accepted
// the message is never contacted again while another domain is still retrying.
func finalizeEmail(session *xorm.Session, emailID int) error {
	outstanding, err := session.Table(&models.EmailQueue{}).
		Where("email_id = ? and state in (?, ?)", emailID, models.EmailQueuePending, models.EmailQueueDelivering).
		Count()
	if err != nil {
		return err
	}
	if outstanding > 0 {
		return nil
	}

	var failed models.EmailQueue
	found, err := session.Table(&models.EmailQueue{}).
		Where("email_id = ? and state = ?", emailID, models.EmailQueueFailed).
		Desc("id").Limit(1).Get(&failed)
	if err != nil {
		return err
	}
	if found {
		reason := LastError(&failed)
		if reason == "" {
			reason = "delivery to a remote domain failed"
		}
		return setEmailResult(session, emailID, consts.EmailStatusFail, reason)
	}
	return setEmailResult(session, emailID, consts.EmailStatusSent, nil)
}

// LastError renders the stored error text for logs.
func LastError(entry *models.EmailQueue) string {
	if !entry.LastError.Valid {
		return ""
	}
	return entry.LastError.String
}

// logAttempt writes one line per delivery attempt, as the operations page and
// the logs need to follow a queued message.
func logAttempt(ctx *context.Context, entry *models.EmailQueue, recipients []string, outcome string, err error) {
	if err != nil {
		log.WithContext(ctx).Warnf("outbox id=%d msg_id=%q attempt=%d recipients=%s outcome=%s error=%v",
			entry.Id, entry.MsgID, entry.Attempts, strings.Join(recipients, ","), outcome, err)
		return
	}
	log.WithContext(ctx).Infof("outbox id=%d msg_id=%q attempt=%d recipients=%s outcome=%s",
		entry.Id, entry.MsgID, entry.Attempts, strings.Join(recipients, ","), outcome)
}
