package models

type User struct {
	ID          int    `xorm:"id unsigned int not null pk autoincr"`
	Account     string `xorm:"varchar(20) notnull unique comment('account login name')"`
	Name        string `xorm:"varchar(10) notnull comment('user name')"`
	Password    string `xorm:"varchar(255) notnull comment('login password, PBKDF2-HMAC-SHA256')" json:"-"`
	Disabled    int    `xorm:"disabled unsigned int not null default(0) comment('0 enabled, 1 disabled')"`
	IsAdmin     int    `xorm:"is_admin unsigned int not null default(0) comment('0 not admin, 1 admin')"`
	AuthVersion int64  `xorm:"auth_version bigint notnull default(0)" json:"-"`
	// Quota is the mailbox size limit in bytes. 0 means unlimited.
	Quota int64 `xorm:"quota bigint notnull default(0) comment('mailbox quota in bytes, 0 = unlimited')" json:"quota"`
	// Domain is the primary domain of this mailbox. Empty means any
	// configured domain (legacy accounts).
	Domain string `xorm:"domain varchar(100) notnull default('') comment('primary domain of the mailbox')" json:"domain"`
}

func (p User) TableName() string {
	return "user"
}
