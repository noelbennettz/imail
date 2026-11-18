package del_email

import (
	"github.com/noelbennettz/imail/consts"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"log/slog"
	"xorm.io/xorm"
)
import . "xorm.io/builder"

func DelEmail(ctx *context.Context, ids []int, forcedDel bool) error {
	session := db.Instance.NewSession()
	defer session.Close()
	if err := session.Begin(); err != nil {
		return err
	}
	for _, id := range ids {
		err := deleteOne(ctx, session, cast.ToInt64(id), forcedDel)
		if err != nil {
			session.Rollback()
			return err
		}
	}
	return session.Commit()
}

type num struct {
	Num int `xorm:"num"`
}

func deleteOne(ctx *context.Context, session *xorm.Session, id int64, forcedDel bool) error {
	if !forcedDel {
		_, err := session.Table(&models.UserEmail{}).Where("email_id=? and user_id=?", id, ctx.UserID).Update(map[string]interface{}{
			"status":   consts.EmailStatusDel,
			"group_id": 0,
		})
		return err
	}
	// Delete the association first
	var ue models.UserEmail
	_, err := session.Table(&models.UserEmail{}).Where("email_id=? and user_id=?", id, ctx.UserID).Delete(&ue)
	if err != nil {
		return err
	}
	// Check whether anyone still has permission on the email
	var Num num
	_, err = session.Table(&models.UserEmail{}).Select("count(1) as num").Where("email_id=? ", id).Get(&Num)
	if err != nil {
		return err
	}
	if Num.Num == 0 {
		var email models.Email
		_, err = session.Table(&email).Where("id=?", id).Delete(&email)

	}
	return err
}

func DelByUID(ctx *context.Context, ids []int) error {
	session := db.Instance.NewSession()
	defer session.Close()
	for _, id := range ids {
		var ue models.UserEmail
		session.Table("user_email").Where(Eq{"id": id, "user_id": ctx.UserID}).Get(&ue)
		if ue.ID == 0 {
			log.WithContext(ctx).Warn("no user email found")
			return nil
		}
		emailId := ue.EmailID

		// Delete the association first
		_, err := session.Table(&models.UserEmail{}).Where("id=? and user_id=?", id, ctx.UserID).Delete(&ue)
		if err != nil {
			slog.Error("SQLError", slog.Any("err", err))
			session.Rollback()
			return err
		}

		// Check whether anyone still has permission on the email
		var Num num
		_, err = session.Table(&models.UserEmail{}).Select("count(1) as num").Where("email_id=? ", emailId).Get(&Num)
		if err != nil {
			slog.Error("SQLError", slog.Any("err", err))
			session.Rollback()
			return err
		}
		if Num.Num == 0 {
			var email models.Email
			_, err = session.Table(&email).Where("id=?", emailId).Delete(&email)
			if err != nil {
				slog.Error("SQLError", slog.Any("err", err))
			}
		}
	}
	session.Commit()
	return nil
}
