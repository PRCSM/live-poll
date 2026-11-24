// Package apperr defines the errors handlers return to clients.
// Anything that is not an *Error is treated as an internal failure and never shown.
package apperr

import (
	"errors"
	"net/http"
)

type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func New(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Some fields are invalid.", Fields: fields}
}

var (
	ErrNotFound     = New(http.StatusNotFound, "not_found", "Not found.")
	ErrUnauthorized = New(http.StatusUnauthorized, "unauthorized", "Sign in to continue.")
	ErrRateLimited  = New(http.StatusTooManyRequests, "rate_limited", "Too many requests. Slow down and try again shortly.")
	ErrBadJSON      = New(http.StatusBadRequest, "bad_request", "Request body is not valid JSON for this endpoint.")
	ErrUnavailable  = New(http.StatusServiceUnavailable, "unavailable", "Live service is temporarily unavailable. Try again in a moment.")
)

// As extracts an *Error from err, if there is one.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
