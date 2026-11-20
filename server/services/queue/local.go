package queue

import (
	"fmt"
	"net/textproto"
	"strings"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/send"
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

// deliverLocal inserts the mailbox relation for every local recipient. The
// message row already exists, so a retry is idempotent: an existing relation is
// left alone.
func deliverLocal(ctx *context.Context, emailID int, recipients []string) error {
	for _, address := range recipients {
		users, err := localRecipients(address)
		if err != nil {
			return err
		}
		for _, user := range users {
			exists, err := db.Instance.Table(&models.UserEmail{}).
				Where("email_id=? and user_id=? and status=?", emailID, user.ID, consts.EmailStatusWait).
				Exist()
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			if _, err := db.Instance.Insert(&models.UserEmail{
				UserID:  user.ID,
				EmailID: emailID,
				Status:  consts.EmailStatusWait,
				GroupId: 0,
				IsRead:  0,
			}); err != nil {
				return fmt.Errorf("store local copy for %s: %w", address, err)
			}
		}
	}
	return nil
}
