package detail

import (
	"regexp"
	"sort"
	"strings"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
)

var threadSubjectPrefix = regexp.MustCompile(`^(?i)((re|fwd|fw|aw|wg|回复|回覆|答复|轉寄|转发)\s*:\s*)+`)

// NormalizeThreadSubject mirrors the list view's subject-based grouping so the
// detail page can collect every message that belongs to the same conversation.
func NormalizeThreadSubject(subject string) string {
	subject = threadSubjectPrefix.ReplaceAllString(strings.TrimSpace(subject), "")
	return strings.ToLower(strings.TrimSpace(subject))
}

type threadCandidate struct {
	ID      int    `xorm:"id"`
	Subject string `xorm:"subject"`
	Status  int    `xorm:"status"`
}

// GetEmailThread returns a page of messages in the conversation that contains
// the requested email. Messages are ordered oldest first. total is the full
// conversation size so the client can decide whether to request another page.
func GetEmailThread(ctx *context.Context, id int, markRead bool, offset, limit int) ([]*response.EmailResponseData, int, error) {
	anchor, err := GetEmailDetail(ctx, id, markRead)
	if err != nil {
		return nil, 0, err
	}
	base := NormalizeThreadSubject(anchor.Subject)
	if base == "" {
		return []*response.EmailResponseData{anchor}, 1, nil
	}

	var anchorStatus struct {
		Status int `xorm:"status"`
	}
	_, _ = db.Instance.Table("user_email").Where("email_id = ? AND user_id = ?", id, ctx.UserID).Get(&anchorStatus)
	includeDeleted := anchorStatus.Status == 3

	pattern := "%" + escapeLike(base) + "%"
	candidates := make([]threadCandidate, 0, 8)
	err = db.Instance.SQL(
		`SELECT e.id, e.subject, ue.status FROM email e
		 JOIN user_email ue ON e.id = ue.email_id
		 WHERE ue.user_id = ? AND LOWER(e.subject) LIKE LOWER(?) ESCAPE '\'
		 ORDER BY e.id ASC`, ctx.UserID, pattern).Find(&candidates)
	if err != nil {
		return nil, 0, errors.Wrap(err)
	}

	ids := make([]int, 0, len(candidates))
	seen := map[int]bool{id: true}
	ids = append(ids, id)
	for _, candidate := range candidates {
		if !includeDeleted && candidate.Status == 3 {
			continue
		}
		if NormalizeThreadSubject(candidate.Subject) != base {
			continue
		}
		if seen[candidate.ID] {
			continue
		}
		seen[candidate.ID] = true
		ids = append(ids, candidate.ID)
	}
	sort.Ints(ids)

	total := len(ids)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 5
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}

	messages := make([]*response.EmailResponseData, 0, end-offset)
	for _, messageID := range ids[offset:end] {
		if messageID == id {
			messages = append(messages, anchor)
			continue
		}
		message, detailErr := GetEmailDetail(ctx, messageID, markRead)
		if detailErr != nil {
			continue
		}
		messages = append(messages, message)
	}
	return messages, total, nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}
