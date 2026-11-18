package controllers

import (
	"net/http"

	"github.com/noelbennettz/imail/dto/response"
)

func Ping(w http.ResponseWriter, req *http.Request) {
	response.NewSuccessResponse("pong").FPrint(w)
}
