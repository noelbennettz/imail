package models

// MailboxState stores the stable IMAP UIDVALIDITY and the monotonic UIDNEXT of
// one mailbox. UIDNEXT may never decrease, even after the newest message is
// deleted, so it cannot be derived from MAX(id) alone.
type MailboxState struct {
	ID          int    `xorm:"id int unsigned not null pk autoincr"`
	UserID      int    `xorm:"user_id int not null unique('idx_mailbox_state')"`
	Mailbox     string `xorm:"varchar(255) not null unique('idx_mailbox_state')"`
	UIDValidity int    `xorm:"uid_validity int not null default(0)"`
	UIDNext     int    `xorm:"uid_next int not null default(1)"`
}

func (p *MailboxState) TableName() string {
	return "mailbox_state"
}
