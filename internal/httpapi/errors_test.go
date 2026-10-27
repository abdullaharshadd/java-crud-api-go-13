package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

func decodeErrorMessage(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("invalid JSON body %q: %v", body, err)
	}
	return m
}

func TestWriteError_UserNotFound(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"sentinel", service.ErrUserNotFound, "User are not available"},
		{"wrapped sentinel drops context", fmt.Errorf("lookup id 5: %w", service.ErrUserNotFound), "User are not available"},
		{"custom message", service.NewUserNotFoundErrorMsg("User not found with id 5"), "User not found with id 5"},
		{"no message falls back", service.NewUserNotFoundError(), "User are not available"},
		{"empty message falls back", service.NewUserNotFoundErrorMsg(""), "User are not available"},
		{"from cause", service.NewUserNotFoundErrorFromCause(errors.New("db gone")), "db gone"},
		{"with cause", service.NewUserNotFoundErrorWithCause("missing", errors.New("x")), "missing"},
		{"wrapped struct", fmt.Errorf("ctx: %w", service.NewUserNotFoundErrorMsg("User not found with id 7")), "User not found with id 7"},
	}
	paths := []string{"/users/5", "/other/endpoint"}
	for _, tt := range tests {
		for _, p := range paths {
			t.Run(tt.name+p, func(t *testing.T) {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, p, nil)
				WriteError(rec, req, tt.err)
				if rec.Code != http.StatusNotFound {
					t.Fatalf("status = %d, want 404", rec.Code)
				}
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Errorf("content-type = %q", ct)
				}
				var em model.ErrorMessage
				if err := json.Unmarshal(rec.Body.Bytes(), &em); err != nil {
					t.Fatalf("decode: %v", err)
				}
				want := model.NewErrorMessage(model.HTTPStatusNotFound, tt.wantMsg)
				if !want.Equal(&em) {
					t.Errorf("body = %v, want %v", em.String(), want.String())
				}
				m := decodeErrorMessage(t, rec.Body.String())
				if len(m) != 2 {
					t.Errorf("unexpected fields: %v", m)
				}
			})
		}
	}
}

func TestErrorsFile_WriteErrorStandardErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  int
		wantAllow string
	}{
		{"method not allowed", &MethodNotAllowedError{Method: "DELETE", Allowed: []string{"GET", "POST"}}, 405, "GET, POST"},
		{"method not allowed no allowed", &MethodNotAllowedError{Method: "PUT"}, 405, ""},
		{"wrapped method not allowed", fmt.Errorf("x: %w", &MethodNotAllowedError{Method: "PUT", Allowed: []string{"GET"}}), 405, "GET"},
		{"bad request sentinel", ErrBadRequest, 400, ""},
		{"missing param", BadRequest(errors.New("missing required parameter 'id'")), 400, ""},
		{"validation failed", BadRequest(errors.New("name must not be blank")), 400, ""},
		{"bad request nil cause", BadRequest(nil), 400, ""},
		{"fmt wrapped bad request", fmt.Errorf("decode: %w", ErrBadRequest), 400, ""},
		{"unsupported media", ErrUnsupportedMediaType, 415, ""},
		{"not acceptable", ErrNotAcceptable, 406, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, httptest.NewRequest(http.MethodGet, "/users", nil), tt.err)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Allow"); got != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rec.Body.String())
			}
		})
	}
}

func TestMethodNotAllowedError_Error(t *testing.T) {
	e := &MethodNotAllowedError{Method: "PATCH"}
	if got := e.Error(); got != "request method 'PATCH' not supported" {
		t.Errorf("Error() = %q", got)
	}
}

func TestBadRequest_Wraps(t *testing.T) {
	cause := errors.New("boom")
	err := BadRequest(cause)
	if !errors.Is(err, ErrBadRequest) || !errors.Is(err, cause) {
		t.Errorf("BadRequest should wrap both sentinel and cause")
	}
	if BadRequest(nil) != ErrBadRequest {
		t.Errorf("BadRequest(nil) should return ErrBadRequest")
	}
}

func checkDefaultBody(t *testing.T, rec *httptest.ResponseRecorder, code int, path string) {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("status = %d, want %d", rec.Code, code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	var b defaultErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b.Status != code || b.Error != http.StatusText(code) || b.Path != path {
		t.Errorf("body = %+v", b)
	}
	if _, err := time.Parse(springTimestampLayout, b.Timestamp); err != nil {
		t.Errorf("timestamp %q: %v", b.Timestamp, err)
	}
	m := decodeErrorMessage(t, rec.Body.String())
	if _, ok := m["message"]; ok {
		t.Errorf("default body must not be ErrorMessage format: %v", m)
	}
}

func TestWriteDefaultError(t *testing.T) {
	for _, code := range []int{400, 404, 405, 415, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteDefaultError(rec, httptest.NewRequest(http.MethodGet, "/a/b", nil), code)
			checkDefaultBody(t, rec, code, "/a/b")
		})
	}
}

func TestNotFoundHandler(t *testing.T) {
	for _, p := range []string{"/nope", "/users/x/y"} {
		t.Run(p, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NotFoundHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
			checkDefaultBody(t, rec, 404, p)
		})
	}
}

func TestRecoverer(t *testing.T) {
	t.Run("panic becomes 500", func(t *testing.T) {
		h := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("kaboom") }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))
		checkDefaultBody(t, rec, 500, "/boom")
	})
	t.Run("no panic passes through", func(t *testing.T) {
		h := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("ok"))
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusTeapot || strings.TrimSpace(rec.Body.String()) != "ok" {
			t.Errorf("got %d %q", rec.Code, rec.Body.String())
		}
	})
	t.Run("ErrAbortHandler re-panics", func(t *testing.T) {
		h := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
		defer func() {
			if r := recover(); r != http.ErrAbortHandler {
				t.Errorf("recovered %v, want ErrAbortHandler", r)
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

func TestWriteJSON_MarshalFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, make(chan int))
	if rec.Code != http.StatusInternalServerError || rec.Body.Len() != 0 {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}