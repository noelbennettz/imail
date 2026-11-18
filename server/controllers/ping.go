package controllers

import (
	"github.com/noelbennettz/imail/dto/response"
	"net/http"
)

func Ping(w http.ResponseWriter, req *http.Request) {
	response.NewSuccessResponse("pong").FPrint(w)
}
