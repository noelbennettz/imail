package imap_server

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/services/group"
)

func (s *serverSession) Create(mailbox string, options *imap.CreateOptions) error {
	mailbox = strings.Trim(strings.TrimSpace(mailbox), `"`)
	if mailbox == "" || group.IsDefaultBox(mailbox) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "Mailbox already exists",
		}
	}
	groupPath := strings.Split(mailbox, "/")

	var parentId int
	for _, path := range groupPath {
		newGroup, err := group.CreateGroup(s.ctx, path, parentId)
		if err != nil {
			return &imap.Error{
				Type: imap.StatusResponseTypeNo,
				Text: err.Error(),
			}
		}
		parentId = newGroup.ID
	}

	return nil
}
