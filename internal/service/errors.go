// Package service contains the business logic layer of the application.
package service

import "errors"

// ErrUserNotFound signals that a requested user does not exist.
//
// The text is kept exactly as the source's service layer used it
// ("User are not available") because the HTTP layer echoes it verbatim in
// the 404 ErrorMessage body. Callers either return it directly, wrap it with
// fmt.Errorf("...: %w", ErrUserNotFound), or build a *UserNotFoundError when
// they need a custom message or cause. In all cases errors.Is(err,
// ErrUserNotFound) reports true.
var ErrUserNotFound = errors.New("User are not available")

// UserNotFoundError is the structured form of ErrUserNotFound, mirroring the
// Java UserNotFoundException's optional detail message and cause.
//
// MIGRATION_NOTE: Java's five constructor overloads collapse into the
// constructor functions below. The protected
// (message, cause, enableSuppression, writableStackTrace) constructor has no
// Go equivalent: Go errors carry no stack traces or suppressed-exception
// lists, so NewUserNotFoundErrorWithCause covers its message/cause semantics
// and the two boolean flags are intentionally dropped.
type UserNotFoundError struct {
	// Message is the optional detail message ("" means none).
	Message string
	// Cause is the optional underlying error (nil means none).
	Cause error
}

// NewUserNotFoundError returns a UserNotFoundError with no detail message
// and no cause (Java: UserNotFoundException()).
func NewUserNotFoundError() *UserNotFoundError {
	return &UserNotFoundError{}
}

// NewUserNotFoundErrorMsg returns a UserNotFoundError with the given detail
// message and no cause (Java: UserNotFoundException(String)).
func NewUserNotFoundErrorMsg(message string) *UserNotFoundError {
	return &UserNotFoundError{Message: message}
}

// NewUserNotFoundErrorWithCause returns a UserNotFoundError with the given
// detail message and underlying cause
// (Java: UserNotFoundException(String, Throwable)).
func NewUserNotFoundErrorWithCause(message string, cause error) *UserNotFoundError {
	return &UserNotFoundError{Message: message, Cause: cause}
}

// NewUserNotFoundErrorFromCause returns a UserNotFoundError wrapping cause;
// its message is derived from the cause (Java: UserNotFoundException(Throwable)).
func NewUserNotFoundErrorFromCause(cause error) *UserNotFoundError {
	return &UserNotFoundError{Cause: cause}
}

// Error implements the error interface. It returns the detail message if
// set, otherwise the cause's message, otherwise the ErrUserNotFound text.
func (e *UserNotFoundError) Error() string {
	if e == nil {
		return ErrUserNotFound.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	// MIGRATION_NOTE: Java's no-arg exception has a null message; Go errors
	// must return a string, so fall back to the sentinel's text.
	return ErrUserNotFound.Error()
}

// Unwrap exposes the underlying cause to errors.Is / errors.As.
func (e *UserNotFoundError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is makes errors.Is(err, ErrUserNotFound) report true for any
// *UserNotFoundError.
func (e *UserNotFoundError) Is(target error) bool {
	return target == ErrUserNotFound
}
