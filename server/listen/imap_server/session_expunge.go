package imap_server

import (
	"log/slog"
	"sort"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/noelbennettz/imail/services/del_email"
	"github.com/noelbennettz/imail/services/list"
	"github.com/spf13/cast"
)

func (s *serverSession) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	if uids == nil && len(s.deleteUidList) == 0 {
		return nil
	}
	uidList := []int{}

	if uids != nil {
		for _, uidRange := range *uids {
			if uidRange.Start > 0 && uidRange.Stop > 0 {
				if uidRange.Stop-uidRange.Start > 10000 {
					return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "UID range is too large"}
				}
				for i := uidRange.Start; i <= uidRange.Stop; i++ {
					uidList = append(uidList, cast.ToInt(uint32(i)))
				}
			}
		}
	}

	if len(s.deleteUidList) > 0 {
		uidList = append(uidList, s.deleteUidList...)
	}

	if len(uidList) == 0 {
		return nil
	}

	// Resolve sequence numbers before deleting so the client gets the untagged
	// EXPUNGE responses it expects. Only messages that are still in the
	// selected mailbox are deleted, so a stale UID from another mailbox cannot
	// remove a message the client is not looking at.
	var serials []uint32
	var pending []int
	rows := list.GetEmailListByGroup(s.ctx, s.currentMailbox, list.ImapListReq{UidList: uidList}, true)
	for _, row := range rows {
		pending = append(pending, row.UeId)
		serials = append(serials, cast.ToUint32(row.SerialNumber))
	}
	if len(pending) == 0 {
		s.deleteUidList = []int{}
		return nil
	}

	slog.Debug("DeleteUidList:", slog.Any("uidList", pending))

	if err := del_email.DelByUID(s.ctx, pending); err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: err.Error(),
		}
	}
	s.deleteUidList = []int{}

	sort.Slice(serials, func(i, j int) bool { return serials[i] > serials[j] })
	for _, seq := range serials {
		if err := w.WriteExpunge(seq); err != nil {
			return err
		}
	}

	return nil
}
