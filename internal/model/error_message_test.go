package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewEmptyErrorMessage(t *testing.T) {
	e := NewEmptyErrorMessage()
	if e == nil {
		t.Fatal("expected non-nil")
	}
	if e.GetStatus() != "" || e.GetMessage() != "" {
		t.Fatalf("expected empty fields, got %+v", e)
	}
}

func TestNewErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		status  HTTPStatus
		message string
	}{
		{"not found", HTTPStatusNotFound, "Contact not found"},
		{"empty", "", ""},
		{"bad request", HTTPStatusBadRequest, "Invalid input"},
		{"untransformed", HTTPStatus("weird"), "  spaced  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewErrorMessage(tt.status, tt.message)
			if e.GetStatus() != tt.status {
				t.Errorf("status = %q, want %q", e.GetStatus(), tt.status)
			}
			if e.GetMessage() != tt.message {
				t.Errorf("message = %q, want %q", e.GetMessage(), tt.message)
			}
		})
	}
}

func TestErrorMessageSetStatus(t *testing.T) {
	tests := []struct {
		name   string
		status HTTPStatus
	}{
		{"bad request", HTTPStatusBadRequest},
		{"unset", ""},
		{"internal", HTTPStatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewErrorMessage(HTTPStatusNotFound, "keep")
			e.SetStatus(tt.status)
			if e.GetStatus() != tt.status {
				t.Errorf("status = %q, want %q", e.GetStatus(), tt.status)
			}
			if e.GetMessage() != "keep" {
				t.Errorf("message altered: %q", e.GetMessage())
			}
		})
	}
}

func TestErrorMessageSetMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"text", "Invalid input"},
		{"empty", ""},
		{"whitespace", "  x \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewErrorMessage(HTTPStatusNotFound, "orig")
			e.SetMessage(tt.message)
			if e.GetMessage() != tt.message {
				t.Errorf("message = %q, want %q", e.GetMessage(), tt.message)
			}
			if e.GetStatus() != HTTPStatusNotFound {
				t.Errorf("status altered: %q", e.GetStatus())
			}
		})
	}
}

func TestErrorMessageEqual(t *testing.T) {
	var nilE *ErrorMessage
	tests := []struct {
		name string
		a, b *ErrorMessage
		want bool
	}{
		{"identical", NewErrorMessage(HTTPStatusNotFound, "x"), NewErrorMessage(HTTPStatusNotFound, "x"), true},
		{"both empty", NewEmptyErrorMessage(), NewEmptyErrorMessage(), true},
		{"diff status", NewErrorMessage(HTTPStatusNotFound, "x"), NewErrorMessage(HTTPStatusBadRequest, "x"), false},
		{"diff message", NewErrorMessage(HTTPStatusNotFound, "x"), NewErrorMessage(HTTPStatusNotFound, "y"), false},
		{"other nil", NewErrorMessage(HTTPStatusNotFound, "x"), nil, false},
		{"receiver nil", nilE, NewEmptyErrorMessage(), false},
		{"both nil", nilE, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("a.Equal(b) = %v, want %v", got, tt.want)
			}
			if got := tt.b.Equal(tt.a); got != tt.want {
				t.Errorf("symmetry: b.Equal(a) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorMessageEqualReflexiveTransitive(t *testing.T) {
	a := NewErrorMessage(HTTPStatusNotFound, "x")
	b := NewErrorMessage(HTTPStatusNotFound, "x")
	c := NewErrorMessage(HTTPStatusNotFound, "x")
	if !a.Equal(a) {
		t.Error("not reflexive")
	}
	if !(a.Equal(b) && b.Equal(c) && a.Equal(c)) {
		t.Error("not transitive")
	}
	if *a != *b {
		t.Error("equal values should compare equal as map keys")
	}
	m := map[ErrorMessage]int{*a: 1}
	if m[*b] != 1 {
		t.Error("equal values should hash identically")
	}
}

func TestErrorMessageString(t *testing.T) {
	var nilE *ErrorMessage
	tests := []struct {
		name string
		e    *ErrorMessage
		want string
	}{
		{"full", NewErrorMessage(HTTPStatusNotFound, "x"), "ErrorMessage(status=NOT_FOUND, message=x)"},
		{"empty", NewEmptyErrorMessage(), "ErrorMessage(status=null, message=null)"},
		{"status only", NewErrorMessage(HTTPStatusBadRequest, ""), "ErrorMessage(status=BAD_REQUEST, message=null)"},
		{"message only", NewErrorMessage("", "m"), "ErrorMessage(status=null, message=m)"},
		{"nil", nilE, "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHTTPStatusFromCode(t *testing.T) {
	tests := []struct {
		code   int
		want   HTTPStatus
		wantOK bool
	}{
		{http.StatusNotFound, HTTPStatusNotFound, true},
		{http.StatusBadRequest, HTTPStatusBadRequest, true},
		{http.StatusInternalServerError, HTTPStatusInternalServerError, true},
		{http.StatusOK, "OK", true},
		{http.StatusTeapot, "IM_A_TEAPOT", true},
		{http.StatusNonAuthoritativeInfo, "NON_AUTHORITATIVE_INFORMATION", true},
		{999, "", false},
		{0, "", false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.code), func(t *testing.T) {
			got, ok := HTTPStatusFromCode(tt.code)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("HTTPStatusFromCode(%d) = (%q,%v), want (%q,%v)", tt.code, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestErrorMessageJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		e    *ErrorMessage
		want string
	}{
		{"not found", NewErrorMessage(HTTPStatusNotFound, "User are not available"), `{"status":"NOT_FOUND","message":"User are not available"}`},
		{"empty", NewEmptyErrorMessage(), `{"status":"","message":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.e)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.want {
				t.Errorf("json = %s, want %s", b, tt.want)
			}
			back := NewEmptyErrorMessage()
			if err := json.Unmarshal(b, back); err != nil {
				t.Fatal(err)
			}
			if !back.Equal(tt.e) {
				t.Errorf("round trip mismatch: %v vs %v", back, tt.e)
			}
		})
	}
}

func TestErrorMessageJSONNullDecodes(t *testing.T) {
	e := NewEmptyErrorMessage()
	if err := json.Unmarshal([]byte(`{"status":null,"message":null}`), e); err != nil {
		t.Fatal(err)
	}
	if !e.Equal(NewEmptyErrorMessage()) {
		t.Errorf("got %v", e)
	}
}

func TestErrorMessageInHTTPResponse(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st, _ := HTTPStatusFromCode(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(NewErrorMessage(st, "User are not available"))
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/1", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2 || raw["status"] != "NOT_FOUND" || raw["message"] != "User are not available" {
		t.Errorf("unexpected body: %v", raw)
	}
}