package queue

import (
	"database/sql"
	"fmt"
	"net/textproto"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/quota"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/send"
	"xorm.io/xorm"
)

func localDomains() []string {
	if config.Instance == nil {
		return nil
	}
	domains := make([]string, 0, len(config.Instance.Domains)+1)
	domains = append(domains, config.Instance.Domains...)
	if config.Instance.Domain != "" {
		domains = append(domains, config.Instance.Domain)
	}
	return domains
}

// splitQueueRecipients separates recipients this server hosts from addresses
// that need an MX lookup. Local mailboxes are written directly by the worker,
// so local delivery neither depends on the domain's MX record nor on port 25
// being reachable from the server to itself.
func splitQueueRecipients(recipients []string) (local, remote []string) {
	users := make([]*parsemail.User, 0, len(recipients))
	for _, address := range recipients {
		users = append(users, &parsemail.User{EmailAddress: address})
	}
	localUsers, remoteAddresses := send.SplitRecipients(users, localDomains())
	for _, user := range localUsers {
		local = append(local, user.EmailAddress)
	}
	return local, remoteAddresses
}

func localRecipients(address string) ([]*models.User, error) {
	account, domain, found := strings.Cut(strings.TrimSpace(address), "@")
	if !found || account == "" || domain == "" {
		return nil, &textproto.Error{Code: 550, Msg: "invalid local recipient"}
	}
	isLocal := false
	for _, configured := range localDomains() {
		if strings.EqualFold(configured, domain) {
			isLocal = true
			break
		}
	}
	if !isLocal {
		return nil, &textproto.Error{Code: 550, Msg: "recipient domain is not local"}
	}

	var user models.User
	found, err := db.Instance.Table(&models.User{}).
		Where("LOWER(account)=LOWER(?)", account).
		Get(&user)
	if err != nil {
		return nil, err
	}
	if found && (user.Domain == "" || strings.EqualFold(user.Domain, domain)) {
		return []*models.User{&user}, nil
	}

	// Mirror inbound delivery: an address that has no mailbox is handed to the
	// administrators instead of being silently dropped.
	var admins []*models.User
	if err := db.Instance.Table(&models.User{}).Where("is_admin=1").Find(&admins); err != nil {
		return nil, err
	}
	if len(admins) == 0 {
		return nil, &textproto.Error{Code: 550, Msg: "recipient not found"}
	}
	return admins, nil
}

// deliverLocal stores an inbound copy for each local recipient, exactly like an
// SMTP delivery would: a received email row with its own id and an unread Inbox
// relation. This keeps the sender's Sent copy and the recipient's Inbox copy
// independent, including when both are the same user.
func deliverLocal(ctx *context.Context, original *models.Email, recipients []string) error {
	for _, address := range recipients {
		users, err := localRecipients(address)
		if err != nil {
			return err
		}

		var pending []*models.User
		for _, user := range users {
			exists, err := localInboxCopyExists(user.ID, original.MsgID)
			if err != nil {
				return err
			}
			if !exists {
				pending = append(pending, user)
			}
		}
		if len(pending) == 0 {
			continue
		}
		if err := quota.Check(pending, len(original.RawMIME)); err != nil {
			return &textproto.Error{Code: 552, Msg: "mailbox quota exceeded"}
		}

		copyEmail := *original
		copyEmail.Id = 0
		copyEmail.Type = consts.EmailTypeReceive
		copyEmail.Status = consts.EmailStatusWait
		copyEmail.SendUserID = 0
		copyEmail.Error = sql.NullString{}
		if err := db.Transaction(db.Instance, func(session *xorm.Session) error {
			if _, err := session.Insert(&copyEmail); err != nil {
				return err
			}
			for _, user := range pending {
				if _, err := session.Insert(&models.UserEmail{
					UserID:  user.ID,
					EmailID: copyEmail.Id,
					Status:  consts.EmailStatusWait,
					GroupId: 0,
					IsRead:  0,
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("store local copy for %s: %w", address, err)
		}
	}
	return nil
}

func localInboxCopyExists(userID int, msgID string) (bool, error) {
	if msgID == "" {
		return false, nil
	}
	var copies []models.Email
	if err := db.Instance.Table("email").Where("type=? and msg_id=?", consts.EmailTypeReceive, msgID).Find(&copies); err != nil {
		return false, err
	}
	for _, copyEmail := range copies {
		exists, err := db.Instance.Table(&models.UserEmail{}).
			Where("user_id=? and email_id=? and status=?", userID, copyEmail.Id, consts.EmailStatusWait).
			Exist()
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}
