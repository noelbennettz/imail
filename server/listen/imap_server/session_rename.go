package imap_server

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/services/group"
)

func (s *serverSession) Rename(mailbox, newName string) error {
	mailbox = strings.Trim(strings.TrimSpace(mailbox), `"`)
	newName = strings.Trim(strings.TrimSpace(newName), `"`)
	if group.IsDefaultBox(mailbox) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "This mailbox does not support rename.",
		}
	}

	if err := group.RenameByPath(s.ctx, mailbox, newName); err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: err.Error(),
		}
	}
	return nil
}
