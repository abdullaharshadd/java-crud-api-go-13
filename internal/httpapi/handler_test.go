package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

type fakeService struct {
	saveCalls   int
	savedUser   *model.User
	saveErr     error
	listCalls   int
	list        []*model.User
	listErr     error
	byIDCalls   int
	byIDArg     int32
	byIDUser    *model.User
	byIDErr     error
	delCalls    int
	delArg      int32
	delErr      error
	updCalls    int
	updID       int32
	updUser     *model.User
	updErr      error
	nameCalls   int
	nameArg     string
	nameUser    *model.User
	nameErr     error
}

func (f *fakeService) SaveUser(ctx context.Context, u *model.User) (*model.User, error) {
	f.saveCalls++
	f.savedUser = u
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return u, nil
}
func (f *fakeService) FetchUserList(ctx context.Context) ([]*model.User, error) {
	f.listCalls++
	return f.list, f.listErr
}
func (f *fakeService) FetchUserByID(ctx context.Context, id int32) (*model.User, error) {
	f.byIDCalls++
	f.byIDArg = id
	return f.byIDUser, f.byIDErr
}
func (f *fakeService) DeleteUser(ctx context.Context, id int32) error {
	f.delCalls++
	f.delArg = id
	return f.delErr
}
func (f *fakeService) UpdateUser(ctx context.Context, id int32, u *model.User) error {
	f.updCalls++
	f.updID = id
	f.updUser = u
	return f.updErr
}
func (f *fakeService) GetUserNameByName(ctx context.Context, name string) (*model.User, error) {
	f.nameCalls++
	f.nameArg = name
	return f.nameUser, f.nameErr
}

var _ service.UserService = (*fakeService)(nil)

func do(t *testing.T, svc *fakeService, method, path, ct, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(svc, zerolog.Nop())
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func sp(s string) *string { return &s }

func TestSaveUser(t *testing.T) {
	tests := []struct {
		name      string
		ct, body  string
		saveErr   error
		code      int
		wantBody  string
		wantCalls int
	}{
		{"valid", "application/json", `{"name":"alice","email":"a@x","extra":1}`, nil, 200, msgUserSaved, 1},
		{"valid charset", "application/json; charset=UTF-8", `{"name":"alice"}`, nil, 200, msgUserSaved, 1},
		{"blank name", "application/json", `{"name":"   "}`, nil, 400, "", 0},
		{"missing name", "application/json", `{"email":"a"}`, nil, 400, "", 0},
		{"malformed", "application/json", `{"name":`, nil, 400, "", 0},
		{"empty body", "application/json", "", nil, 400, "", 0},
		{"null body", "application/json", "null", nil, 400, "", 0},
		{"wrong content type", "text/plain", `{"name":"a"}`, nil, 415, "", 0},
		{"service error", "application/json", `{"name":"a"}`, errors.New("db"), 500, "", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{saveErr: tc.saveErr}
			rec := do(t, svc, http.MethodPost, "/save_user_data", tc.ct, tc.body)
			if rec.Code != tc.code {
				t.Fatalf("code=%d want %d", rec.Code, tc.code)
			}
			if svc.saveCalls != tc.wantCalls {
				t.Fatalf("calls=%d want %d", svc.saveCalls, tc.wantCalls)
			}
			if tc.code == 200 {
				if rec.Body.String() != tc.wantBody {
					t.Fatalf("body=%q", rec.Body.String())
				}
				if ct := rec.Header().Get("Content-Type"); ct != contentTypeText {
					t.Fatalf("ct=%q", ct)
				}
				if svc.savedUser == nil || svc.savedUser.Name == nil || *svc.savedUser.Name == "" {
					t.Fatalf("user not passed")
				}
			} else if tc.code != 500 && rec.Body.Len() != 0 {
				t.Fatalf("expected empty body, got %q", rec.Body.String())
			}
			if tc.code == 500 && strings.Contains(rec.Body.String(), msgUserSaved) {
				t.Fatal("success message on error")
			}
		})
	}
}

func TestFetchUserList(t *testing.T) {
	tests := []struct {
		name string
		list []*model.User
		err  error
		code int
		want string
	}{
		{"users", []*model.User{model.NewUserBuilder().ID(2).Name("b").Build(), model.NewUserBuilder().ID(1).Name("a").Build()}, nil, 200,
			`[{"id":2,"name":"b","email":null,"password":null,"role":null,"about":null},{"id":1,"name":"a","email":null,"password":null,"role":null,"about":null}]`},
		{"nil", nil, nil, 200, `[]`},
		{"empty", []*model.User{}, nil, 200, `[]`},
		{"error", nil, errors.New("x"), 500, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{list: tc.list, listErr: tc.err}
			rec := do(t, svc, http.MethodGet, "/get_user_data", "", "")
			if rec.Code != tc.code || svc.listCalls != 1 {
				t.Fatalf("code=%d calls=%d", rec.Code, svc.listCalls)
			}
			if tc.want != "" && rec.Body.String() != tc.want {
				t.Fatalf("body=%s", rec.Body.String())
			}
		})
	}
}

func TestFetchUserByID(t *testing.T) {
	u := model.NewUserBuilder().ID(5).Name("e").Build()
	tests := []struct {
		name  string
		path  string
		user  *model.User
		err   error
		code  int
		calls int
		id    int32
	}{
		{"found", "/get_user_data/5", u, nil, 200, 1, 5},
		{"trailing slash", "/get_user_data/5/", u, nil, 200, 1, 5},
		{"not found", "/get_user_data/9", nil, service.NewUserNotFoundErrorMsg(service.ErrUserNotFound.Error()), 404, 1, 9},
		{"bad id", "/get_user_data/abc", nil, nil, 400, 0, 0},
		{"overflow id", "/get_user_data/99999999999", nil, nil, 400, 0, 0},
		{"other error", "/get_user_data/1", nil, errors.New("db"), 500, 1, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{byIDUser: tc.user, byIDErr: tc.err}
			rec := do(t, svc, http.MethodGet, tc.path, "", "")
			if rec.Code != tc.code || svc.byIDCalls != tc.calls {
				t.Fatalf("code=%d calls=%d", rec.Code, svc.byIDCalls)
			}
			if tc.calls == 1 && svc.byIDArg != tc.id {
				t.Fatalf("id=%d", svc.byIDArg)
			}
			switch tc.code {
			case 200:
				var got model.User
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Equal(u) {
					t.Fatalf("body=%s", rec.Body.String())
				}
			case 404:
				var m map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
					t.Fatal(err)
				}
				if m["message"] != "User are not available" {
					t.Fatalf("body=%s", rec.Body.String())
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		err   error
		code  int
		calls int
	}{
		{"ok", "/delete_user_data/3", nil, 200, 1},
		{"service error", "/delete_user_data/3", errors.New("not exist"), 500, 1},
		{"bad id", "/delete_user_data/x", nil, 400, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{delErr: tc.err}
			rec := do(t, svc, http.MethodDelete, tc.path, "", "")
			if rec.Code != tc.code || svc.delCalls != tc.calls {
				t.Fatalf("code=%d calls=%d", rec.Code, svc.delCalls)
			}
			if tc.code == 200 && (rec.Body.String() != "user data deleted Successfully" || svc.delArg != 3) {
				t.Fatalf("body=%q id=%d", rec.Body.String(), svc.delArg)
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		ct    string
		body  string
		err   error
		code  int
		calls int
	}{
		{"ok", "/update_user_data/7", "application/json", `{"id":1,"name":"n","email":"e"}`, nil, 200, 1},
		{"blank allowed", "/update_user_data/7", "application/json", `{"name":""}`, nil, 200, 1},
		{"service error", "/update_user_data/7", "application/json", `{"name":"n"}`, errors.New("x"), 500, 1},
		{"malformed", "/update_user_data/7", "application/json", `{`, nil, 400, 0},
		{"bad id", "/update_user_data/z", "application/json", `{"name":"n"}`, nil, 400, 0},
		{"bad ct", "/update_user_data/7", "text/plain", `{"name":"n"}`, nil, 415, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{updErr: tc.err}
			rec := do(t, svc, http.MethodPut, tc.path, tc.ct, tc.body)
			if rec.Code != tc.code || svc.updCalls != tc.calls {
				t.Fatalf("code=%d calls=%d", rec.Code, svc.updCalls)
			}
			if tc.calls == 1 && svc.updID != 7 {
				t.Fatalf("id=%d", svc.updID)
			}
			if tc.code == 200 {
				var got model.User
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				var in model.User
				_ = json.Unmarshal([]byte(tc.body), &in)
				in.ID = 7
				if !got.Equal(&in) {
					t.Fatalf("got %v want %v", &got, &in)
				}
			}
		})
	}
}

func TestGetUserNameByName(t *testing.T) {
	u := model.NewUser(1, sp("john doe"), nil, nil, nil, nil)
	tests := []struct {
		name    string
		path    string
		user    *model.User
		err     error
		code    int
		wantArg string
		empty   bool
	}{
		{"found", "/get_user_name/name/john%20doe", u, nil, 200, "john doe", false},
		{"none", "/get_user_name/name/bob", nil, nil, 200, "bob", true},
		{"error", "/get_user_name/name/bob", nil, errors.New("x"), 500, "bob", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{nameUser: tc.user, nameErr: tc.err}
			rec := do(t, svc, http.MethodGet, tc.path, "", "")
			if rec.Code != tc.code || svc.nameCalls != 1 || svc.nameArg != tc.wantArg {
				t.Fatalf("code=%d calls=%d arg=%q", rec.Code, svc.nameCalls, svc.nameArg)
			}
			if tc.empty && rec.Body.Len() != 0 {
				t.Fatalf("body=%q", rec.Body.String())
			}
			if tc.user != nil {
				var got model.User
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Equal(u) {
					t.Fatalf("body=%s", rec.Body.String())
				}
			}
		})
	}
}

func TestIsJSONContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"", false},
		{"application/json", true},
		{"application/json;charset=UTF-8", true},
		{"application/vnd.api+json", true},
		{"text/json", false},
		{"text/plain", false},
		{";;bad", false},
	}
	for _, tc := range tests {
		if got := isJSONContentType(tc.ct); got != tc.want {
			t.Errorf("%q: got %v", tc.ct, got)
		}
	}
}