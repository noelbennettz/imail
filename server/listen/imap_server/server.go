package imap_server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/group"
	"github.com/noelbennettz/imail/services/quota"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/id"
	"xorm.io/xorm"
)

// Server is a server instance.
//
// A server contains a list of users.
type Server struct{}

// NewServer creates a new server.
func NewServer() *Server {
	return &Server{}
}

type Status int8

const (
	UNAUTHORIZED Status = 1
	AUTHORIZED   Status = 2
	SELECTED     Status = 3
	LOGOUT       Status = 4
)

type serverSession struct {
	server         *Server // immutable
	ctx            *context.Context
	status         Status
	currentMailbox string
	deleteUidList  []int
	// remoteAddr is the client address without port, used to key the
	// failed sign-in limiter.
	remoteAddr string
}

// NewSession creates a new IMAP session.
func (s *Server) NewSession() imapserver.Session {
	return s.newSession("")
}

// newSession creates a session that remembers the client address.
func (s *Server) newSession(remoteAddr string) imapserver.Session {
	tc := &context.Context{}
	tc.SetValue(context.LogID, id.GenLogID())

	return &serverSession{
		server:     s,
		ctx:        tc,
		remoteAddr: remoteAddr,
	}
}

func (s *serverSession) Close() error {
	return nil
}

func (s *serverSession) Subscribe(mailbox string) error {
	return nil
}

func (s *serverSession) Unsubscribe(mailbox string) error {
	return nil
}

func (s *serverSession) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	mailbox = strings.Trim(strings.TrimSpace(mailbox), `"`)
	if mailbox == "" {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
	}

	status := consts.EmailStatusWait
	groupID := 0
	if !group.IsDefaultBox(mailbox) {
		box, err := group.ResolveBox(s.ctx, mailbox)
		if err != nil || box == nil || box.ID == 0 {
			return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
		}
		mailbox = box.FullPath
		groupID = box.ID
	} else {
		switch mailbox {
		case "Sent Messages":
			status = consts.EmailStatusSent
		case "Drafts":
			status = consts.EmailStatusDrafts
		case "Deleted Messages":
			status = consts.EmailStatusDel
		case "Junk":
			status = consts.EmailStatusJunk
		}
	}

	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Unable to read message"}
	}
	email := parsemail.NewEmailFromReader(nil, bytes.NewReader(raw), len(raw))
	var owner models.User
	if _, err := db.Instance.ID(s.ctx.UserID).Get(&owner); err == nil {
		if err := quota.Check([]*models.User{&owner}, len(raw)); err != nil {
			return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox quota exceeded"}
		}
	}
	domain := config.Instance.Domain
	if s.ctx.UserDomain != "" {
		domain = s.ctx.UserDomain
	}
	if email.From == nil || email.From.EmailAddress == "" {
		email.From = &parsemail.User{Name: s.ctx.UserName, EmailAddress: s.ctx.UserAccount + "@" + domain}
	}
	if email.Sender == nil {
		email.Sender = email.From
	}
	msgID := email.MsgID
	if msgID == "" {
		msgID = parsemail.GenerateMsgID(domain)
	}
	now := time.Now()
	if options != nil && !options.Time.IsZero() {
		now = options.Time
	}
	modelEmail := models.Email{
		Type:         consts.EmailTypeReceive,
		Subject:      email.Subject,
		ReplyTo:      jsonString(email.ReplyTo),
		FromName:     email.From.Name,
		FromAddress:  email.From.EmailAddress,
		To:           jsonString(email.To),
		Bcc:          jsonString(email.Bcc),
		Cc:           jsonString(email.Cc),
		Text:         sql.NullString{String: string(email.Text), Valid: true},
		Html:         sql.NullString{String: string(email.HTML), Valid: true},
		Sender:       jsonString(email.Sender),
		Attachments:  jsonString(email.Attachments),
		RawMIME:      string(raw),
		Size:         len(raw),
		SPFCheck:     1,
		DKIMCheck:    1,
		SendUserID:   s.ctx.UserID,
		SendDate:     now,
		CronSendTime: now,
		Status:       status,
		CreateTime:   now,
		MsgID:        msgID,
	}
	if status == consts.EmailStatusSent {
		modelEmail.Type = consts.EmailTypeSend
	}

	var inserted models.UserEmail
	err = db.Transaction(db.Instance, func(session *xorm.Session) error {
		if _, err := session.Insert(&modelEmail); err != nil {
			return err
		}
		inserted = models.UserEmail{
			UserID:  s.ctx.UserID,
			EmailID: modelEmail.Id,
			Status:  status,
			GroupId: groupID,
			IsRead:  0,
		}
		if options != nil && array.InArray(imap.FlagSeen, options.Flags) {
			inserted.IsRead = 1
		}
		_, err := session.Insert(&inserted)
		return err
	})
	if err != nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Unable to store message"}
	}

	_, data := group.GetGroupStatus(s.ctx, mailbox, []string{"UIDVALIDITY"})
	return &imap.AppendData{
		UID:         imap.UID(inserted.ID),
		UIDValidity: uint32(data["UIDVALIDITY"]),
	}, nil
}

func jsonString(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func (s *serverSession) Unselect() error {
	s.currentMailbox = ""
	return nil
}
