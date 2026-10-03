package v1

import "encoding/json"

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewError(code, msg string) []byte {
	b, _ := json.Marshal(ErrorResponse{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: msg}})
	return b
}
