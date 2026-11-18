package imap_server

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/services/group"
)

func (s *serverSession) Rename(mailbox, newName string) error {
	if group.IsDefaultBox(mailbox) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "This mailbox does not support rename.",
		}
	}

	groupPath := strings.Split(mailbox, "/")

	oldGroupName := groupPath[len(groupPath)-1]

	newGroupPath := strings.Split(newName, "/")

	newGroupName := newGroupPath[len(newGroupPath)-1]

	err := group.Rename(s.ctx, oldGroupName, newGroupName)
	if err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: err.Error(),
		}
	}
	return nil
}
