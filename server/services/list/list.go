package list

import (
	"fmt"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/group"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)
import . "xorm.io/builder"

var emailColumnNames = []string{
	"id", "type", "subject", "reply_to", "from_name", "from_address", "to", "bcc", "cc",
	"text", "html", "sender", "attachments", "spf_check", "dkim_check", "status",
	"cron_send_time", "update_time", "send_user_id", "size", "error", "send_date", "create_time", "msg_id",
}

func quoteIdentifier(name string) string {
	if config.Instance != nil && config.Instance.DbType == config.DBTypeMySQL {
		return "`" + name + "`"
	}
	return `"` + name + `"`
}

func emailColumns() string {
	quoted := make([]string, 0, len(emailColumnNames))
	for _, name := range emailColumnNames {
		quoted = append(quoted, quoteIdentifier(name))
	}
	return strings.Join(quoted, ",")
}

func GetEmailList(ctx *context.Context, tagInfo dto.SearchTag, keyword string, pop3List bool, offset, limit int) (emailList []*response.EmailResponseData, total int64) {
	return getList(ctx, tagInfo, keyword, pop3List, offset, limit)
}

func getList(ctx *context.Context, tagInfo dto.SearchTag, keyword string, pop3List bool, offset, limit int) (emailList []*response.EmailResponseData, total int64) {
	querySQL, queryParams := genSQL(ctx, false, tagInfo, keyword, pop3List, offset, limit)

	err := db.Instance.SQL(querySQL, queryParams...).Find(&emailList)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL ERROR: %s ,Error:%s", querySQL, err)
	}

	totalSQL, totalParams := genSQL(ctx, true, tagInfo, keyword, pop3List, offset, limit)

	_, err = db.Instance.SQL(totalSQL, totalParams...).Get(&total)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL ERROR: %s ,Error:%s", querySQL, err)
	}

	return emailList, total
}

func genSQL(ctx *context.Context, count bool, tagInfo dto.SearchTag, keyword string, pop3List bool, offset, limit int) (string, []any) {
	sqlParams := []any{ctx.UserID}
	sql := "select "

	if count {
		sql += `count(1) from email e left join user_email ue on e.id=ue.email_id where ue.user_id = ? `
	} else if pop3List {
		sql += `e.id,e.size from email e left join user_email ue on e.id=ue.email_id where ue.user_id = ? `
	} else {
		// Do not select raw_mime or other large columns for list rows.
		sql += `e.id,e.type,e.subject,e.from_name,e.from_address,` + "e." + quoteIdentifier("to") + `,e.sender,e.text,e.size,
			e.spf_check,e.dkim_check,e.error,e.send_date,e.create_time,e.status,ue.is_read
			from email e left join user_email ue on e.id=ue.email_id where ue.user_id = ? `
	}

	if tagInfo.Status != -1 {
		switch tagInfo.Status {
		case consts.EmailStatusDel:
			sql += " and (ue.status =? or ue.group_id=?) "
			sqlParams = append(sqlParams, tagInfo.Status, models.Deleted)
		case consts.EmailStatusDrafts:
			sql += " and (ue.status =? or ue.group_id=?) "
			sqlParams = append(sqlParams, tagInfo.Status, models.Drafts)
		case consts.EmailStatusJunk:
			sql += " and (ue.status =? or ue.group_id=?) "
			sqlParams = append(sqlParams, tagInfo.Status, models.Junk)
		}
	} else if tagInfo.Status == -1 {
		if tagInfo.Type != 1 {
			sql += " and ue.status = 0"
		} else {
			// The outbox does not show deleted emails
			sql += " and ue.status != 3"
		}
	}

	if tagInfo.Type == consts.EmailTypeReceive {
		sql += " and type =? "
		sqlParams = append(sqlParams, tagInfo.Type)
	} else if tagInfo.Type == consts.EmailTypeSend {
		sql += " and (type =? or ue.group_id=?)"
		sqlParams = append(sqlParams, tagInfo.Type, models.Sent)
	}

	if tagInfo.GroupId != -1 {
		if tagInfo.GroupId > 0 {
			sql += " and ue.group_id=? "
			sqlParams = append(sqlParams, tagInfo.GroupId)
		} else if tagInfo.GroupId == 0 && tagInfo.Status == -1 {
			sql += " and (ue.group_id=? or ue.group_id=0)"
			sqlParams = append(sqlParams, models.INBOX)
		}
	} else {
		sql += " and (ue.group_id=0 or ue.group_id =?) "
		sqlParams = append(sqlParams, models.INBOX)
	}

	if keyword != "" {
		sql += " and (subject like ? or text like ? )"
		sqlParams = append(sqlParams, "%"+keyword+"%", "%"+keyword+"%")
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	sql += " order by e.id desc"
	sql += fmt.Sprintf(" LIMIT %d OFFSET %d ", limit, offset)

	return sql, sqlParams
}

type statRes struct {
	Total int64
	Size  int64
}

// Stat queries the total number and size of emails
func Stat(ctx *context.Context) (int64, int64) {
	sql := `select count(1) as total,sum(size) as size from email e left join user_email ue on e.id=ue.email_id where ue.user_id = ? and e.type = 0 and ue.status != 3`
	var ret statRes
	_, err := db.Instance.SQL(sql, ctx.UserID).Get(&ret)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL ERROR: %s ,Error:%s", sql, err)
	}
	return ret.Total, ret.Size
}

type ImapListReq struct {
	UidList []int
	Star    int
	End     int
}

func GetUEListByUID(ctx *context.Context, groupName string, star, end int, uidList []int) []*response.UserEmailUIDData {
	var ue []*response.UserEmailUIDData
	sql, params, ok := userEmailRowsSQL(ctx, groupName, true, ImapListReq{UidList: uidList, Star: star, End: end})
	if !ok {
		return nil
	}
	if err := db.Instance.SQL(sql, params...).Find(&ue); err != nil {
		log.WithContext(ctx).Errorf("SQL ERROR: %s ,Error:%s", sql, err)
	}
	return ue
}

// userEmailRowsSQL returns the SELECT used by the IMAP list paths. byUID picks
// the UID (user_email.id) semantics; otherwise Star/End/UidList are sequence
// numbers within the mailbox. The table name is quoted per dialect.
func userEmailRowsSQL(ctx *context.Context, groupName string, byUID bool, req ImapListReq) (string, []any, bool) {
	boxSQL, boxArgs, ok := boxFilterSQL(ctx, groupName)
	if !ok {
		return "", nil, false
	}
	table := quoteIdentifier("user_email")
	params := []any{ctx.UserID}
	params = append(params, boxArgs...)

	if byUID {
		sql := "SELECT id,email_id, is_read, ROW_NUMBER() OVER (ORDER BY id) AS serial_number FROM " + table + " WHERE user_id = ? AND " + boxSQL
		if len(req.UidList) > 0 {
			sql += " AND id IN (" + placeholders(len(req.UidList)) + ")"
			params = append(params, intsToAny(req.UidList)...)
		} else {
			if req.Star > 0 {
				sql += " AND id >= ?"
				params = append(params, req.Star)
			}
			if req.End > 0 {
				sql += " AND id <= ?"
				params = append(params, req.End)
			}
		}
		return sql, params, true
	}

	sql := "SELECT * FROM (SELECT id,email_id, is_read, ROW_NUMBER() OVER (ORDER BY id) AS serial_number FROM " + table + " WHERE user_id = ? AND " + boxSQL + ") a"
	if len(req.UidList) > 0 {
		sql += " WHERE serial_number IN (" + placeholders(len(req.UidList)) + ")"
		params = append(params, intsToAny(req.UidList)...)
	} else {
		var conditions []string
		if req.Star > 0 {
			conditions = append(conditions, "serial_number >= ?")
			params = append(params, req.Star)
		}
		if req.End > 0 {
			conditions = append(conditions, "serial_number <= ?")
			params = append(params, req.End)
		}
		if len(conditions) > 0 {
			sql += " WHERE " + strings.Join(conditions, " AND ")
		}
	}
	return sql, params, true
}

// boxFilterSQL maps an IMAP mailbox name to the user_email predicate. Default
// mailboxes keep the legacy group_id values working as well as the status
// column, and custom mailboxes are resolved by their full path.
func boxFilterSQL(ctx *context.Context, groupName string) (string, []any, bool) {
	switch strings.Trim(strings.TrimSpace(groupName), "/") {
	case "INBOX":
		return "(status = ? AND (group_id = 0 OR group_id = ?))", []any{0, models.INBOX}, true
	case "Sent Messages":
		return "((status = ? AND group_id = 0) OR group_id = ?)", []any{1, models.Sent}, true
	case "Drafts":
		return "((status = ? AND group_id = 0) OR group_id = ?)", []any{4, models.Drafts}, true
	case "Deleted Messages":
		return "((status = ? AND group_id = 0) OR group_id = ?)", []any{3, models.Deleted}, true
	case "Junk":
		return "((status = ? AND group_id = 0) OR group_id = ?)", []any{5, models.Junk}, true
	}
	box, err := group.ResolveBox(ctx, groupName)
	if err != nil || box == nil || box.ID == 0 {
		return "", nil, false
	}
	return "group_id = ?", []any{box.ID}, true
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func intsToAny(values []int) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}

func getEmailListByUidList(ctx *context.Context, groupName string, req ImapListReq, uid bool) []*response.EmailResponseData {
	sql, params, ok := userEmailRowsSQL(ctx, groupName, uid, req)
	if !ok {
		return nil
	}
	var ue []*response.UserEmailUIDData
	if err := db.Instance.SQL(sql, params...).Find(&ue); err != nil {
		log.WithContext(ctx).Errorf("SQL ERROR: %s ,Error:%s", sql, err)
		return nil
	}
	return joinEmails(ue)
}

// joinEmails loads the email rows for the user_email rows returned by IMAP and
// keeps the per-mailbox sequence number and UID.
func joinEmails(ue []*response.UserEmailUIDData) []*response.EmailResponseData {
	var ret []*response.EmailResponseData
	ueMap := map[int]*response.UserEmailUIDData{}
	var emailIds []int
	for _, email := range ue {
		ueMap[email.EmailID] = email
		emailIds = append(emailIds, email.EmailID)
	}
	if len(emailIds) == 0 {
		return ret
	}
	_ = db.Instance.Table("email").Select(emailColumns()).Where(Eq{"id": emailIds}).Find(&ret)
	for i, data := range ret {
		mapped := ueMap[data.Id]
		if mapped == nil {
			continue
		}
		ret[i].IsRead = mapped.IsRead
		ret[i].SerialNumber = mapped.SerialNumber
		ret[i].UeId = mapped.ID
	}
	return ret
}

func GetEmailListByGroup(ctx *context.Context, groupName string, req ImapListReq, uid bool) []*response.EmailResponseData {
	if len(req.UidList) == 0 && req.Star == 0 && req.End == 0 {
		return nil
	}
	return getEmailListByUidList(ctx, groupName, req, uid)
}
