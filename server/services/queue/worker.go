package queue

import (
	"fmt"
	"sync"
	"time"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/id"
	"github.com/noelbennettz/imail/utils/send"
	log "github.com/sirupsen/logrus"
)

// deliveryFunc delivers one queued entry. It is a field so tests can point it
// at a loopback SMTP server instead of the real MX lookup.
type deliveryFunc func(ctx *context.Context, entry *models.EmailQueue, email *models.Email) (error, map[string]error)

// Worker drains the outbound queue. One worker runs per process; the database
// lease, not the process count, is what keeps a row from being delivered twice.
type Worker struct {
	Interval time.Duration
	Lease    time.Duration
	// PurgeInterval is how often finished rows beyond their retention are
	// deleted, so the table stays bounded on a long-running server.
	PurgeInterval time.Duration
	Now           func() time.Time
	// deliver is nil outside tests, where send.Deliver is used.
	deliver deliveryFunc

	stop      chan struct{}
	done      chan struct{}
	stopOnce  sync.Once
	nextPurge time.Time
}

// NewWorker returns a worker with the production timings.
func NewWorker() *Worker {
	return &Worker{
		Interval:      10 * time.Second,
		Lease:         DefaultLease,
		Now:           time.Now,
		stop:          make(chan struct{}),
		PurgeInterval: time.Hour,
		done:          make(chan struct{}),
	}
}

// Start runs the worker in the background. It processes whatever is already due
// before the first tick, so a restart resumes pending mail immediately.
func (w *Worker) Start() {
	go w.Run()
}

// Stop cancels the worker and waits for an in-flight delivery to finish, so a
// clean shutdown leaves the row in the state the delivery produced.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

// Run is the worker loop.
func (w *Worker) Run() {
	defer close(w.done)
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		w.maintain()
		w.Drain()
		select {
		case <-w.stop:
			return
		case <-ticker.C:
		}
	}
}

// maintain applies the retention policy at most once per PurgeInterval. It runs
// before the first drain, so a restart also cleans up.
func (w *Worker) maintain() {
	now := w.Now().UTC()
	if !w.nextPurge.IsZero() && now.Before(w.nextPurge) {
		return
	}
	if w.PurgeInterval <= 0 {
		w.PurgeInterval = time.Hour
	}
	w.nextPurge = now.Add(w.PurgeInterval)
	delivered, failed, err := Purge(now)
	if err != nil {
		log.Errorf("outbox: retention cleanup failed: %v", err)
		return
	}
	if delivered+failed > 0 {
		log.Infof("outbox: purged %d delivered and %d failed rows", delivered, failed)
	}
}

// Drain claims and delivers every entry that is due right now. It stops early
// when the worker is asked to shut down.
func (w *Worker) Drain() int {
	processed := 0
	for {
		select {
		case <-w.stop:
			return processed
		default:
		}
		entry, err := Claim(w.Now(), w.Lease)
		if err != nil {
			log.Errorf("outbox: claim failed: %v", err)
			return processed
		}
		if entry == nil {
			return processed
		}
		w.process(entry)
		processed++
	}
}

// process delivers one claimed entry and records the outcome.
func (w *Worker) process(entry *models.EmailQueue) {
	ctx := &context.Context{}
	ctx.SetValue(context.LogID, id.GenLogID())
	now := w.Now().UTC()

	recipients, err := Recipients(entry)
	if err != nil {
		w.fail(ctx, entry, err, now)
		return
	}

	var email models.Email
	found, err := db.Instance.ID(entry.EmailID).Get(&email)
	if err != nil {
		// The body may be readable on the next attempt.
		w.retry(ctx, entry, recipients, fmt.Errorf("load email %d: %w", entry.EmailID, err), now)
		return
	}
	if !found {
		w.fail(ctx, entry, fmt.Errorf("email row %d no longer exists", entry.EmailID), now)
		return
	}

	deliveryErr, domainErrors := w.deliverEntry(ctx, entry, &email)
	if deliveryErr == nil {
		logAttempt(ctx, entry, recipients, "delivered", nil)
		if err := MarkDelivered(ctx, entry, now); err != nil {
			// The remote accepted the message. Leaving the row leased means it
			// is retried after the lease expires (at-least-once); the CRITICAL
			// log tells the operator why a duplicate may appear.
			log.WithContext(ctx).Errorf("CRITICAL outbox id=%d msg_id=%q delivered but the result could not be stored: %v",
				entry.Id, entry.MsgID, err)
		}
		return
	}

	if permanentFailure(domainErrors) {
		logAttempt(ctx, entry, recipients, "permanent-failure", deliveryErr)
		w.fail(ctx, entry, deliveryErr, now)
		return
	}
	w.retry(ctx, entry, recipients, deliveryErr, now)
}

// fail records a final failure.
func (w *Worker) fail(ctx *context.Context, entry *models.EmailQueue, reason error, now time.Time) {
	if err := MarkFailed(ctx, entry, reason.Error(), now); err != nil {
		log.WithContext(ctx).Errorf("outbox id=%d msg_id=%q could not store the failure: %v (reason: %v)",
			entry.Id, entry.MsgID, err, reason)
	}
}

// retry schedules the next attempt, or gives up when the entry is older than
// MaxRetryAge.
func (w *Worker) retry(ctx *context.Context, entry *models.EmailQueue, recipients []string, reason error, now time.Time) {
	next, keepGoing := NextAttempt(now, entry.CreateTime, entry.Attempts)
	if !keepGoing {
		logAttempt(ctx, entry, recipients, "retry-window-exhausted", reason)
		w.fail(ctx, entry, fmt.Errorf("retry window of %s exhausted: %w", MaxRetryAge, reason), now)
		return
	}
	logAttempt(ctx, entry, recipients, "retry-at="+next.UTC().Format(time.RFC3339), reason)
	if err := MarkRetry(ctx, entry, reason.Error(), next, now); err != nil {
		log.WithContext(ctx).Errorf("outbox id=%d msg_id=%q could not schedule the retry: %v (reason: %v)",
			entry.Id, entry.MsgID, err, reason)
	}
}

// deliverEntry sends the stored MIME with the existing MX/TLS logic.
func (w *Worker) deliverEntry(ctx *context.Context, entry *models.EmailQueue, email *models.Email) (error, map[string]error) {
	if w.deliver != nil {
		return w.deliver(ctx, entry, email)
	}
	recipients, err := Recipients(entry)
	if err != nil {
		return err, nil
	}
	local, remote := splitQueueRecipients(recipients)
	if len(local) > 0 {
		if localErr := deliverLocal(ctx, email, local); localErr != nil {
			return localErr, map[string]error{"local": localErr}
		}
	}
	if len(remote) == 0 {
		return nil, nil
	}
	data := []byte(email.RawMIME)
	if len(data) == 0 {
		// Rows written before raw MIME was stored can still be rebuilt from the
		// parsed columns; the DKIM signature is regenerated.
		data = parsemail.NewEmailFromModel(*email).BuildBytes(ctx, true)
	}
	if len(data) == 0 {
		return fmt.Errorf("email %d has no message body", email.Id), nil
	}
	return send.Deliver(ctx, entry.EnvelopeFrom, remote, data)
}

// permanentFailure reports whether every failing recipient domain answered with
// a 5xx reply, which retrying cannot fix. An empty error map means the cause is
// unknown, so the entry is retried.
func permanentFailure(domainErrors map[string]error) bool {
	if len(domainErrors) == 0 {
		return false
	}
	for _, err := range domainErrors {
		if err == nil || !send.IsPermanentSMTPResponse(err) {
			return false
		}
	}
	return true
}

var (
	workerMu sync.Mutex
	instance *Worker
)

// Start runs the outbound queue worker for this process.
func Start() {
	workerMu.Lock()
	defer workerMu.Unlock()
	if instance != nil {
		return
	}
	instance = NewWorker()
	log.Infof("Outbound queue worker started (interval %s)", instance.Interval)
	instance.Start()
}

// Stop stops the outbound queue worker and waits for the current delivery.
func Stop() {
	workerMu.Lock()
	current := instance
	instance = nil
	workerMu.Unlock()
	if current != nil {
		current.Stop()
		log.Infof("Outbound queue worker stopped")
	}
}
