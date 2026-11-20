package imap_server

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

var defaultMailboxes = []struct {
	name  string
	attrs []imap.MailboxAttr
}{
	{"INBOX", []imap.MailboxAttr{imap.MailboxAttrHasNoChildren}},
	{"Sent Messages", []imap.MailboxAttr{imap.MailboxAttrHasNoChildren, imap.MailboxAttrSent}},
	{"Drafts", []imap.MailboxAttr{imap.MailboxAttrHasNoChildren, imap.MailboxAttrDrafts}},
	{"Deleted Messages", []imap.MailboxAttr{imap.MailboxAttrHasNoChildren, imap.MailboxAttrTrash}},
	{"Junk", []imap.MailboxAttr{imap.MailboxAttrHasNoChildren, imap.MailboxAttrJunk}},
}

// mailboxMatch implements the IMAP wildcard rules: % matches anything except
// the hierarchy delimiter, * matches everything. INBOX is case-insensitive.
func mailboxMatch(name, pattern string) bool {
	return matchRunes([]rune(name), []rune(pattern), func(a, b rune) bool {
		return strings.EqualFold(string(a), string(b))
	})
}

func matchRunes(name, pattern []rune, equal func(a, b rune) bool) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	switch pattern[0] {
	case '*':
		for i := 0; i <= len(name); i++ {
			if matchRunes(name[i:], pattern[1:], equal) {
				return true
			}
		}
		return false
	case '%':
		for i := 0; i <= len(name); i++ {
			if i > 0 && name[i-1] == '/' {
				break
			}
			if matchRunes(name[i:], pattern[1:], equal) {
				return true
			}
		}
		return false
	default:
		if len(name) == 0 || !equal(name[0], pattern[0]) {
			return false
		}
		return matchRunes(name[1:], pattern[1:], equal)
	}
}

func mailboxHasChildren(ctx *context.Context, name string) bool {
	var groups []*models.Group
	if err := db.Instance.Table("group").Where("user_id=? and full_path LIKE ?", ctx.UserID, name+"/%").Limit(1).Find(&groups); err != nil {
		return false
	}
	return len(groups) > 0
}

func writeMailbox(w *imapserver.ListWriter, ctx *context.Context, name string, attrs []imap.MailboxAttr) {
	if mailboxHasChildren(ctx, name) {
		attrs = append(attrs, imap.MailboxAttrHasChildren)
	}
	w.WriteList(&imap.ListData{
		Attrs:   attrs,
		Delim:   '/',
		Mailbox: name,
	})
}

func (s *serverSession) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	log.WithContext(s.ctx).Debugf("imap server list, ref: %s ,patterns: %s ", ref, patterns)

	ref = strings.TrimSuffix(strings.TrimSpace(ref), "/")
	patterns = append([]string{}, patterns...)
	if len(patterns) == 0 {
		patterns = []string{""}
	}

	var custom []*models.Group
	if err := db.Instance.Table("group").Where("user_id=?", s.ctx.UserID).Find(&custom); err != nil {
		return err
	}
	written := map[string]bool{}

	for _, pattern := range patterns {
		combined := strings.TrimSpace(pattern)
		if ref != "" {
			if strings.HasPrefix(combined, "/") {
				combined = ref + combined
			} else {
				combined = ref + "/" + combined
			}
		}
		for _, box := range defaultMailboxes {
			if mailboxMatch(box.name, combined) {
				if !written[box.name] {
					written[box.name] = true
					writeMailbox(w, s.ctx, box.name, append([]imap.MailboxAttr{}, box.attrs...))
				}
			}
		}
		for _, item := range custom {
			if mailboxMatch(item.FullPath, combined) {
				if !written[item.FullPath] {
					written[item.FullPath] = true
					writeMailbox(w, s.ctx, item.FullPath, nil)
				}
			}
		}
	}
	return nil
}
