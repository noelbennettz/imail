package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/i18n"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

// OutboxStatus reports how much outbound mail is waiting and how much has
// finally failed. It is read-only: the queue is drained by the worker.
func OutboxStatus(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	pending, failed, err := queue.Counts()
	if err != nil {
		log.WithContext(ctx).Errorf("Read outbox counts: %v", err)
		response.NewErrorResponse(response.ServerError, "Unable to read the outbox", "").FPrint(w)
		return
	}
	response.NewSuccessResponse(map[string]any{
		"pending": pending,
		"failed":  failed,
	}).FPrint(w)
}

// OutboxList returns one page of the outbox for the administration page. The
// filter is `pending` or `failed`; the response carries the rows and the total
// so the UI can page.
func OutboxList(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return
	}
	query := req.URL.Query()
	entries, total, err := queue.List(query.Get("state"), cast.ToInt(query.Get("page")), cast.ToInt(query.Get("page_size")))
	if err != nil {
		log.WithContext(ctx).Errorf("Read outbox list: %v", err)
		response.NewErrorResponse(response.ParamsError, "Unable to read the outbox", "").FPrint(w)
		return
	}
	response.NewSuccessResponse(map[string]any{
		"entries": entries,
		"total":   total,
	}).FPrint(w)
}

type outboxEntryRequest struct {
	Id int64 `json:"id"`
}

// OutboxRetry puts a finally failed row back in the queue at once.
func OutboxRetry(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	id, ok := decodeOutboxEntry(ctx, w, req)
	if !ok {
		return
	}
	if err := queue.Retry(id, time.Now()); err != nil {
		outboxError(ctx, w, err)
		return
	}
	log.WithContext(ctx).Infof("Outbox entry %d queued for an immediate retry", id)
	response.NewSuccessResponse(i18n.GetText(ctx.Lang, "succ")).FPrint(w)
}

// OutboxDelete removes one finished row.
func OutboxDelete(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	id, ok := decodeOutboxEntry(ctx, w, req)
	if !ok {
		return
	}
	if err := queue.Delete(id); err != nil {
		outboxError(ctx, w, err)
		return
	}
	log.WithContext(ctx).Infof("Outbox entry %d deleted", id)
	response.NewSuccessResponse(i18n.GetText(ctx.Lang, "succ")).FPrint(w)
}

// decodeOutboxEntry enforces the administrator check and reads {"id": n}.
func decodeOutboxEntry(ctx *context.Context, w http.ResponseWriter, req *http.Request) (int64, bool) {
	if !ctx.IsAdmin {
		response.NewErrorResponse(response.NoAccessPrivileges, "No access privileges", "").FPrint(w)
		return 0, false
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
		return 0, false
	}
	var payload outboxEntryRequest
	if err := json.Unmarshal(data, &payload); err != nil || payload.Id <= 0 {
		response.NewErrorResponse(response.ParamsError, "id is required", "").FPrint(w)
		return 0, false
	}
	return payload.Id, true
}

func outboxError(ctx *context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, queue.ErrEntryNotFound):
		response.NewErrorResponse(response.ParamsError, "Outbox entry not found", "").FPrint(w)
	case errors.Is(err, queue.ErrEntryState):
		response.NewErrorResponse(response.ParamsError, err.Error(), "").FPrint(w)
	default:
		log.WithContext(ctx).Errorf("Outbox change failed: %v", err)
		response.NewErrorResponse(response.ServerError, "Unable to change the outbox entry", "").FPrint(w)
	}
}
