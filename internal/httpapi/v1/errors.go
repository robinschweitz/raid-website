package v1

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewError(code, msg string) []byte {
	b, _ := json.Marshal(ErrorResponse{Error: ErrorBody{Code: code, Message: msg}})
	return b
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func WriteError(w http.ResponseWriter, status int, code, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: msg}})
}

// nonNil returns an empty slice when items is nil, so that lists are encoded as
// JSON arrays instead of null and clients can always iterate the response.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// writeListJSON writes a list response with the items normalised to a non-nil
// slice. Every list endpoint must use it instead of WriteJSON.
func writeListJSON[T any](w http.ResponseWriter, items []T) {
	WriteJSON(w, http.StatusOK, map[string]any{"data": nonNil(items)})
}

const maxBodyBytes = 1 << 20

func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

var ErrInvalidBody = errors.New("invalid request body")

func BadRequest(w http.ResponseWriter, msg string) {
	WriteError(w, http.StatusBadRequest, "invalid_request", msg)
}
