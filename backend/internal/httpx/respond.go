// Package httpx holds HTTP plumbing shared by all handlers: JSON decoding,
// error responses and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/PRCSM/live-poll/backend/internal/apperr"
)

// Fail writes err as a JSON error. Unknown errors become a generic 500 and are logged,
// so driver messages and stack details never reach the client.
func Fail(c *gin.Context, err error) {
	if e, ok := apperr.As(err); ok {
		c.AbortWithStatusJSON(e.Status, gin.H{"error": e})
		return
	}
	slog.ErrorContext(c.Request.Context(), "request failed",
		"err", err, "path", c.FullPath(), "request_id", c.GetString(RequestIDKey))
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": apperr.New(http.StatusInternalServerError, "internal", "Something went wrong on our side.")})
}

// DecodeJSON strictly decodes the body: unknown fields, trailing data and oversized
// bodies are rejected rather than silently ignored.
func DecodeJSON(c *gin.Context, dst any) error {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return apperr.New(http.StatusRequestEntityTooLarge, "body_too_large", "Request body is too large.")
		}
		return apperr.ErrBadJSON
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return apperr.ErrBadJSON
	}
	return nil
}
