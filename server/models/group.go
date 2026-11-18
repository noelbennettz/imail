package models

type Group struct {
	ID       int    `xorm:"id int unsigned not null pk autoincr" json:"id"`
	Name     string `xorm:"varchar(10) notnull default('') comment('group name')" json:"name"`
	ParentId int    `xorm:"parent_id int unsigned notnull default(0) comment('parent group name')" json:"parent_id"`
	UserId   int    `xorm:"user_id int unsigned notnull default(0) comment('user id')" json:"-"`
	FullPath string `xrom:"full_path varchar(600) comment('full path')" json:"full_path"`
}

const (
	INBOX   = 2000000000 // Inbox
	Sent    = 2000000001 // Sent
	Drafts  = 2000000002 // Drafts
	Deleted = 2000000003 // Trash
	Junk    = 2000000004 // Junk
)

var GroupNameToCode = map[string]int{
	"INBOX":            INBOX,
	"Sent Messages":    Sent,
	"Drafts":           Drafts,
	"Deleted Messages": Deleted,
	"Junk":             Junk,
}

var GroupCodeToName = map[int]string{
	INBOX:   "INBOX",
	Sent:    "Sent Messages",
	Drafts:  "Drafts",
	Deleted: "Deleted Messages",
	Junk:    "Junk",
}

func (p *Group) TableName() string {
	return "group"
}
