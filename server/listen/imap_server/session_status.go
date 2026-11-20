package imap_server

import (
	"github.com/emersion/go-imap/v2"
	"github.com/noelbennettz/imail/services/group"
	"github.com/spf13/cast"
)

func (s *serverSession) Status(mailbox string, options *imap.StatusOptions) (*imap.StatusData, error) {
	if options == nil {
		options = &imap.StatusOptions{}
	}
	category := []string{}
	if options.UIDNext {
		category = append(category, "UIDNEXT")
	}
	if options.NumMessages {
		category = append(category, "MESSAGES")
	}
	if options.UIDValidity {
		category = append(category, "UIDVALIDITY")
	}
	if options.NumUnseen {
		category = append(category, "UNSEEN")
	}

	_, data := group.GetGroupStatus(s.ctx, mailbox, category)

	ret := &imap.StatusData{Mailbox: mailbox}
	if options.NumMessages {
		value := cast.ToUint32(data["MESSAGES"])
		ret.NumMessages = &value
	}
	if options.NumUnseen {
		value := cast.ToUint32(data["UNSEEN"])
		ret.NumUnseen = &value
	}
	if options.UIDNext {
		ret.UIDNext = imap.UID(cast.ToUint32(data["UIDNEXT"]))
	}
	if options.UIDValidity {
		ret.UIDValidity = cast.ToUint32(data["UIDVALIDITY"])
	}

	return ret, nil
}
