package attachments

import (
	"encoding/json"
	"mime"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/auth"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

// InlineContentType returns the safe image type to render inline.
func InlineContentType(raw string) (string, bool) {
	switch ct, _, _ := mime.ParseMediaType(raw); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/x-icon":
		return ct, true
	}
	return "application/octet-stream", false
}

func GetAttachments(ctx *context.Context, emailId int, cid string) *parsemail.Attachment {
	// Fetch the email content
	var email models.Email
	_, err := db.Instance.ID(emailId).Get(&email)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL error:%+v", err)
		return nil
	}

	// Check permissions
	if !auth.HasAuth(ctx, &email) {
		return nil
	}

	var atts []parsemail.Attachment
	_ = json.Unmarshal([]byte(email.Attachments), &atts)
	for i := range atts {
		if atts[i].ContentID == cid {
			return &atts[i]
		}
	}
	return nil
}

func GetAttachmentsByIndex(ctx *context.Context, emailId int, index int) (string, []byte) {
	// Fetch the email content
	var email models.Email
	_, err := db.Instance.ID(emailId).Get(&email)
	if err != nil {
		log.WithContext(ctx).Errorf("SQL error:%+v", err)
		return "", nil
	}

	// Check permissions
	if !auth.HasAuth(ctx, &email) {
		return "", nil
	}

	var atts []parsemail.Attachment
	_ = json.Unmarshal([]byte(email.Attachments), &atts)

	if index >= 0 && index < len(atts) {
		return atts[index].Filename, atts[index].Content
	}
	return "", nil
}
