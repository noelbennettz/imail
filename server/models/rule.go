package models

type Rule struct {
	Id     int    `xorm:"id int unsigned not null pk autoincr" json:"id"`
	UserId int    `xorm:"user_id notnull default(0) comment('user id')" json:"user_id"`
	Name   string `xorm:"name notnull default('') comment('rule name')" json:"name"`
	Value  string `xorm:"value text comment('rule content')" json:"value"`
	Action int    `xorm:"action notnull default(0) comment('action to run: 1 read, 2 forward, 3 delete')" json:"action"`
	Params string `xorm:"params notnull default('') comment('action parameters')" json:"params"`
	Sort   int    `xorm:"sort notnull default(0) comment('sort order, higher runs first')" json:"sort"`
}

func (p *Rule) TableName() string {
	return "rule"
}
