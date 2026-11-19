package models

import (
	"database/sql"
	"time"
)

// Outbound queue states. A row starts pending, is leased while a worker
// delivers it, and ends delivered or failed.
const (
	EmailQueuePending    int8 = 0
	EmailQueueDelivering int8 = 1
	EmailQueueDelivered  int8 = 2
	EmailQueueFailed     int8 = 3
)

// EmailQueue is one durable outbound delivery: the remote recipients of a
// stored email. The message body is not copied; the row references the email
// row written by the same transaction, whose raw_mime column holds the original
// bytes.
type EmailQueue struct {
	Id           int64          `xorm:"id pk autoincr notnull"`
	EmailID      int            `xorm:"email_id notnull index(ix_email_queue_email)"`
	MsgID        string         `xorm:"msg_id varchar(255) notnull default('')"`
	EnvelopeFrom string         `xorm:"envelope_from varchar(320) notnull default('')"`
	Recipients   string         `xorm:"recipients text comment('JSON array of envelope recipients')"`
	State        int8           `xorm:"state tinyint(4) notnull default(0) index(ix_email_queue_state) index(ix_email_queue_state_time)"`
	Attempts     int            `xorm:"attempts notnull default(0)"`
	NextAttempt  time.Time      `xorm:"next_attempt index(ix_email_queue_next)"`
	LeaseUntil   time.Time      `xorm:"lease_until comment('a delivering row may be claimed again after this time')"`
	LastError    sql.NullString `xorm:"last_error text"`
	CreateTime   time.Time      `xorm:"create_time created"`
	// UpdateTime is set explicitly by every queue update, not by xorm's
	// `updated` tag: the tag adds its own assignment on an UPDATE, which
	// PostgreSQL rejects as a duplicate assignment to the same column.
	UpdateTime time.Time `xorm:"update_time index(ix_email_queue_state_time)"`
}

func (d *EmailQueue) TableName() string {
	return "email_queue"
}
