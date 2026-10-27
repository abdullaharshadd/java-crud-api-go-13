package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"migrated-app/internal/model"
	"migrated-app/internal/service"
)

// Fixed plain-text response bodies, kept verbatim from the Java controller.
const (
	msgUserSaved   = "User data saved successfully!"
	msgUserDeleted = "user data deleted Successfully"

	contentTypeText = "text/plain;charset=UTF-8"
	contentTypeJSON = "application/json"
)

// Handler exposes the User CRUD endpoints (Java: UserController). It holds no
// business logic; everything is delegated to service.UserService.
type Handler struct {
	svc service.UserService
	log zerolog.Logger
}

// NewHandler returns a Handler that delegates to svc and logs with logger.
func NewHandler(svc service.UserService, logger zerolog.Logger) *Handler {
	return &Handler{svc: svc, log: logger}
}

// Register mounts every User route on mux using Go 1.22 method+path patterns.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /save_user_data", h.SaveUser)
	mux.HandleFunc("GET /get_user_data", h.FetchUserList)
	mux.HandleFunc("GET /get_user_data/{id}", h.FetchUserByID)
	mux.HandleFunc("DELETE /delete_user_data/{id}", h.DeleteUser)
	mux.HandleFunc("PUT /update_user_data/{id}", h.UpdateUser)
	// Java mapping was "get_user_name/name/{name}"; Spring prepends the slash.
	mux.HandleFunc("GET /get_user_name/name/{name}", h.GetUserNameByName)
}

// Routes returns a complete http.Handler with all User routes registered,
// wrapped in panic recovery and trailing-slash normalisation (Spring Boot 2.7
// matched "/path/" to "/path" by default).
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	h.Register(mux)
	return Recoverer(StripTrailingSlash(mux))
}

// StripTrailingSlash removes a single trailing slash from the request path
// (except for the root path) before passing the request on, mirroring Spring
// MVC's default trailing-slash matching.
func StripTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if len(p) > 1 && strings.HasSuffix(p, "/") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimSuffix(p, "/")
			if r2.URL.RawPath != "" {
				r2.URL.RawPath = strings.TrimSuffix(r2.URL.RawPath, "/")
			}
			r = r2
		}
		next.ServeHTTP(w, r)
	})
}

// SaveUser handles POST /save_user_data: decodes and validates the body,
// persists it and answers with a fixed plain-text confirmation.
func (h *Handler) SaveUser(w http.ResponseWriter, r *http.Request) {
	h.log.Info().Msg("inside the saveUser of UserController ")

	u, err := decodeUser(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	// @Valid: bean validation failure -> MethodArgumentNotValidException -> 400.
	if err := u.Validate(); err != nil {
		WriteError(w, r, BadRequest(err))
		return
	}
	if _, err := h.svc.SaveUser(r.Context(), u); err != nil {
		WriteError(w, r, err)
		return
	}
	writeText(w, http.StatusOK, msgUserSaved)
}

// FetchUserList handles GET /get_user_data and returns all users as a JSON
// array ([] when there are none).
func (h *Handler) FetchUserList(w http.ResponseWriter, r *http.Request) {
	h.log.Info().Msg("inside the fetchUserList of UserController ")

	users, err := h.svc.FetchUserList(r.Context())
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if users == nil {
		users = make([]*model.User, 0)
	}
	writeJSON(w, http.StatusOK, users)
}

// FetchUserByID handles GET /get_user_data/{id}. A missing user yields the
// 404 ErrorMessage body via WriteError.
func (h *Handler) FetchUserByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	u, err := h.svc.FetchUserByID(r.Context(), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// DeleteUser handles DELETE /delete_user_data/{id}. Deleting a missing user
// surfaces as a 500 (parity with Spring Data's EmptyResultDataAccessException).
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if err := h.svc.DeleteUser(r.Context(), id); err != nil {
		WriteError(w, r, err)
		return
	}
	writeText(w, http.StatusOK, msgUserDeleted)
}

// UpdateUser handles PUT /update_user_data/{id}. No validation is applied
// (the Java method had no @Valid). The response echoes the request body with
// its id set to the path id — the Java service mutated the same object via
// user.setId(id) — rather than the persisted entity.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	u, err := decodeUser(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if err := h.svc.UpdateUser(r.Context(), id, u); err != nil {
		WriteError(w, r, err)
		return
	}
	u.ID = id
	writeJSON(w, http.StatusOK, u)
}

// GetUserNameByName handles GET /get_user_name/name/{name}. When no user
// matches, Spring wrote 200 with an empty body; that quirk is preserved.
func (h *Handler) GetUserNameByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	u, err := h.svc.GetUserNameByName(r.Context(), name)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if u == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// pathID parses the {id} path segment as a 32-bit signed integer, as Spring's
// String->int conversion did. Failure maps to an empty-body 400.
func pathID(r *http.Request) (int32, error) {
	raw := r.PathValue("id")
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, BadRequest(fmt.Errorf("invalid id %q: %w", raw, err))
	}
	return int32(n), nil
}

// decodeUser enforces a JSON Content-Type (415 otherwise) and decodes the
// body into a User. Unknown properties are ignored, matching Jackson's
// Spring Boot defaults. An empty, malformed or literal-null body is a 400.
func decodeUser(r *http.Request) (*model.User, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return nil, ErrUnsupportedMediaType
	}
	var u *model.User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		return nil, BadRequest(fmt.Errorf("decode user: %w", err))
	}
	if u == nil {
		return nil, BadRequest(errors.New("required request body is missing"))
	}
	return u, nil
}

// isJSONContentType reports whether ct is application/json or an
// application/*+json type, which Spring's Jackson converter accepts.
func isJSONContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == contentTypeJSON || (strings.HasPrefix(mt, "application/") && strings.HasSuffix(mt, "+json"))
}

// writeText writes a plain-text body with Spring's text/plain content type.
func writeText(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", contentTypeText)
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}
