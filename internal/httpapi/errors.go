// Package httpapi contains the HTTP transport layer: handlers, routing helpers
// and the centralised error-to-response mapping.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

// This file replaces the Java RestResponseEntityExceptionHandling
// @ControllerAdvice, which extended Spring's ResponseEntityExceptionHandler.
//
// MIGRATION_NOTE: Go has no global exception interception. Instead, every
// handler must route its failures through WriteError (and the router should
// wrap itself in Recoverer and use NotFoundHandler) to get the same
// cross-cutting behaviour that @ControllerAdvice provided implicitly.

// ErrBadRequest marks client errors that Spring's ResponseEntityExceptionHandler
// answered with 400 and an empty body (malformed JSON, failed bean validation,
// type-mismatched path variables, missing parameters, ...). Wrap it with
// BadRequest or fmt.Errorf("...: %w", ErrBadRequest).
var ErrBadRequest = errors.New("bad request")

// ErrUnsupportedMediaType mirrors HttpMediaTypeNotSupportedException (415,
// empty body).
var ErrUnsupportedMediaType = errors.New("unsupported media type")

// ErrNotAcceptable mirrors HttpMediaTypeNotAcceptableException (406, empty
// body).
var ErrNotAcceptable = errors.New("not acceptable")

// BadRequest wraps cause so that WriteError answers with an empty-body 400.
func BadRequest(cause error) error {
	if cause == nil {
		return ErrBadRequest
	}
	return fmt.Errorf("%w: %w", ErrBadRequest, cause)
}

// MethodNotAllowedError mirrors HttpRequestMethodNotSupportedException:
// Spring answered with 405, an Allow header and an empty body.
type MethodNotAllowedError struct {
	// Method is the HTTP method the client used.
	Method string
	// Allowed lists the methods the resource supports.
	Allowed []string
}

// Error implements the error interface.
func (e *MethodNotAllowedError) Error() string {
	return fmt.Sprintf("request method '%s' not supported", e.Method)
}

// WriteError converts err into the HTTP response the Spring application would
// have produced:
//
//   - service.ErrUserNotFound (or any *service.UserNotFoundError) → 404 with
//     an ErrorMessage JSON body {"status":"NOT_FOUND","message":<msg>}.
//   - *MethodNotAllowedError → 405, Allow header, empty body.
//   - ErrBadRequest → 400, empty body.
//   - ErrUnsupportedMediaType → 415, empty body.
//   - ErrNotAcceptable → 406, empty body.
//   - anything else → 500 with Spring Boot's default error JSON
//     {timestamp, status, error, path}, logged at error level.
//
// A nil err is a no-op.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	var mna *MethodNotAllowedError
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		msg := userNotFoundMessage(err)
		writeJSON(w, http.StatusNotFound, model.NewErrorMessage(model.HTTPStatusNotFound, msg))
	case errors.As(err, &mna):
		if len(mna.Allowed) > 0 {
			w.Header().Set("Allow", strings.Join(mna.Allowed, ", "))
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	case errors.Is(err, ErrBadRequest):
		w.WriteHeader(http.StatusBadRequest)
	case errors.Is(err, ErrUnsupportedMediaType):
		w.WriteHeader(http.StatusUnsupportedMediaType)
	case errors.Is(err, ErrNotAcceptable):
		w.WriteHeader(http.StatusNotAcceptable)
	default:
		log.Error().Err(err).Str("method", r.Method).Str("path", r.URL.Path).Msg("unhandled error")
		WriteDefaultError(w, r, http.StatusInternalServerError)
	}
}

// userNotFoundMessage returns what Java's exception.getMessage() would have
// produced: the UserNotFoundError's own message when present, otherwise the
// sentinel's text (outer fmt.Errorf wrapping context is not part of the
// Java exception message, so it is dropped).
func userNotFoundMessage(err error) string {
	var unf *service.UserNotFoundError
	if errors.As(err, &unf) {
		return unf.Error()
	}
	return service.ErrUserNotFound.Error()
}

// defaultErrorBody is Spring Boot's BasicErrorController JSON shape.
type defaultErrorBody struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Error     string `json:"error"`
	Path      string `json:"path"`
}

// springTimestampLayout matches Jackson's default rendering of the error
// timestamp, e.g. "2024-01-02T15:04:05.000+00:00".
const springTimestampLayout = "2006-01-02T15:04:05.000-07:00"

// WriteDefaultError writes Spring Boot's default error JSON for the given
// status code: {"timestamp":..., "status":code, "error":reason, "path":...}.
func WriteDefaultError(w http.ResponseWriter, r *http.Request, code int) {
	body := defaultErrorBody{
		Timestamp: time.Now().Format(springTimestampLayout),
		Status:    code,
		Error:     http.StatusText(code),
		Path:      r.URL.Path,
	}
	writeJSON(w, code, body)
}

// NotFoundHandler returns the handler for unmatched routes, producing Spring
// Boot's default 404 JSON ({..., "status":404, "error":"Not Found", ...}).
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteDefaultError(w, r, http.StatusNotFound)
	})
}

// Recoverer is middleware that converts a panicking handler into Spring's
// default 500 JSON response, mirroring how uncaught exceptions in a
// controller surfaced in the source application.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			log.Error().Interface("panic", rec).Str("method", r.Method).Str("path", r.URL.Path).Msg("handler panicked")
			WriteDefaultError(w, r, http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}

// writeJSON serialises v as the response body with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Msg("failed to encode error response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(payload); err != nil {
		log.Debug().Err(err).Msg("failed to write error response")
	}
}
