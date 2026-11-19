package rule

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/context"
	"xorm.io/xorm"
)

// enqueueForward stores a rule-forwarded message and its queue rows in one
// transaction, so forwarding is retried by the outbound worker like any other
// message instead of blocking the inbound delivery. No mailbox relation is
// written: the original message is already in the owner's mailbox, so a
// second copy would be a duplicate.
func enqueueForward(ctx *context.Context, email *parsemail.Email, data []byte, forwardAddress string, user *models.User) error {
	if len(data) == 0 {
		return fmt.Errorf("forwarded message has no body")
	}
	recipients, err := queueRequestRecipients(forwardAddress)
	if err != nil {
		return err
	}
	from := user.Account + "@" + config.Instance.Domains[0]
	now := time.Now()
	modelEmail := models.Email{
		Type:         consts.EmailTypeSend,
		Subject:      email.Subject,
		FromName:     user.Name,
		FromAddress:  from,
		To:           marshalUsers(recipients),
		RawMIME:      string(data),
		Size:         len(data),
		SPFCheck:     1,
		DKIMCheck:    1,
		SendUserID:   user.ID,
		SendDate:     now,
		CronSendTime: now,
		CreateTime:   now,
		Status:       consts.EmailStatusDeliveryPending,
		MsgID:        parsemail.GenerateMsgID(config.Instance.Domain),
	}
	return db.Transaction(db.Instance, func(session *xorm.Session) error {
		if _, err := session.Insert(&modelEmail); err != nil {
			return err
		}
		if modelEmail.Id <= 0 {
			return fmt.Errorf("forwarded email insert returned no id")
		}
		addresses := make([]string, 0, len(recipients))
		for _, recipient := range recipients {
			addresses = append(addresses, recipient.EmailAddress)
		}
		entries, err := queue.Entries(modelEmail.Id, modelEmail.MsgID, from, addresses, now)
		if err != nil {
			return err
		}
		for i := range entries {
			if _, err := session.Insert(&entries[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// queueRequestRecipients validates the forwarding target and returns it as a
// single envelope recipient.
func queueRequestRecipients(forwardAddress string) ([]parsemail.User, error) {
	forwardUser := parsemail.BuilderUser(forwardAddress)
	if forwardUser == nil || forwardUser.EmailAddress == "" {
		return nil, fmt.Errorf("invalid forward address: %s", forwardAddress)
	}
	return []parsemail.User{{Name: forwardUser.Name, EmailAddress: forwardUser.EmailAddress}}, nil
}

func marshalUsers(users []parsemail.User) string {
	data, _ := json.Marshal(users)
	return string(data)
}
