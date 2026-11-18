package models

import "time"

// UserEmail status values. Trashed messages are excluded from quota usage.
const DeletedStatus int8 = 3

type UserEmail struct {
	ID      int       `xorm:"id int unsigned not null pk autoincr"`
	UserID  int       `xorm:"user_id int not null index('idx_eid') index comment('user id')"`
	EmailID int       `xorm:"email_id not null index('idx_eid') index comment('email id')"`
	IsRead  int8      `xorm:"is_read tinyint(1) comment('whether it has been read')" json:"is_read"`
	GroupId int       `xorm:"group_id int notnull default(0) comment('group id')'" json:"group_id"`
	Status  int8      `xorm:"status tinyint(4) notnull default(0) comment('0 not sent or received, 1 sent, 2 send failed, 3 deleted, 4 draft, 5 spam')" json:"status"` // 0 = not sent or received, 1 = sent, 2 = send failed, 3 = deleted, 4 = draft, 5 = junk
	Created time.Time `xorm:"create datetime created index('idx_create_time')"`
}

func (p UserEmail) TableName() string {
	return "user_email"
}
