package imap_server

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/services/group"
)

func (s *serverSession) Delete(mailbox string) error {
	mailbox = strings.Trim(strings.TrimSpace(mailbox), `"`)
	if group.IsDefaultBox(mailbox) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "This mailbox does not support delete.",
		}
	}
	groupInfo, err := group.ResolveBox(s.ctx, mailbox)
	if err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: err.Error(),
		}
	}
	if groupInfo == nil || groupInfo.ID == 0 {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "Mailbox does not exist",
		}
	}
	if _, err := group.DelGroup(s.ctx, groupInfo.ID); err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: err.Error(),
		}
	}

	return nil
}
