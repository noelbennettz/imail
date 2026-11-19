package email

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/noelbennettz/imail/config"
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/parsemail"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/async"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/send"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"xorm.io/xorm"
)

type sendRequest struct {
	ReplyTo     []user       `json:"reply_to"`
	From        user         `json:"from"`
	To          []user       `json:"to"`
	Bcc         []user       `json:"bcc"`
	Cc          []user       `json:"cc"`
	Subject     string       `json:"subject"`
	Text        string       `json:"text"`   // Plaintext message (optional)
	HTML        string       `json:"html"`   // Html message (optional)
	Sender      user         `json:"sender"` // override From as SMTP envelope sender (optional)
	ReadReceipt []string     `json:"read_receipt"`
	Attachments []attachment `json:"attrs"`
}

type user struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type attachment struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func Send(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.WithContext(ctx).Errorf("%+v", err)
		response.NewErrorResponse(response.ParamsError, "params error", err.Error()).FPrint(w)
		return
	}
	log.WithContext(ctx).Infof("Sending email")

	var reqData sendRequest
	err = json.Unmarshal(reqBytes, &reqData)
	if err != nil {
		log.WithContext(ctx).Errorf("%+v", err)
		response.NewErrorResponse(response.ParamsError, "params error", err.Error()).FPrint(w)
		return
	}

	if reqData.From.Email != "" {
		infos := strings.Split(reqData.From.Email, "@")
		if len(infos) != 2 || !array.InArray(infos[1], config.Instance.Domains) {
			response.NewErrorResponse(response.ParamsError, "params error", "").FPrint(w)
			return
		}
		if !ctx.IsAdmin && infos[0] != ctx.UserAccount {
			response.NewErrorResponse(response.ParamsError, "params error", "").FPrint(w)
			return
		}
		if !ctx.IsAdmin && ctx.UserDomain != "" && !strings.EqualFold(infos[1], ctx.UserDomain) {
			response.NewErrorResponse(response.ParamsError, "sender domain is not assigned to this mailbox", "").FPrint(w)
			return
		}

	}

	if reqData.From.Email == "" {
		domain := config.Instance.Domain
		if ctx.UserDomain != "" {
			domain = ctx.UserDomain
		}
		reqData.From.Email = ctx.UserAccount + "@" + domain
	}

	if reqData.From.Email == "" {
		response.NewErrorResponse(response.ParamsError, "sender is required", "sender is required").FPrint(w)
		return
	}

	if reqData.From.Name == "" {
		reqData.From.Name = ctx.UserName
	}

	if reqData.Subject == "" {
		response.NewErrorResponse(response.ParamsError, "subject is required", "subject is required").FPrint(w)
		return
	}

	if len(reqData.To) <= 0 {
		response.NewErrorResponse(response.ParamsError, "recipient is required", "recipient is required").FPrint(w)
		return
	}

	e := &parsemail.Email{}

	for _, to := range reqData.To {
		e.To = append(e.To, &parsemail.User{
			Name:         to.Name,
			EmailAddress: to.Email,
		})
	}

	for _, bcc := range reqData.Bcc {
		e.Bcc = append(e.Bcc, &parsemail.User{
			Name:         bcc.Name,
			EmailAddress: bcc.Email,
		})
	}

	for _, cc := range reqData.Cc {
		e.Cc = append(e.Cc, &parsemail.User{
			Name:         cc.Name,
			EmailAddress: cc.Email,
		})
	}

	e.From = &parsemail.User{
		Name:         reqData.From.Name,
		EmailAddress: reqData.From.Email,
	}
	if reqData.Sender.Email != "" {
		e.Sender = &parsemail.User{
			Name:         reqData.Sender.Name,
			EmailAddress: reqData.Sender.Email,
		}
	} else {
		e.Sender = &parsemail.User{
			Name:         reqData.From.Name,
			EmailAddress: reqData.From.Email,
		}
	}

	e.Text = []byte(reqData.Text)
	e.HTML = []byte(reqData.HTML)
	e.Subject = reqData.Subject
	for _, att := range reqData.Attachments {
		att.Data = strings.TrimPrefix(att.Data, "data:")
		infos := strings.SplitN(att.Data, ";", 2)
		if len(infos) != 2 {
			response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "att_err"), "malformed attachment data").FPrint(w)
			return
		}
		contentType := infos[0]
		content := strings.TrimPrefix(infos[1], "base64,")
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			log.WithContext(ctx).Errorf("attachment decoding error! %v", err)
			response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "att_err"), err.Error()).FPrint(w)
			return
		}
		e.Attachments = append(e.Attachments, &parsemail.Attachment{
			Filename:    att.Name,
			ContentType: contentType,
			Content:     decoded,
		})

	}

	// The message is rendered once: the stored copy and the queue rows must
	// carry exactly the bytes that are delivered.
	rawMIME := e.BuildBytes(ctx, true)
	localRecipients, remoteRecipients := send.SplitRecipients(
		append(append(append([]*parsemail.User{}, e.To...), e.Cc...), e.Bcc...),
		config.Instance.Domains,
	)
	queued := len(remoteRecipients) > 0

	initialStatus := int8(consts.EmailStatusSent)
	initialError := sql.NullString{}
	if queued {
		initialStatus = consts.EmailStatusDeliveryPending
		initialError = sql.NullString{String: webmailPendingError, Valid: true}
	}

	modelEmail := models.Email{
		Type:         consts.EmailTypeSend,
		Subject:      e.Subject,
		ReplyTo:      json2string(e.ReplyTo),
		FromName:     e.From.Name,
		FromAddress:  e.From.EmailAddress,
		To:           json2string(e.To),
		Bcc:          json2string(e.Bcc),
		Cc:           json2string(e.Cc),
		Text:         sql.NullString{String: string(e.Text), Valid: true},
		Html:         sql.NullString{String: string(e.HTML), Valid: true},
		Sender:       json2string(e.Sender),
		Attachments:  json2string(e.Attachments),
		RawMIME:      string(rawMIME),
		SPFCheck:     1,
		DKIMCheck:    1,
		SendUserID:   ctx.UserID,
		SendDate:     time.Now(),
		CronSendTime: time.Now(),
		Status:       initialStatus,
		CreateTime:   time.Now(),
		MsgID:        parsemail.GenerateMsgID(config.Instance.Domain),
		Error:        initialError,
	}

	// The sent copy, its mailbox relation and the queue rows commit together,
	// so the API only reports success once the remote copies are durable.
	err = db.Transaction(db.Instance, func(session *xorm.Session) error {
		if _, err := session.Insert(&modelEmail); err != nil {
			return err
		}
		if modelEmail.Id <= 0 {
			return errors.New("email insert returned no id")
		}
		relation := models.UserEmail{
			UserID:  ctx.UserID,
			EmailID: modelEmail.Id,
			Status:  modelEmail.Status,
			IsRead:  1,
		}
		if _, err := session.Insert(&relation); err != nil {
			return err
		}
		if !queued {
			return nil
		}
		entries, err := queue.Entries(modelEmail.Id, modelEmail.MsgID, e.From.EmailAddress, remoteRecipients, time.Now())
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
	if err != nil {
		log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=webmail msg_id=%q error=%v", modelEmail.MsgID, err)
		response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "send_fail"), err.Error()).FPrint(w)
		return
	}

	e.MessageId = cast.ToInt64(modelEmail.Id)
	e.MsgID = modelEmail.MsgID

	log.WithContext(ctx).Infof("Webmail message %d queued for %d remote recipient(s)", modelEmail.Id, len(remoteRecipients))

	if len(localRecipients) > 0 {
		// Local mailboxes are still handed over inline, after the commit. When a
		// remote copy is queued the final status belongs to the worker.
		async.New(ctx).Process(func(p any) {
			deliveryErr, _ := send.SendTo(ctx, e, localRecipients)
			if queued {
				if deliveryErr != nil {
					log.WithContext(ctx).Warnf("Inline delivery to local recipients failed after the message was queued: %v", deliveryErr)
					if _, err := db.Instance.Exec(db.WithContext(ctx, "update email set error=? where id=?"), webmailPendingError+": "+deliveryErr.Error(), modelEmail.Id); err != nil {
						log.WithContext(ctx).Errorf("sql Error :%+v", err)
					}
				}
				return
			}

			targetStatus := int8(consts.EmailStatusSent)
			var errorText any
			if deliveryErr != nil {
				targetStatus = consts.EmailStatusFail
				errorText = deliveryErr.Error()
			}
			if _, err := db.Instance.Exec(db.WithContext(ctx, "update email set status=?, error=? where id=?"), targetStatus, errorText, modelEmail.Id); err != nil {
				log.WithContext(ctx).Errorf("sql Error :%+v", err)
			}
			if _, err := db.Instance.Exec(db.WithContext(ctx, "update user_email set status=? where email_id=?"), targetStatus, modelEmail.Id); err != nil {
				log.WithContext(ctx).Errorf("sql Error :%+v", err)
			}
		}, nil)
	}

	response.NewSuccessResponse(i18n.GetText(ctx.Lang, "succ")).FPrint(w)
}

// webmailPendingError marks a composed message that is waiting in the outbound
// queue, so the sent folder shows why it is not marked sent yet.
const webmailPendingError = "waiting for delivery to other servers"

func json2string(d any) string {
	by, _ := json.Marshal(d)
	return string(by)
}
