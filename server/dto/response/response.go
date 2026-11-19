package response

import (
	"encoding/json"
	"net/http"
)

const (
	NeedSetup          = 402
	NeedLogin          = 403
	NoAccessPrivileges = 405
	// InvalidSetupToken is the error code of the setup wizard when the one-time
	// setup token is missing, wrong or no longer accepted. The HTTP status is
	// 403; the distinct code keeps the web client from treating it as an
	// expired login session.
	InvalidSetupToken = 406
	ParamsError       = 100
	ServerError       = 500
)

type Response struct {
	ErrorNo  int    `json:"errorNo"`
	ErrorMsg string `json:"errorMsg"`
	Data     any    `json:"data"`
}

func (p *Response) FPrint(w http.ResponseWriter) {
	bytesData, _ := json.Marshal(p)
	w.Write(bytesData)
}

func NewSuccessResponse(data any) *Response {
	return &Response{
		Data: data,
	}
}

func NewErrorResponse(errorNo int, errorMsg string, data any) *Response {
	return &Response{
		ErrorNo:  errorNo,
		ErrorMsg: errorMsg,
		Data:     data,
	}
}
