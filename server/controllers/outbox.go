package controllers

import (
	"net/http"

	"github.com/noelbennettz/imail/dto/response"
	"github.com/noelbennettz/imail/services/queue"
	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
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
