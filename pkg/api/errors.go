package api

import (
	"encoding/json"
	"net/http"
)

// ErrorCode identifies the category of an API error.
type ErrorCode string

const (
	ErrCodeNotFound       ErrorCode = "NOT_FOUND"
	ErrCodeConflict       ErrorCode = "CONFLICT"
	ErrCodeBadRequest     ErrorCode = "BAD_REQUEST"
	ErrCodeInternalError  ErrorCode = "INTERNAL_ERROR"
	ErrCodeRevisionNeeded ErrorCode = "REVISION_REQUIRED"
)

// APIError is the structured error response returned by the API.
type APIError struct {
	Error string    `json:"error"`
	Code  ErrorCode `json:"code"`
}

func writeError(w http.ResponseWriter, status int, code ErrorCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIError{Error: msg, Code: code})
}

func notFound(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusNotFound, ErrCodeNotFound, msg)
}

func conflict(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusConflict, ErrCodeConflict, msg)
}

func badRequest(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusBadRequest, ErrCodeBadRequest, msg)
}

func internalError(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusInternalServerError, ErrCodeInternalError, msg)
}
