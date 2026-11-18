package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Email struct {
	Id           int            `xorm:"id pk unsigned int autoincr notnull" json:"id"`
	Type         int8           `xorm:"type tinyint(4) notnull default(0) comment('email type, 0: received, 1: sent')" json:"type"`
	Subject      string         `xorm:"subject varchar(1000) notnull default('') comment('email subject')" json:"subject"`
	ReplyTo      string         `xorm:"reply_to text comment('reply-to')" json:"reply_to"`
	FromName     string         `xorm:"from_name varchar(50) notnull default('') comment('sender name')" json:"from_name"`
	FromAddress  string         `xorm:"from_address varchar(100) notnull default('') comment('sender email address')" json:"from_address"`
	To           string         `xorm:"to text comment('recipient addresses')" json:"to"`
	Bcc          string         `xorm:"bcc text comment('bcc')" json:"bcc"`
	Cc           string         `xorm:"cc text comment('cc')" json:"cc"`
	Text         sql.NullString `xorm:"text text comment('plain text content')" json:"text"`
	Html         sql.NullString `xorm:"html mediumtext comment('html content')" json:"html"`
	Sender       string         `xorm:"sender text comment('sender')" json:"sender"`
	Attachments  string         `xorm:"attachments longtext comment('attachments')" json:"attachments"`
	RawMIME      string         `xorm:"raw_mime longtext comment('raw MIME source')" json:"-"`
	SPFCheck     int8           `xorm:"spf_check tinyint(1) comment('whether spf validation passed')" json:"spf_check"`
	DKIMCheck    int8           `xorm:"dkim_check tinyint(1) comment('whether dkim validation passed')" json:"dkim_check"`
	Status       int8           `xorm:"status tinyint(4) notnull default(0) comment('0 not sent, 1 sent, 2 send failed')" json:"status"` // 0 = not sent, 1 = sent, 2 = send failed
	CronSendTime time.Time      `xorm:"cron_send_time comment('scheduled send time')" json:"cron_send_time"`
	UpdateTime   time.Time      `xorm:"update_time updated comment('update time')" json:"update_time"`
	SendUserID   int            `xorm:"send_user_id unsigned int  notnull default(0) comment('sender user id')" json:"send_user_id"`
	Size         int            `xorm:"size unsigned int  notnull default(1000) comment('email size')" json:"size"`
	Error        sql.NullString `xorm:"error text comment('delivery error message')" json:"error"`
	SendDate     time.Time      `xorm:"send_date comment('delivery time')" json:"send_date"`
	CreateTime   time.Time      `xorm:"create_time created" json:"create_time"`
	MsgID        string         `xorm:"msg_id varchar(255) notnull default('') comment('RFC-compliant Message-ID, generated once on creation')" json:"msg_id"`
}

func (d *Email) TableName() string {
	return "email"
}

type attachments struct {
	Filename    string
	ContentType string
	Index       int
	//Content     []byte
}

func (d *Email) MarshalJSON() ([]byte, error) {
	type Alias Email

	var allAtt = []attachments{}
	var showAtt = []attachments{}
	if d.Attachments != "" {
		_ = json.Unmarshal([]byte(d.Attachments), &allAtt)
		for i, att := range allAtt {
			att.Index = i
			//if att.ContentType == "application/octet-stream" {
			showAtt = append(showAtt, att)
			//}

		}
	}

	return json.Marshal(&struct {
		Alias
		CronSendTime string        `json:"send_time"`
		SendDate     string        `json:"send_date"`
		UpdateTime   string        `json:"update_time"`
		CreateTime   string        `json:"create_time"`
		Text         string        `json:"text"`
		Html         string        `json:"html"`
		Error        string        `json:"error"`
		Attachments  []attachments `json:"attachments"`
	}{
		Alias:        (Alias)(*d),
		CronSendTime: d.CronSendTime.UTC().Format(time.RFC3339),
		UpdateTime:   d.UpdateTime.UTC().Format(time.RFC3339),
		CreateTime:   d.CreateTime.UTC().Format(time.RFC3339),
		SendDate:     d.SendDate.UTC().Format(time.RFC3339),
		Text:         d.Text.String,
		Html:         d.Html.String,
		Error:        d.Error.String,
		Attachments:  showAtt,
	})
}
