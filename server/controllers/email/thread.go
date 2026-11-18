package email

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/detail"
	"github.com/noelbennettz/imail/utils/context"
)

// EmailThread returns all messages in the conversation that contains the
// requested email, oldest first.
func EmailThread(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	var payload struct {
		ID     int `json:"id"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return
	}
	if payload.ID <= 0 {
		response.NewErrorResponse(response.ParamsError, "invalid ID", "").FPrint(w)
		return
	}
	if payload.Limit <= 0 || payload.Limit > 50 {
		payload.Limit = 5
	}
	messages, total, err := detail.GetEmailThread(ctx, payload.ID, true, payload.Offset, payload.Limit)
	if err != nil {
		response.NewErrorResponse(response.ServerError, err.Error(), "").FPrint(w)
		return
	}
	response.NewSuccessResponse(map[string]any{
		"list":     messages,
		"total":    total,
		"has_more": payload.Offset+len(messages) < total,
	}).FPrint(w)
}
