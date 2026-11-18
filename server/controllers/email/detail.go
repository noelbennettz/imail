package email

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/detail"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

type emailDetailRequest struct {
	ID int `json:"id"`
}

func EmailDetail(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.WithContext(ctx).Errorf("%+v", err)
	}
	var retData emailDetailRequest
	err = json.Unmarshal(reqBytes, &retData)
	if err != nil {
		log.WithContext(ctx).Errorf("%+v", err)
	}

	if retData.ID <= 0 {
		response.NewErrorResponse(response.ParamsError, "invalid ID", "").FPrint(w)
		return
	}

	email, err := detail.GetEmailDetail(ctx, retData.ID, true)
	if err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	}

	response.NewSuccessResponse(email).FPrint(w)
}
