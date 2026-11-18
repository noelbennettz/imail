package imap_server

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/services/group"
)

func (s *serverSession) Delete(mailbox string) error {
	groupPath := strings.Split(mailbox, "/")

	groupName := groupPath[len(groupPath)-1]
	groupInfo, err := group.GetGroupByName(s.ctx, groupName)
	if err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: err.Error(),
		}
	}
	_, err = group.DelGroup(s.ctx, groupInfo.ID)
	if err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: err.Error(),
		}
	}

	return nil
}
