package service

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/model"
	"migrated-app/internal/repository"
)

type fakeRepo struct {
	insertFn     func(ctx context.Context, u *model.User) error
	updateFn     func(ctx context.Context, u *model.User) error
	findAllFn    func(ctx context.Context) ([]*model.User, error)
	findByIDFn   func(ctx context.Context, id int32) (*model.User, bool, error)
	findByNameFn func(ctx context.Context, name string) (*model.User, bool, error)
	deleteFn     func(ctx context.Context, id int32) error

	insertCalls, updateCalls, findAllCalls, findByIDCalls, findByNameCalls, deleteCalls int
	lastFindID, lastDeleteID                                                             int32
	lastName                                                                             string
	updatedIDs                                                                           []int32
}

var _ repository.UserRepository = (*fakeRepo)(nil)

func (f *fakeRepo) Insert(ctx context.Context, u *model.User) error {
	f.insertCalls++
	if f.insertFn != nil {
		return f.insertFn(ctx, u)
	}
	u.SetID(100)
	return nil
}

func (f *fakeRepo) Update(ctx context.Context, u *model.User) error {
	f.updateCalls++
	f.updatedIDs = append(f.updatedIDs, u.GetID())
	if f.updateFn != nil {
		return f.updateFn(ctx, u)
	}
	return nil
}

func (f *fakeRepo) Save(ctx context.Context, u *model.User) (*model.User, error) {
	return nil, errors.New("Save should not be called by the service")
}

func (f *fakeRepo) FindAll(ctx context.Context) ([]*model.User, error) {
	f.findAllCalls++
	if f.findAllFn != nil {
		return f.findAllFn(ctx)
	}
	return nil, nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id int32) (*model.User, bool, error) {
	f.findByIDCalls++
	f.lastFindID = id
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, false, nil
}

func (f *fakeRepo) FindByName(ctx context.Context, name string) (*model.User, bool, error) {
	f.findByNameCalls++
	f.lastName = name
	if f.findByNameFn != nil {
		return f.findByNameFn(ctx, name)
	}
	return nil, false, nil
}

func (f *fakeRepo) DeleteByID(ctx context.Context, id int32) error {
	f.deleteCalls++
	f.lastDeleteID = id
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

func (f *fakeRepo) Count(ctx context.Context) (int64, error)            { return 0, nil }
func (f *fakeRepo) ExistsByID(ctx context.Context, id int32) (bool, error) { return false, nil }

func sp(s string) *string { return &s }

func TestSaveUser(t *testing.T) {
	dbErr := errors.New("duplicate email")
	tests := []struct {
		name        string
		user        *model.User
		repo        *fakeRepo
		wantErr     bool
		wantErrIs   error
		wantID      int32
		wantInserts int
		wantUpdates int
	}{
		{
			name:        "new user gets generated id",
			user:        model.NewUserBuilder().Name("a").Email("a@x").Build(),
			repo:        &fakeRepo{},
			wantID:      100,
			wantInserts: 1,
		},
		{
			name:        "existing id is updated",
			user:        model.NewUserBuilder().ID(5).Name("b").Build(),
			repo:        &fakeRepo{},
			wantID:      5,
			wantUpdates: 1,
		},
		{
			name: "nonexistent id inserts with new id",
			user: model.NewUserBuilder().ID(77).Name("c").Build(),
			repo: &fakeRepo{updateFn: func(context.Context, *model.User) error {
				return repository.ErrUserNotExist
			}},
			wantID:      100,
			wantUpdates: 1,
			wantInserts: 1,
		},
		{
			name: "update db error propagates",
			user: model.NewUserBuilder().ID(5).Build(),
			repo: &fakeRepo{updateFn: func(context.Context, *model.User) error { return dbErr }},
			wantErr: true, wantErrIs: dbErr, wantUpdates: 1,
		},
		{
			name: "insert db error propagates",
			user: model.NewUserBuilder().Name("d").Build(),
			repo: &fakeRepo{insertFn: func(context.Context, *model.User) error { return dbErr }},
			wantErr: true, wantErrIs: dbErr, wantInserts: 1,
		},
		{
			name:    "nil user errors",
			user:    nil,
			repo:    &fakeRepo{},
			wantErr: true,
		},
		{
			name:        "no validation: blank name saved",
			user:        model.NewEmptyUser(),
			repo:        &fakeRepo{},
			wantID:      100,
			wantInserts: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewUserService(tt.repo)
			got, err := svc.SaveUser(context.Background(), tt.user)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("err = %v, want wrapping %v", err, tt.wantErrIs)
				}
				if errors.Is(err, ErrUserNotFound) {
					t.Fatalf("save error must not be not-found")
				}
				if got != nil {
					t.Fatalf("expected nil user on error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.user {
					t.Fatalf("expected the same entity returned")
				}
				if got.GetID() != tt.wantID {
					t.Fatalf("id = %d, want %d", got.GetID(), tt.wantID)
				}
			}
			if tt.repo.insertCalls != tt.wantInserts || tt.repo.updateCalls != tt.wantUpdates {
				t.Fatalf("inserts=%d updates=%d, want %d/%d", tt.repo.insertCalls, tt.repo.updateCalls, tt.wantInserts, tt.wantUpdates)
			}
		})
	}
}

func TestFetchUserList(t *testing.T) {
	dbErr := errors.New("db down")
	users := []*model.User{
		model.NewUserBuilder().ID(1).Name("a").Build(),
		model.NewUserBuilder().ID(2).Name("b").Build(),
	}
	tests := []struct {
		name    string
		fn      func(context.Context) ([]*model.User, error)
		wantLen int
		wantErr bool
	}{
		{"users present", func(context.Context) ([]*model.User, error) { return users, nil }, 2, false},
		{"empty slice", func(context.Context) ([]*model.User, error) { return []*model.User{}, nil }, 0, false},
		{"nil from repo becomes empty", func(context.Context) ([]*model.User, error) { return nil, nil }, 0, false},
		{"repo error", func(context.Context) ([]*model.User, error) { return nil, dbErr }, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{findAllFn: tt.fn}
			got, err := NewUserService(repo).FetchUserList(context.Background())
			if tt.wantErr {
				if !errors.Is(err, dbErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if got == nil {
				t.Fatalf("slice must not be nil")
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d want %d", len(got), tt.wantLen)
			}
			if repo.findAllCalls != 1 || repo.insertCalls+repo.updateCalls+repo.deleteCalls != 0 {
				t.Fatalf("expected read-only single FindAll")
			}
		})
	}
}

func TestFetchUserByID(t *testing.T) {
	dbErr := errors.New("db down")
	u := model.NewUserBuilder().ID(3).Name("hemraj").Build()
	tests := []struct {
		name         string
		fn           func(context.Context, int32) (*model.User, bool, error)
		wantUser     *model.User
		wantNotFound bool
		wantErrIs    error
	}{
		{"found", func(context.Context, int32) (*model.User, bool, error) { return u, true, nil }, u, false, nil},
		{"not found", func(context.Context, int32) (*model.User, bool, error) { return nil, false, nil }, nil, true, nil},
		{"found flag but nil user", func(context.Context, int32) (*model.User, bool, error) { return nil, true, nil }, nil, true, nil},
		{"repo error", func(context.Context, int32) (*model.User, bool, error) { return nil, false, dbErr }, nil, false, dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{findByIDFn: tt.fn}
			got, err := NewUserService(repo).FetchUserByID(context.Background(), 3)
			if repo.lastFindID != 3 {
				t.Fatalf("id passed = %d", repo.lastFindID)
			}
			switch {
			case tt.wantNotFound:
				if !errors.Is(err, ErrUserNotFound) {
					t.Fatalf("want not found, got %v", err)
				}
				var nf *UserNotFoundError
				if !errors.As(err, &nf) {
					t.Fatalf("want *UserNotFoundError")
				}
				if err.Error() != "User are not available" {
					t.Fatalf("msg = %q", err.Error())
				}
				if got != nil {
					t.Fatalf("want nil user")
				}
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) || errors.Is(err, ErrUserNotFound) {
					t.Fatalf("err = %v", err)
				}
			default:
				if err != nil || got != tt.wantUser {
					t.Fatalf("got %v, %v", got, err)
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	dbErr := errors.New("db down")
	tests := []struct {
		name         string
		fn           func(context.Context, int32) error
		wantErr      bool
		wantNoEntity bool
		wantErrIs    error
	}{
		{"exists", nil, false, false, nil},
		{"missing", func(context.Context, int32) error { return repository.ErrUserNotExist }, true, true, nil},
		{"wrapped missing", func(context.Context, int32) error {
			return errors.Join(errors.New("ctx"), repository.ErrUserNotExist)
		}, true, true, nil},
		{"db error", func(context.Context, int32) error { return dbErr }, true, false, dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{deleteFn: tt.fn}
			err := NewUserService(repo).DeleteUser(context.Background(), 9)
			if repo.deleteCalls != 1 || repo.lastDeleteID != 9 {
				t.Fatalf("delete calls=%d id=%d", repo.deleteCalls, repo.lastDeleteID)
			}
			if repo.findByIDCalls != 0 {
				t.Fatalf("no service-level not-found check expected")
			}
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error")
			}
			if errors.Is(err, ErrUserNotFound) {
				t.Fatalf("delete must not produce not-found error")
			}
			if tt.wantNoEntity && !errors.Is(err, errNoEntity) {
				t.Fatalf("want errNoEntity, got %v", err)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("want %v, got %v", tt.wantErrIs, err)
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	dbErr := errors.New("db down")
	tests := []struct {
		name        string
		id          int
		user        *model.User
		repo        *fakeRepo
		wantErr     bool
		wantFinalID int32
		wantInserts int
	}{
		{
			name: "absent id forced", id: 4,
			user: model.NewUserBuilder().Name("x").Build(),
			repo: &fakeRepo{}, wantFinalID: 4,
		},
		{
			name: "different id overridden", id: 4,
			user: model.NewUserBuilder().ID(99).Name("x").Build(),
			repo: &fakeRepo{}, wantFinalID: 4,
		},
		{
			name: "null fields overwrite too", id: 4,
			user: model.NewUser(1, sp("x"), nil, nil, nil, nil),
			repo: &fakeRepo{updateFn: func(_ context.Context, u *model.User) error {
				if u.GetEmail() != nil || u.GetAbout() != nil {
					return errors.New("nil fields not passed through")
				}
				return nil
			}},
			wantFinalID: 4,
		},
		{
			name: "missing record merges to insert, no not-found", id: 50,
			user: model.NewUserBuilder().Name("y").Build(),
			repo: &fakeRepo{updateFn: func(context.Context, *model.User) error { return repository.ErrUserNotExist }},
			wantFinalID: 100, wantInserts: 1,
		},
		{
			name: "db error", id: 4,
			user:    model.NewUserBuilder().Name("z").Build(),
			repo:    &fakeRepo{updateFn: func(context.Context, *model.User) error { return dbErr }},
			wantErr: true,
		},
		{name: "nil user", id: 4, user: nil, repo: &fakeRepo{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewUserService(tt.repo).UpdateUser(context.Background(), tt.id, tt.user)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if errors.Is(err, ErrUserNotFound) {
					t.Fatalf("unexpected not-found")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if got != tt.user {
				t.Fatalf("expected mutated input returned")
			}
			if len(tt.repo.updatedIDs) != 1 || int(tt.repo.updatedIDs[0]) != tt.id {
				t.Fatalf("update called with ids %v, want [%d]", tt.repo.updatedIDs, tt.id)
			}
			if got.GetID() != tt.wantFinalID {
				t.Fatalf("final id = %d want %d", got.GetID(), tt.wantFinalID)
			}
			if tt.repo.insertCalls != tt.wantInserts {
				t.Fatalf("inserts = %d", tt.repo.insertCalls)
			}
		})
	}
}

func TestGetUserNameByName(t *testing.T) {
	hemraj := model.NewUserBuilder().ID(3).Name("hemraj").Email("hemrajmalhi1234@gmail.com").
		About("Sr").Role("java developer").Build()
	tests := []struct {
		name      string
		query     string
		fn        func(context.Context, string) (*model.User, bool, error)
		wantFound bool
		wantErrIs error
	}{
		{"found", "hemraj", func(context.Context, string) (*model.User, bool, error) { return hemraj, true, nil }, true, nil},
		{"not found", "nobody", func(context.Context, string) (*model.User, bool, error) { return nil, false, nil }, false, nil},
		{"found flag nil user", "x", func(context.Context, string) (*model.User, bool, error) { return nil, true, nil }, false, nil},
		{"non unique", "dup", func(context.Context, string) (*model.User, bool, error) {
			return nil, false, repository.ErrNonUniqueResult
		}, false, repository.ErrNonUniqueResult},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{findByNameFn: tt.fn}
			got, found, err := NewUserService(repo).GetUserNameByName(context.Background(), tt.query)
			if repo.lastName != tt.query || repo.findByNameCalls != 1 {
				t.Fatalf("FindByName not called correctly")
			}
			if repo.insertCalls+repo.updateCalls+repo.deleteCalls != 0 {
				t.Fatalf("must be read-only")
			}
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) || errors.Is(err, ErrUserNotFound) {
					t.Fatalf("err = %v", err)
				}
				if got != nil || found {
					t.Fatalf("expected no user on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v", found)
			}
			if !tt.wantFound {
				if got != nil {
					t.Fatalf("expected nil user")
				}
				return
			}
			if got.GetName() == nil || *got.GetName() != tt.query {
				t.Fatalf("returned name mismatch")
			}
			if !got.Equal(hemraj) {
				t.Fatalf("got %v", got)
			}
		})
	}
}