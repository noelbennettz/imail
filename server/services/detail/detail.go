package detail

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	log "github.com/sirupsen/logrus"
)

func GetEmailDetail(ctx *context.Context, id int, markRead bool) (*response.EmailResponseData, error) {
	// Check whether the email belongs to the current user
	var ue models.UserEmail
	_, err := db.Instance.Where("email_id = ? AND user_id = ?", id, ctx.UserID).Get(&ue)
	if err != nil {
		log.Error(err)
	}
	if ue.ID == 0 && !ctx.IsAdmin {
		return nil, errors.New("Not authorized")
	}

	// Fetch the email content
	var email response.EmailResponseData
	_, err = db.Instance.Select("*,1 as is_read").Table("email").Where("id=?", id).Get(&email)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL error:%+v", err)
		return nil, err
	}

	email.IsRead = ue.IsRead

	if markRead && ue.IsRead == 0 {
		ue.IsRead = 1
		_, err = db.Instance.Where("id=?", ue.ID).Update(&ue)
		if err != nil {
			log.WithContext(ctx).Errorf("SQL error:%+v", err)
		}
	}

	// Replace cid references in the content with urls
	if email.Attachments != "" {
		var atts []parsemail.Attachment
		_ = json.Unmarshal([]byte(email.Attachments), &atts)
		for _, att := range atts {
			email.Html = sql.NullString{
				String: strings.ReplaceAll(email.Html.String, fmt.Sprintf("cid:%s", att.ContentID), fmt.Sprintf("/attachments/%d/%s", id, att.ContentID)),
			}
		}
	}

	return &email, nil
}

// MakeRead changes one mailbox copy (user_email.id), not every copy of the same
// message, so reading a folder copy does not mark another folder's copy read.
func MakeRead(ctx *context.Context, uid int, read bool) {
	value := 0
	if read {
		value = 1
	}
	if _, err := db.Instance.Table(&models.UserEmail{}).
		Where("id = ? and user_id=?", uid, ctx.UserID).
		Update(map[string]interface{}{"is_read": value}); err != nil {
		log.WithContext(ctx).Errorf("update read state: %v", err)
	}
}
