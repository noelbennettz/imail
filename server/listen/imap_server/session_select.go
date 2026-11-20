package imap_server

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/services/group"
	"github.com/spf13/cast"
)

func (s *serverSession) Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error) {
	mailbox = strings.Trim(strings.TrimSpace(mailbox), `"`)
	if strings.EqualFold(mailbox, "INBOX") {
		mailbox = "INBOX"
	}
	if mailbox == "" {
		return nil, &imap.Error{
			Type: imap.StatusResponseTypeBad,
			Text: "mailbox not found",
		}
	}
	if !group.IsDefaultBox(mailbox) {
		box, err := group.ResolveBox(s.ctx, mailbox)
		if err != nil || box == nil || box.ID == 0 {
			return nil, &imap.Error{
				Type: imap.StatusResponseTypeNo,
				Text: "Mailbox does not exist",
			}
		}
		mailbox = box.FullPath
	}
	s.currentMailbox = mailbox
	_, data := group.GetGroupStatus(s.ctx, s.currentMailbox, []string{"MESSAGES", "UNSEEN", "UIDNEXT", "UIDVALIDITY"})

	ret := &imap.SelectData{
		Flags:          []imap.Flag{imap.FlagSeen},
		PermanentFlags: []imap.Flag{imap.FlagSeen},
		NumMessages:    cast.ToUint32(data["MESSAGES"]),
		UIDNext:        imap.UID(data["UIDNEXT"]),
		UIDValidity:    cast.ToUint32(data["UIDVALIDITY"]),
	}

	return ret, nil
}
