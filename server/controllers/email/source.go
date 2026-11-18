package email

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/detail"
	"github.com/noelbennettz/imail/utils/context"
)

// EmailSource returns a reconstructed RFC 822 source view for one message.
func EmailSource(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	var payload emailDetailRequest
	if err := json.Unmarshal(data, &payload); err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	if payload.ID <= 0 {
		response.NewErrorResponse(response.ParamsError, "invalid ID", "").FPrint(w)
		return
	}
	source, err := detail.GetEmailSource(ctx, payload.ID)
	if err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	}
	response.NewSuccessResponse(map[string]any{
		"id":     payload.ID,
		"source": source,
	}).FPrint(w)
}
