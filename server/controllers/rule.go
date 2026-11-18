package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/noelbennettz/imail/db"
	"github.com/noelbennettz/imail/dto"
	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/models"
	"github.com/noelbennettz/imail/services/rule"
	"github.com/noelbennettz/imail/utils/address"
	"github.com/noelbennettz/imail/utils/array"
	"github.com/noelbennettz/imail/utils/context"
	"github.com/noelbennettz/imail/utils/errors"
	log "github.com/sirupsen/logrus"
)

func GetRule(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	res := rule.GetAllRules(ctx, ctx.UserID)
	response.NewSuccessResponse(res).FPrint(w)
}

func UpsertRule(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	requestBody, err := io.ReadAll(req.Body)
	if err != nil {
		log.WithContext(ctx).Errorf("ReadError:%v", err)
		return
	}

	var data *dto.Rule
	err = json.Unmarshal(requestBody, &data)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, "params error", err).FPrint(w)
		return
	}

	if data.Action == dto.FORWARD && !address.IsValidEmailAddress(data.Params) {

		response.NewErrorResponse(response.ParamsError, "ParamsError error", i18n.GetText(ctx.Lang, "invalid_email_address")).FPrint(w)
		return
	}

	for _, r := range data.Rules {
		if !array.InArray(r.Field, []string{"From", "Subject", "To", "Cc", "Text", "Html", "Content"}) {
			response.NewErrorResponse(response.ParamsError, "ParamsError error", "params error! Rule Field Error!").FPrint(w)
			return
		}
	}
	if data.Action == dto.MOVE {
		groupId, parseErr := strconv.Atoi(data.Params)
		if parseErr != nil {
			response.NewErrorResponse(response.ParamsError, "ParamsError", "invalid target folder").FPrint(w)
			return
		}
		if _, isDefault := models.GroupCodeToName[groupId]; !isDefault {
			var group models.Group
			found, queryErr := db.Instance.Where("id=? and user_id=?", groupId, ctx.UserID).Get(&group)
			if queryErr != nil || !found {
				response.NewErrorResponse(response.ParamsError, "ParamsError", "target folder not found").FPrint(w)
				return
			}
		}
	}

	err = save(ctx, data.Encode())
	if err != nil {
		response.NewErrorResponse(response.ServerError, "server error", err).FPrint(w)
		return
	}
	response.NewSuccessResponse("succ").FPrint(w)
}

func save(ctx *context.Context, p *models.Rule) error {
	if p.Id > 0 {
		_, err := db.Instance.Exec(db.WithContext(ctx, "update rule set name=? ,value = ? ,action = ?,params = ?,sort = ? where id = ? and user_id = ?"), p.Name, p.Value, p.Action, p.Params, p.Sort, p.Id, ctx.UserID)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	} else {
		_, err := db.Instance.Exec(db.WithContext(ctx, "insert into rule (name,value,user_id,action,params,sort) values (?,?,?,?,?,?)"), p.Name, p.Value, ctx.UserID, p.Action, p.Params, p.Sort)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}
}

type delRuleReq struct {
	Id int `json:"id"`
}

func DelRule(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	requestBody, err := io.ReadAll(req.Body)
	if err != nil {
		log.WithContext(ctx).Errorf("ReadError:%v", err)
		return
	}

	var data delRuleReq
	err = json.Unmarshal(requestBody, &data)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, "params error", err).FPrint(w)
		return
	}

	if data.Id <= 0 {
		response.NewErrorResponse(response.ParamsError, "params error", "id is empty").FPrint(w)
		return
	}

	_, err = db.Instance.Exec(db.WithContext(ctx, "delete from rule where id =? and user_id =?"), data.Id, ctx.UserID)
	if err != nil {
		response.NewErrorResponse(response.ServerError, "unknown error", err).FPrint(w)
		return
	}

	response.NewSuccessResponse("succ").FPrint(w)
}
