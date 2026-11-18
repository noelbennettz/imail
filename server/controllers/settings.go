package controllers

import (
	"encoding/json"
	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/session"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/password"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
)

type modifyPasswordRequest struct {
	Password string `json:"password"`
}

func ModifyPassword(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Errorf("%+v", err)
	}
	var retData modifyPasswordRequest
	err = json.Unmarshal(reqBytes, &retData)
	if err != nil {
		log.Errorf("%+v", err)
	}

	if retData.Password != "" {
		if len(retData.Password) < 8 {
			response.NewErrorResponse(response.ParamsError, "Password must be at least 8 characters", "").FPrint(w)
			return
		}
		encodePwd := password.Encode(retData.Password)

		_, err := db.Instance.Table("user").Where("id=?", ctx.UserID).Incr("auth_version").Update(map[string]interface{}{"password": encodePwd})
		if err != nil {
			response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "unknowError"), "").FPrint(w)
			return
		}
		// All sessions, including the one changing the password, must sign in again.
		if err := session.Instance.Destroy(req.Context()); err != nil {
			log.WithContext(ctx).Errorf("Destroy session after password change: %v", err)
			response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "unknowError"), "").FPrint(w)
			return
		}

	}

	response.NewSuccessResponse(i18n.GetText(ctx.Lang, "succ")).FPrint(w)
}
