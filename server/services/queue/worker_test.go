package queue

import (
	"errors"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/smtp"
)

func temporaryFailure() (error, map[string]error) {
	return errors.New("dial timeout"), map[string]error{"example.net": &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("i/o timeout")}}
}

func permanentDeliveryFailure() (error, map[string]error) {
	return errors.New("delivery failed"), map[string]error{"example.net": &textproto.Error{Code: 550, Msg: "mailbox unavailable"}}
}

func success(*context.Context, *models.EmailQueue, *models.Email) (error, map[string]error) {
	return nil, nil
}

func newTestWorker(now func() time.Time) *Worker {
	worker := NewWorker()
	worker.Interval = time.Hour // only Drain() runs in these tests
	worker.Now = now
	return worker
}

func TestWorkerRetriesTemporaryFailureWithBackoff(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "retry@example.com")
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	entry := enqueueAt(t, engine, email, now, "recipient@example.net")

	worker := newTestWorker(func() time.Time { return now })
	worker.deliver = func(*context.Context, *models.EmailQueue, *models.Email) (error, map[string]error) {
		return temporaryFailure()
	}

	if processed := worker.Drain(); processed != 1 {
		t.Fatalf("Drain() = %d, want 1", processed)
	}
	got := loadEntry(t, engine, entry.Id)
	if got.State != models.EmailQueuePending {
		t.Fatalf("state after a temporary failure = %d, want pending", got.State)
	}
	if got.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got.Attempts)
	}
	if want := now.Add(time.Minute); !got.NextAttempt.Equal(want) {
		t.Fatalf("next attempt = %s, want %s", got.NextAttempt, want)
	}
	if !strings.Contains(LastError(&got), "dial timeout") {
		t.Fatalf("last error = %q, want the delivery reason", LastError(&got))
	}
	if status := loadEmail(t, engine, email.Id).Status; status != consts.EmailStatusDeliveryPending {
		t.Fatalf("email status = %d, want the pending state", status)
	}

	// Nothing is due a second before the backoff expires.
	now = now.Add(59 * time.Second)
	if processed := worker.Drain(); processed != 0 {
		t.Fatalf("Drain() before the backoff = %d, want 0", processed)
	}
	// A later attempt succeeds.
	now = now.Add(2 * time.Second)
	worker.deliver = success
	if processed := worker.Drain(); processed != 1 {
		t.Fatalf("Drain() after the backoff = %d, want 1", processed)
	}
	got = loadEntry(t, engine, entry.Id)
	if got.State != models.EmailQueueDelivered || got.Attempts != 2 {
		t.Fatalf("state after delivery = %d attempts %d, want delivered/2", got.State, got.Attempts)
	}
	if status := loadEmail(t, engine, email.Id).Status; status != consts.EmailStatusSent {
		t.Fatalf("email status = %d, want sent", status)
	}
}

func TestWorkerBackoffGrowsWithEachAttempt(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "backoff@example.com")
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	entry := enqueueAt(t, engine, email, now, "recipient@example.net")

	worker := newTestWorker(func() time.Time { return now })
	worker.deliver = func(*context.Context, *models.EmailQueue, *models.Email) (error, map[string]error) {
		return temporaryFailure()
	}

	for attempt, delay := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute} {
		if processed := worker.Drain(); processed != 1 {
			t.Fatalf("attempt %d: Drain() = %d, want 1", attempt+1, processed)
		}
		got := loadEntry(t, engine, entry.Id)
		if want := now.Add(delay); !got.NextAttempt.Equal(want) {
			t.Fatalf("attempt %d: next attempt = %s, want %s", attempt+1, got.NextAttempt, want)
		}
		now = got.NextAttempt
	}
}

func TestWorkerPermanentFailureFailsImmediately(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "permanent@example.com")
	entry := enqueue(t, engine, email, "recipient@example.net")

	worker := newTestWorker(time.Now)
	worker.deliver = func(*context.Context, *models.EmailQueue, *models.Email) (error, map[string]error) {
		return permanentDeliveryFailure()
	}
	if processed := worker.Drain(); processed != 1 {
		t.Fatalf("Drain() = %d, want 1", processed)
	}
	got := loadEntry(t, engine, entry.Id)
	if got.State != models.EmailQueueFailed || got.Attempts != 1 {
		t.Fatalf("state = %d attempts = %d, want failed after one attempt", got.State, got.Attempts)
	}
	stored := loadEmail(t, engine, email.Id)
	if stored.Status != consts.EmailStatusFail {
		t.Fatalf("email status = %d, want failed", stored.Status)
	}
	if stored.Error.String != "delivery failed" {
		t.Fatalf("email error = %q, want the delivery reason", stored.Error.String)
	}
}

func TestWorkerGivesUpAfterRetryWindow(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "too-old@example.com")
	longAgo := time.Now().Add(-MaxRetryAge - time.Hour)
	entry := enqueueAt(t, engine, email, longAgo, "recipient@example.net")

	worker := newTestWorker(time.Now)
	worker.deliver = func(*context.Context, *models.EmailQueue, *models.Email) (error, map[string]error) {
		return temporaryFailure()
	}
	worker.Drain()
	got := loadEntry(t, engine, entry.Id)
	if got.State != models.EmailQueueFailed {
		t.Fatalf("state = %d, want failed once the retry window is exhausted", got.State)
	}
	if !strings.Contains(LastError(&got), "retry window") {
		t.Fatalf("last error = %q, want the retry window message", LastError(&got))
	}
}

func TestConcurrentWorkersDeliverOnce(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "one-delivery@example.com")
	enqueue(t, engine, email, "recipient@example.net")

	now := time.Now()
	var sends atomic.Int32
	deliver := func(*context.Context, *models.EmailQueue, *models.Email) (error, map[string]error) {
		sends.Add(1)
		time.Sleep(50 * time.Millisecond)
		return nil, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		worker := newTestWorker(func() time.Time { return now })
		worker.deliver = deliver
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker.Drain()
		}()
	}
	wg.Wait()
	if got := sends.Load(); got != 1 {
		t.Fatalf("deliveries = %d, want exactly 1", got)
	}
}

func TestWorkerResumesPendingEntriesOnStart(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "resume@example.com")
	entry := enqueue(t, engine, email, "recipient@example.net")

	var sends atomic.Int32
	worker := NewWorker()
	worker.Interval = 10 * time.Millisecond
	worker.deliver = func(*context.Context, *models.EmailQueue, *models.Email) (error, map[string]error) {
		sends.Add(1)
		return nil, nil
	}
	worker.Start()
	t.Cleanup(worker.Stop)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := loadEntry(t, engine, entry.Id); got.State == models.EmailQueueDelivered {
			if sends.Load() != 1 {
				t.Fatalf("deliveries = %d, want 1", sends.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("worker did not deliver the pending entry, state = %d", loadEntry(t, engine, entry.Id).State)
}

func TestCounts(t *testing.T) {
	engine, _ := newTestEngine(t)
	first := insertEmail(t, engine, "counts-1@example.com")
	second := insertEmail(t, engine, "counts-2@example.com")
	pending := enqueue(t, engine, first, "one@example.net")
	failed := enqueue(t, engine, second, "two@example.net")

	if err := MarkFailed(&context.Context{}, &failed, "rejected", time.Now()); err != nil {
		t.Fatal(err)
	}
	gotPending, gotFailed, err := Counts()
	if err != nil {
		t.Fatal(err)
	}
	if gotPending != 1 || gotFailed != 1 {
		t.Fatalf("Counts() = pending %d failed %d, want 1/1", gotPending, gotFailed)
	}
	if err := MarkDelivered(&context.Context{}, &pending, time.Now()); err != nil {
		t.Fatal(err)
	}
	gotPending, gotFailed, err = Counts()
	if err != nil {
		t.Fatal(err)
	}
	if gotPending != 0 || gotFailed != 1 {
		t.Fatalf("Counts() after delivery = pending %d failed %d, want 0/1", gotPending, gotFailed)
	}
}

// startLoopbackSMTP accepts one SMTP transaction and returns the DATA it
// received.
func startLoopbackSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		text := textproto.NewConn(conn)
		defer text.Close()
		if err := text.PrintfLine("220 loopback ESMTP ready"); err != nil {
			return
		}
		expect := func(prefix string) bool {
			line, err := text.ReadLine()
			if err != nil {
				return false
			}
			return strings.HasPrefix(line, prefix)
		}
		if !expect("EHLO ") {
			return
		}
		if err := text.PrintfLine("250 loopback"); err != nil {
			return
		}
		if !expect("MAIL FROM:") {
			return
		}
		if err := text.PrintfLine("250 2.1.0 sender accepted"); err != nil {
			return
		}
		if !expect("RCPT TO:") {
			return
		}
		if err := text.PrintfLine("250 2.1.5 recipient accepted"); err != nil {
			return
		}
		if !expect("DATA") {
			return
		}
		if err := text.PrintfLine("354 end data with <CR><LF>.<CR><LF>"); err != nil {
			return
		}
		body, err := io.ReadAll(text.DotReader())
		if err != nil {
			return
		}
		if err := text.PrintfLine("250 2.0.0 queued"); err != nil {
			return
		}
		received <- string(body)
	}()
	return listener.Addr().String(), received
}

// TestWorkerDeliversOverLoopbackSMTP covers the whole path with the real SMTP
// client: a queued row is claimed, its stored MIME is sent over TCP, and the
// row and the audit row end up delivered. The dial target is injected because
// the real MX path always uses port 25, which an unprivileged test cannot use.
func TestWorkerDeliversOverLoopbackSMTP(t *testing.T) {
	engine, _ := newTestEngine(t)
	email := insertEmail(t, engine, "loopback@example.com")
	entry := enqueue(t, engine, email, "recipient@example.net")

	addr, received := startLoopbackSMTP(t)
	worker := newTestWorker(time.Now)
	worker.deliver = func(ctx *context.Context, entry *models.EmailQueue, email *models.Email) (error, map[string]error) {
		recipients, err := Recipients(entry)
		if err != nil {
			return err, nil
		}
		if err := smtp.SendMailUnsafe("", addr, nil, entry.EnvelopeFrom, "example.com", recipients, []byte(email.RawMIME)); err != nil {
			return err, nil
		}
		return nil, nil
	}

	if processed := worker.Drain(); processed != 1 {
		t.Fatalf("Drain() = %d, want 1", processed)
	}
	select {
	case body := <-received:
		if !strings.Contains(body, "Subject: queued") {
			t.Fatalf("delivered message = %q, want the stored MIME", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loopback SMTP server received no message")
	}

	got := loadEntry(t, engine, entry.Id)
	if got.State != models.EmailQueueDelivered {
		t.Fatalf("state = %d, want delivered", got.State)
	}
	if status := loadEmail(t, engine, email.Id).Status; status != consts.EmailStatusSent {
		t.Fatalf("email status = %d, want sent", status)
	}
}
