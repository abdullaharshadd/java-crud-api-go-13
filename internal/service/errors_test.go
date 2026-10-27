package service

import (
	"errors"
	"fmt"
	"testing"
)

type customErr struct{ msg string }

func (c *customErr) Error() string { return c.msg }

func TestErrUserNotFoundSentinel(t *testing.T) {
	if ErrUserNotFound.Error() != "User are not available" {
		t.Fatalf("unexpected sentinel text %q", ErrUserNotFound.Error())
	}
	wrapped := fmt.Errorf("lookup: %w", ErrUserNotFound)
	if !errors.Is(wrapped, ErrUserNotFound) {
		t.Fatal("wrapped sentinel should match errors.Is")
	}
}

func TestConstructors(t *testing.T) {
	dbErr := errors.New("db")
	stateErr := &customErr{msg: "x"}

	tests := []struct {
		name      string
		err       *UserNotFoundError
		wantMsg   string
		wantField string
		wantCause error
	}{
		{"no args", NewUserNotFoundError(), ErrUserNotFound.Error(), "", nil},
		{"msg", NewUserNotFoundErrorMsg("User not found with id 5"), "User not found with id 5", "User not found with id 5", nil},
		{"empty msg", NewUserNotFoundErrorMsg(""), ErrUserNotFound.Error(), "", nil},
		{"msg and cause", NewUserNotFoundErrorWithCause("not found", dbErr), "not found", "not found", dbErr},
		{"msg and nil cause", NewUserNotFoundErrorWithCause("not found", nil), "not found", "not found", nil},
		{"from cause", NewUserNotFoundErrorFromCause(stateErr), "x", "", stateErr},
		{"from nil cause", NewUserNotFoundErrorFromCause(nil), ErrUserNotFound.Error(), "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("constructor returned nil")
			}
			if tt.err.Message != tt.wantField {
				t.Errorf("Message = %q, want %q", tt.err.Message, tt.wantField)
			}
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
			if tt.err.Cause != tt.wantCause {
				t.Errorf("Cause = %v, want %v (identity)", tt.err.Cause, tt.wantCause)
			}
			if u := tt.err.Unwrap(); u != tt.wantCause {
				t.Errorf("Unwrap = %v, want %v", u, tt.wantCause)
			}
			if tt.wantCause != nil && !errors.Is(tt.err, tt.wantCause) {
				t.Error("errors.Is should find the cause")
			}

			var e error = tt.err
			if !errors.Is(e, ErrUserNotFound) {
				t.Error("errors.Is(err, ErrUserNotFound) should be true")
			}
			var target *UserNotFoundError
			if !errors.As(fmt.Errorf("wrap: %w", e), &target) || target != tt.err {
				t.Error("errors.As should recover the same *UserNotFoundError")
			}
		})
	}
}

func TestMessageNotDerivedFromCause(t *testing.T) {
	err := NewUserNotFoundErrorWithCause("not found", errors.New("db"))
	if err.Error() != "not found" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestErrorsAsCustomCause(t *testing.T) {
	cause := &customErr{msg: "x"}
	err := NewUserNotFoundErrorFromCause(cause)
	var ce *customErr
	if !errors.As(err, &ce) || ce != cause {
		t.Fatal("errors.As should find cause by identity")
	}
}

func TestIsOnlyMatchesSentinel(t *testing.T) {
	err := NewUserNotFoundError()
	tests := []struct {
		name   string
		target error
		want   bool
	}{
		{"sentinel", ErrUserNotFound, true},
		{"same text other error", errors.New("User are not available"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := err.Is(tt.target); got != tt.want {
				t.Errorf("Is = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNilReceiver(t *testing.T) {
	var e *UserNotFoundError
	if e.Error() != ErrUserNotFound.Error() {
		t.Errorf("nil Error() = %q", e.Error())
	}
	if e.Unwrap() != nil {
		t.Error("nil Unwrap should be nil")
	}
	if !e.Is(ErrUserNotFound) {
		t.Error("nil Is(sentinel) should be true")
	}
}

func TestConstructorsReturnDistinctInstances(t *testing.T) {
	a, b := NewUserNotFoundError(), NewUserNotFoundError()
	if a == b {
		t.Fatal("constructors should return distinct instances")
	}
}