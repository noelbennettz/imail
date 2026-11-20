package imap_server

import (
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/detail"
	"github.com/noelbennettz/imail/services/list"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/spf13/cast"
)

func (s *serverSession) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	var emailList []*response.EmailResponseData

	switch set := numSet.(type) {
	case imap.SeqSet:
		for _, seq := range set {
			res := list.GetEmailListByGroup(s.ctx, s.currentMailbox, list.ImapListReq{
				Star: cast.ToInt(seq.Start),
				End:  cast.ToInt(seq.Stop),
			}, false)
			emailList = append(emailList, res...)
		}

	case imap.UIDSet:
		for _, uid := range set {
			res := list.GetEmailListByGroup(s.ctx, s.currentMailbox, list.ImapListReq{
				Star: cast.ToInt(uint32(uid.Start)),
				End:  cast.ToInt(uint32(uid.Stop)),
			}, true)
			emailList = append(emailList, res...)
		}
	}

	seen := array.InArray(imap.FlagSeen, flags.Flags)
	deleted := array.InArray(imap.FlagDeleted, flags.Flags)
	seenList := map[int]struct{}{}

	for _, data := range emailList {
		if _, duplicate := seenList[data.UeId]; duplicate {
			continue
		}
		seenList[data.UeId] = struct{}{}
		read := data.IsRead == 1

		switch flags.Op {
		case imap.StoreFlagsAdd:
			if seen {
				read = true
			}
			if deleted {
				s.addDeleteUID(data.UeId)
			}
		case imap.StoreFlagsDel:
			if seen {
				read = false
			}
			if deleted {
				s.removeDeleteUID(data.UeId)
			}
		case imap.StoreFlagsSet:
			read = seen
			if deleted {
				s.addDeleteUID(data.UeId)
			} else {
				s.removeDeleteUID(data.UeId)
			}
		}

		if read != (data.IsRead == 1) {
			detail.MakeRead(s.ctx, data.UeId, read)
		}

		if !flags.Silent {
			writer := w.CreateMessage(cast.ToUint32(data.SerialNumber))
			writer.WriteUID(imap.UID(data.UeId))
			writer.WriteFlags(s.currentFlags(data.UeId, read))
			writer.Close()
		}
	}
	return nil
}

func (s *serverSession) addDeleteUID(uid int) {
	for _, existing := range s.deleteUidList {
		if existing == uid {
			return
		}
	}
	s.deleteUidList = append(s.deleteUidList, uid)
}

func (s *serverSession) removeDeleteUID(uid int) {
	for i, existing := range s.deleteUidList {
		if existing == uid {
			s.deleteUidList = append(s.deleteUidList[:i], s.deleteUidList[i+1:]...)
			return
		}
	}
}

func (s *serverSession) currentFlags(uid int, read bool) []imap.Flag {
	flags := []imap.Flag{}
	if read {
		flags = append(flags, imap.FlagSeen)
	}
	for _, deferred := range s.deleteUidList {
		if deferred == uid {
			flags = append(flags, imap.FlagDeleted)
			break
		}
	}
	return flags
}
