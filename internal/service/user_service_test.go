package service

import (
	"context"
	"errors"
	"testing"

	"migrated-app/internal/model"
	"migrated-app/internal/repository"
)

type fakeRepo struct {
	users  map[int32]*model.User
	nextID int32
	err    error
	nilAll bool
	saves  []*model.User
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[int32]*model.User{}, nextID: 1} }

func cp(u *model.User) *model.User { c := *u; return &c }

func (f *fakeRepo) Insert(ctx context.Context, u *model.User) error {
	if f.err != nil {
		return f.err
	}
	u.ID = f.nextID
	f.nextID++
	f.users[u.ID] = cp(u)
	return nil
}
func (f *fakeRepo) Update(ctx context.Context, u *model.User) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.users[u.ID]; !ok {
		return repository.ErrUserNotExist
	}
	f.users[u.ID] = cp(u)
	return nil
}
func (f *fakeRepo) Save(ctx context.Context, u *model.User) (*model.User, error) {
	f.saves = append(f.saves, cp(u))
	if f.err != nil {
		return nil, f.err
	}
	s := cp(u)
	if _, ok := f.users[s.ID]; s.ID != 0 && ok {
		f.users[s.ID] = cp(s)
		return s, nil
	}
	s.ID = f.nextID
	f.nextID++
	f.users[s.ID] = cp(s)
	return s, nil
}
func (f *fakeRepo) FindAll(ctx context.Context) ([]*model.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.nilAll {
		return nil, nil
	}
	out := make([]*model.User, 0)
	for i := int32(0); i < f.nextID; i++ {
		if u, ok := f.users[i]; ok {
			out = append(out, cp(u))
		}
	}
	return out, nil
}
func (f *fakeRepo) FindByID(ctx context.Context, id int32) (*model.User, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return nil, false, nil
	}
	return cp(u), true, nil
}
func (f *fakeRepo) FindByName(ctx context.Context, name string) (*model.User, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	var found *model.User
	for _, u := range f.users {
		if u.Name != nil && *u.Name == name {
			if found != nil {
				return nil, false, repository.ErrNonUniqueResult
			}
			found = cp(u)
		}
	}
	return found, found != nil, nil
}
func (f *fakeRepo) DeleteByID(ctx context.Context, id int32) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.users[id]; !ok {
		return repository.ErrUserNotExist
	}
	delete(f.users, id)
	return nil
}
func (f *fakeRepo) Count(ctx context.Context) (int64, error) { return int64(len(f.users)), f.err }
func (f *fakeRepo) ExistsByID(ctx context.Context, id int32) (bool, error) {
	_, ok := f.users[id]
	return ok, f.err
}

var errDB = errors.New("db failure")

func user(id int32, name string) *model.User {
	return model.NewUserBuilder().ID(id).Name(name).Email(name + "@x.com").Build()
}

func TestSaveUser(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*fakeRepo)
		in      *model.User
		wantID  int32
		wantErr error
		anyErr  bool
	}{
		{name: "new user gets id", in: user(0, "alice"), wantID: 1},
		{name: "existing id overwritten", setup: func(f *fakeRepo) { f.users[1] = user(1, "old"); f.nextID = 2 }, in: user(1, "new"), wantID: 1},
		{name: "repo error propagates", setup: func(f *fakeRepo) { f.err = errDB }, in: user(0, "x"), wantErr: errDB},
		{name: "nil user", in: nil, anyErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRepo()
			if tt.setup != nil {
				tt.setup(f)
			}
			s := NewUserService(f)
			got, err := s.SaveUser(context.Background(), tt.in)
			if tt.wantErr != nil || tt.anyErr {
				if err == nil || got != nil {
					t.Fatalf("expected error, got %v %v", got, err)
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("err %v not %v", err, tt.wantErr)
				}
				if len(f.users) > 0 && tt.setup == nil {
					t.Fatal("record persisted on error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.wantID {
				t.Fatalf("id=%d want %d", got.ID, tt.wantID)
			}
			fetched, err := s.FetchUserByID(context.Background(), got.ID)
			if err != nil || !fetched.Equal(got) {
				t.Fatalf("fetch mismatch %v %v", fetched, err)
			}
		})
	}
}

func TestFetchUserList(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*fakeRepo)
		wantLen int
		wantErr bool
	}{
		{name: "users exist", setup: func(f *fakeRepo) { f.users[1] = user(1, "a"); f.users[2] = user(2, "b"); f.nextID = 3 }, wantLen: 2},
		{name: "empty store", wantLen: 0},
		{name: "repo returns nil slice", setup: func(f *fakeRepo) { f.nilAll = true }, wantLen: 0},
		{name: "repo error", setup: func(f *fakeRepo) { f.err = errDB }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRepo()
			if tt.setup != nil {
				tt.setup(f)
			}
			got, err := NewUserService(f).FetchUserList(context.Background())
			if tt.wantErr {
				if !errors.Is(err, errDB) {
					t.Fatalf("want errDB got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("nil slice")
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len=%d want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestFetchUserByID(t *testing.T) {
	tests := []struct {
		name     string
		id       int32
		repoErr  error
		notFound bool
	}{
		{name: "existing", id: 1},
		{name: "missing", id: 99, notFound: true},
		{name: "repo error", id: 1, repoErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRepo()
			f.users[1] = user(1, "a")
			f.err = tt.repoErr
			got, err := NewUserService(f).FetchUserByID(context.Background(), tt.id)
			switch {
			case tt.notFound:
				if got != nil || !errors.Is(err, ErrUserNotFound) {
					t.Fatalf("want not found, got %v %v", got, err)
				}
				var unf *UserNotFoundError
				if !errors.As(err, &unf) {
					t.Fatal("not UserNotFoundError")
				}
				if err.Error() != "User are not available" {
					t.Fatalf("msg %q", err.Error())
				}
			case tt.repoErr != nil:
				if got != nil || !errors.Is(err, tt.repoErr) || errors.Is(err, ErrUserNotFound) {
					t.Fatalf("got %v %v", got, err)
				}
			default:
				if err != nil || got == nil || !got.Equal(user(1, "a")) {
					t.Fatalf("got %v %v", got, err)
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name    string
		id      int32
		repoErr error
		wantErr error
	}{
		{name: "existing", id: 1},
		{name: "missing", id: 42, wantErr: repository.ErrUserNotExist},
		{name: "repo error", id: 1, repoErr: errDB, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRepo()
			f.users[1] = user(1, "a")
			f.nextID = 2
			f.err = tt.repoErr
			s := NewUserService(f)
			err := s.DeleteUser(context.Background(), tt.id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got %v want %v", err, tt.wantErr)
				}
				if len(f.users) != 1 {
					t.Fatal("records changed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.FetchUserByID(context.Background(), tt.id); !errors.Is(err, ErrUserNotFound) {
				t.Fatalf("after delete want not found, got %v", err)
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	tests := []struct {
		name      string
		id        int32
		in        *model.User
		repoErr   error
		wantErr   bool
		checkID   int32
		checkName string
		wantCount int
	}{
		{name: "existing id overrides arg id", id: 1, in: user(77, "updated"), checkID: 1, checkName: "updated", wantCount: 1},
		{name: "missing id inserts new", id: 50, in: user(0, "fresh"), checkID: 2, checkName: "fresh", wantCount: 2},
		{name: "repo error", id: 1, in: user(1, "x"), repoErr: errDB, wantErr: true, wantCount: 1},
		{name: "nil user", id: 1, in: nil, wantErr: true, wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRepo()
			f.users[1] = user(1, "orig")
			f.nextID = 2
			s := NewUserService(f)
			var origID int32
			if tt.in != nil {
				origID = tt.in.ID
			}
			f.err = tt.repoErr
			err := s.UpdateUser(context.Background(), tt.id, tt.in)
			if tt.in != nil && tt.in.ID != origID {
				t.Fatal("caller input mutated")
			}
			if tt.in != nil && len(f.saves) > 0 && f.saves[0].ID != tt.id {
				t.Fatalf("saved with id %d want %d", f.saves[0].ID, tt.id)
			}
			f.err = nil
			if len(f.users) != tt.wantCount {
				t.Fatalf("count=%d want %d", len(f.users), tt.wantCount)
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.repoErr != nil && !errors.Is(err, tt.repoErr) {
					t.Fatalf("got %v", err)
				}
				if *f.users[1].Name != "orig" {
					t.Fatal("record modified on error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.FetchUserByID(context.Background(), tt.checkID)
			if err != nil || *got.Name != tt.checkName {
				t.Fatalf("got %v %v", got, err)
			}
		})
	}
}

func TestGetUserNameByName(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		setup   func(*fakeRepo)
		wantNil bool
		wantErr error
	}{
		{name: "match", query: "alice"},
		{name: "no match", query: "nobody", wantNil: true},
		{name: "non unique", query: "alice", setup: func(f *fakeRepo) { f.users[3] = user(3, "alice") }, wantErr: repository.ErrNonUniqueResult},
		{name: "repo error", query: "alice", setup: func(f *fakeRepo) { f.err = errDB }, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRepo()
			f.users[1] = user(1, "alice")
			f.users[2] = user(2, "bob")
			f.nextID = 4
			if tt.setup != nil {
				tt.setup(f)
			}
			before := len(f.users)
			got, err := NewUserService(f).GetUserNameByName(context.Background(), tt.query)
			if len(f.users) != before {
				t.Fatal("state modified")
			}
			if tt.wantErr != nil {
				if got != nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("got %v %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("want nil got %v", got)
				}
				return
			}
			if got == nil || !got.Equal(user(1, "alice")) {
				t.Fatalf("got %v", got)
			}
		})
	}
}