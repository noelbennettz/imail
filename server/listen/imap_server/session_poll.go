package imap_server

import (
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/noelbennettz/imail/services/group"
	"github.com/spf13/cast"
)

func (s *serverSession) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	mailbox := s.currentMailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}
	_, data := group.GetGroupStatus(s.ctx, mailbox, []string{"MESSAGES"})
	w.WriteNumMessages(cast.ToUint32(data["MESSAGES"]))
	return nil
}
