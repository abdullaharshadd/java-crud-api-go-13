package model

import (
	"net/http"
	"strings"
)

// HTTPStatus is the JSON-facing representation of an HTTP status, serialised
// by its Spring HttpStatus enum constant name (e.g. "NOT_FOUND"), which is how
// Jackson rendered org.springframework.http.HttpStatus in the source.
type HTTPStatus string

// HTTP status names used by the application's error responses.
const (
	HTTPStatusBadRequest          HTTPStatus = "BAD_REQUEST"
	HTTPStatusNotFound            HTTPStatus = "NOT_FOUND"
	HTTPStatusInternalServerError HTTPStatus = "INTERNAL_SERVER_ERROR"
)

// HTTPStatusFromCode converts a numeric HTTP status code into its Spring
// HttpStatus enum name (e.g. 404 -> "NOT_FOUND"). It reports false when the
// code is not a standard status known to net/http.
func HTTPStatusFromCode(code int) (HTTPStatus, bool) {
	text := http.StatusText(code)
	if text == "" {
		return "", false
	}
	name := strings.ToUpper(text)
	name = strings.NewReplacer(" ", "_", "-", "_", "'", "").Replace(name)
	return HTTPStatus(name), true
}

// ErrorMessage is the JSON body returned for handled errors, e.g.
// {"status":"NOT_FOUND","message":"User are not available"}.
type ErrorMessage struct {
	Status  HTTPStatus `json:"status"`
	Message string     `json:"message"`
}

// NewEmptyErrorMessage returns an ErrorMessage with all fields unset
// (the Lombok @NoArgsConstructor equivalent).
func NewEmptyErrorMessage() *ErrorMessage {
	return &ErrorMessage{}
}

// NewErrorMessage returns an ErrorMessage with the given status and message
// (the Lombok @AllArgsConstructor equivalent).
func NewErrorMessage(status HTTPStatus, message string) *ErrorMessage {
	return &ErrorMessage{Status: status, Message: message}
}

// GetStatus returns the HTTP status of the error.
func (e *ErrorMessage) GetStatus() HTTPStatus { return e.Status }

// SetStatus sets the HTTP status of the error.
func (e *ErrorMessage) SetStatus(status HTTPStatus) { e.Status = status }

// GetMessage returns the human-readable error message.
func (e *ErrorMessage) GetMessage() string { return e.Message }

// SetMessage sets the human-readable error message.
func (e *ErrorMessage) SetMessage(message string) { e.Message = message }

// Equal reports whether e and other hold the same status and message.
// Two nil pointers are equal; a nil and a non-nil pointer are not.
func (e *ErrorMessage) Equal(other *ErrorMessage) bool {
	if e == nil || other == nil {
		return e == other
	}
	return e.Status == other.Status && e.Message == other.Message
}

// String renders the ErrorMessage in Lombok's toString format, e.g.
// "ErrorMessage(status=NOT_FOUND, message=User are not available)".
// Unset fields render as "null", matching Java's rendering of null fields.
func (e *ErrorMessage) String() string {
	if e == nil {
		return "null"
	}
	status := string(e.Status)
	if status == "" {
		status = "null"
	}
	message := e.Message
	if message == "" {
		message = "null"
	}
	return "ErrorMessage(status=" + status + ", message=" + message + ")"
}
