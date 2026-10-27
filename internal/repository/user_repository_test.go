package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"testing"

	"migrated-app/internal/model"
)

// ---------------------------------------------------------------------------
// In-memory fake database/sql driver
// ---------------------------------------------------------------------------

type fakeRow struct {
	id   int64
	vals [5]driver.Value // name, email, password, role, about (string or nil)
}

type fakeStore struct {
	mu         sync.Mutex
	users      map[int64]fakeRow
	seq        *int64
	failOn     map[string]error
	execs      []string
	commits    int
	rollbacks  int
	snapUsers  map[int64]fakeRow
	snapSeq    *int64
	failBegin  error
	failCommit error
}

var (
	registerOnce sync.Once
	storesMu     sync.Mutex
	stores       = map[string]*fakeStore{}
	storeCounter int
)

type fakeDriver struct{}

func (fakeDriver) Open(name string) (driver.Conn, error) {
	storesMu.Lock()
	defer storesMu.Unlock()
	s, ok := stores[name]
	if !ok {
		return nil, fmt.Errorf("unknown store %q", name)
	}
	return &fakeConn{s: s}, nil
}

type fakeConn struct{ s *fakeStore }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("prepare unsupported") }
func (c *fakeConn) Close() error                         { return nil }

func copyUsers(m map[int64]fakeRow) map[int64]fakeRow {
	out := make(map[int64]fakeRow, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (c *fakeConn) Begin() (driver.Tx, error) {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failBegin != nil {
		return nil, s.failBegin
	}
	s.snapUsers = copyUsers(s.users)
	if s.seq != nil {
		v := *s.seq
		s.snapSeq = &v
	} else {
		s.snapSeq = nil
	}
	return &fakeTx{s: s}, nil
}

type fakeTx struct{ s *fakeStore }

func (t *fakeTx) Commit() error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if t.s.failCommit != nil {
		return t.s.failCommit
	}
	t.s.commits++
	return nil
}

func (t *fakeTx) Rollback() error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.s.rollbacks++
	t.s.users = t.s.snapUsers
	t.s.seq = t.s.snapSeq
	return nil
}

type fakeRows struct {
	cols []string
	data [][]driver.Value
	i    int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.i])
	r.i++
	return nil
}

func argVals(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

func userValues(r fakeRow) []driver.Value {
	return []driver.Value{r.id, r.vals[0], r.vals[1], r.vals[2], r.vals[3], r.vals[4]}
}

var userCols = []string{"user_id", "user_name", "user_email", "user_password", "user_role", "user_about"}

func (s *fakeStore) sortedIDs() []int64 {
	ids := make([]int64, 0, len(s.users))
	for id := range s.users {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (c *fakeConn) QueryContext(_ context.Context, query string, nargs []driver.NamedValue) (driver.Rows, error) {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.failOn[query]; err != nil {
		return nil, err
	}
	args := argVals(nargs)
	switch query {
	case sqlSelectAll:
		rows := &fakeRows{cols: userCols}
		for _, id := range s.sortedIDs() {
			rows.data = append(rows.data, userValues(s.users[id]))
		}
		return rows, nil
	case sqlSelectByID:
		rows := &fakeRows{cols: userCols}
		if u, ok := s.users[args[0].(int64)]; ok {
			rows.data = append(rows.data, userValues(u))
		}
		return rows, nil
	case sqlSelectByName:
		rows := &fakeRows{cols: userCols}
		for _, id := range s.sortedIDs() {
			u := s.users[id]
			if n, ok := u.vals[0].(string); ok && n == args[0].(string) {
				rows.data = append(rows.data, userValues(u))
				if len(rows.data) == 2 {
					break
				}
			}
		}
		return rows, nil
	case sqlLockByID:
		rows := &fakeRows{cols: []string{"user_id"}}
		if _, ok := s.users[args[0].(int64)]; ok {
			rows.data = append(rows.data, []driver.Value{args[0]})
		}
		return rows, nil
	case sqlCount:
		return &fakeRows{cols: []string{"c"}, data: [][]driver.Value{{int64(len(s.users))}}}, nil
	case sqlExistsByID:
		_, ok := s.users[args[0].(int64)]
		v := int64(0)
		if ok {
			v = 1
		}
		return &fakeRows{cols: []string{"e"}, data: [][]driver.Value{{v}}}, nil
	case sqlSeqSelectLock:
		rows := &fakeRows{cols: []string{"next_val"}}
		if s.seq != nil {
			rows.data = append(rows.data, []driver.Value{*s.seq})
		}
		return rows, nil
	}
	return nil, fmt.Errorf("unexpected query %q", query)
}

func (c *fakeConn) ExecContext(_ context.Context, query string, nargs []driver.NamedValue) (driver.Result, error) {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.failOn[query]; err != nil {
		return nil, err
	}
	s.execs = append(s.execs, query)
	args := argVals(nargs)
	switch query {
	case sqlInsert:
		id := args[0].(int64)
		if _, ok := s.users[id]; ok {
			return nil, errors.New("duplicate key")
		}
		s.users[id] = fakeRow{id: id, vals: [5]driver.Value{args[1], args[2], args[3], args[4], args[5]}}
		return driver.RowsAffected(1), nil
	case sqlUpdate:
		id := args[5].(int64)
		if _, ok := s.users[id]; !ok {
			return driver.RowsAffected(0), nil
		}
		s.users[id] = fakeRow{id: id, vals: [5]driver.Value{args[0], args[1], args[2], args[3], args[4]}}
		return driver.RowsAffected(1), nil
	case sqlDeleteByID:
		id := args[0].(int64)
		if _, ok := s.users[id]; !ok {
			return driver.RowsAffected(0), nil
		}
		delete(s.users, id)
		return driver.RowsAffected(1), nil
	case sqlSeqInitialise:
		v := args[0].(int64)
		s.seq = &v
		return driver.RowsAffected(1), nil
	case sqlSeqIncrement:
		if s.seq == nil || *s.seq != args[1].(int64) {
			return driver.RowsAffected(0), nil
		}
		v := args[0].(int64)
		s.seq = &v
		return driver.RowsAffected(1), nil
	}
	return nil, fmt.Errorf("unexpected exec %q", query)
}

var (
	_ driver.QueryerContext = (*fakeConn)(nil)
	_ driver.ExecerContext  = (*fakeConn)(nil)
)

func newFakeRepo(t *testing.T) (*MySQLUserRepository, *fakeStore) {
	t.Helper()
	registerOnce.Do(func() { sql.Register("fakeuserdb", fakeDriver{}) })
	s := &fakeStore{users: map[int64]fakeRow{}, failOn: map[string]error{}}
	storesMu.Lock()
	storeCounter++
	name := fmt.Sprintf("store-%d", storeCounter)
	stores[name] = s
	storesMu.Unlock()
	db, err := sql.Open("fakeuserdb", name)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return NewMySQLUserRepository(db), s
}

func sp(s string) *string { return &s }

func seed(s *fakeStore, id int64, name, email, password, role, about any) {
	conv := func(v any) driver.Value {
		if v == nil {
			return nil
		}
		return v.(string)
	}
	s.users[id] = fakeRow{id: id, vals: [5]driver.Value{conv(name), conv(email), conv(password), conv(role), conv(about)}}
}

func setSeq(s *fakeStore, v int64) { s.seq = &v }

func hemraj() *model.User {
	return model.NewUser(3, sp("hemraj"), sp("hemrajmalhi1234@gmail.com"), sp("root"), sp("java developer"), sp("Sr"))
}

func seedHemraj(s *fakeStore) {
	seed(s, 3, "hemraj", "hemrajmalhi1234@gmail.com", "root", "java developer", "Sr")
}

var errBoom = errors.New("boom")

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestFindByName(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(s *fakeStore)
		query     string
		want      *model.User
		wantFound bool
		wantErr   error
		anyErr    bool
	}{
		{name: "existing user hemraj", setup: seedHemraj, query: "hemraj", want: hemraj(), wantFound: true},
		{name: "no such user", setup: seedHemraj, query: "nobody"},
		{name: "case sensitive", setup: seedHemraj, query: "Hemraj"},
		{name: "empty store", setup: func(*fakeStore) {}, query: "hemraj"},
		{
			name: "non unique",
			setup: func(s *fakeStore) {
				seedHemraj(s)
				seed(s, 4, "hemraj", nil, nil, nil, nil)
			},
			query: "hemraj", wantErr: ErrNonUniqueResult,
		},
		{
			name:   "query error",
			setup:  func(s *fakeStore) { s.failOn[sqlSelectByName] = errBoom },
			query:  "hemraj",
			anyErr: true, wantErr: errBoom,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			tc.setup(s)
			before := len(s.users)
			got, found, err := repo.FindByName(context.Background(), tc.query)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != nil || found {
					t.Fatalf("expected nil,false on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if got != nil && *got.Name != tc.query {
				t.Fatalf("returned name %q != %q", *got.Name, tc.query)
			}
			if len(s.execs) != 0 || len(s.users) != before {
				t.Fatalf("FindByName must not write: execs=%v", s.execs)
			}
		})
	}
}

func TestSave(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *fakeStore)
		input   *model.User
		wantID  int32
		wantErr error
		anyErr  bool
		count   int64
	}{
		{
			name:   "new user gets id from sequence",
			setup:  func(s *fakeStore) { setSeq(s, 10) },
			input:  model.NewUserBuilder().Name("a").Email("a@x").Build(),
			wantID: 10, count: 1,
		},
		{
			name:   "uninitialised sequence starts at 1",
			setup:  func(*fakeStore) {},
			input:  model.NewUserBuilder().Name("a").Build(),
			wantID: 1, count: 1,
		},
		{
			name:   "existing id is updated",
			setup:  func(s *fakeStore) { seedHemraj(s); setSeq(s, 50) },
			input:  model.NewUser(3, sp("hemraj"), sp("new@x"), nil, sp("lead"), nil),
			wantID: 3, count: 1,
		},
		{
			name:   "unknown non-zero id inserted with generated id",
			setup:  func(s *fakeStore) { seedHemraj(s); setSeq(s, 7) },
			input:  model.NewUserBuilder().ID(99).Name("z").Build(),
			wantID: 7, count: 2,
		},
		{name: "nil user", setup: func(*fakeStore) {}, input: nil, anyErr: true},
		{
			name:   "lock failure rolls back",
			setup:  func(s *fakeStore) { seedHemraj(s); s.failOn[sqlLockByID] = errBoom },
			input:  hemraj(),
			wantErr: errBoom, count: 1,
		},
		{
			name:   "insert failure rolls back sequence",
			setup:  func(s *fakeStore) { setSeq(s, 5); s.failOn[sqlInsert] = errBoom },
			input:  model.NewUserBuilder().Name("a").Build(),
			wantErr: errBoom, count: 0,
		},
		{
			name:   "sequence overflow",
			setup:  func(s *fakeStore) { setSeq(s, int64(1)<<31) },
			input:  model.NewUserBuilder().Name("a").Build(),
			anyErr: true, count: 0,
		},
		{
			name:   "begin failure",
			setup:  func(s *fakeStore) { s.failBegin = errBoom },
			input:  model.NewUserBuilder().Name("a").Build(),
			wantErr: errBoom,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			tc.setup(s)
			var origID int32
			if tc.input != nil {
				origID = tc.input.ID
			}
			got, err := repo.Save(context.Background(), tc.input)
			if tc.wantErr != nil || tc.anyErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatal("expected nil user on error")
				}
				s.failBegin = nil
				if tc.input != nil {
					if n, _ := repo.Count(context.Background()); n != tc.count {
						t.Fatalf("count = %d, want %d", n, tc.count)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got.ID != tc.wantID {
				t.Fatalf("id = %d, want %d", got.ID, tc.wantID)
			}
			if tc.input.ID != origID {
				t.Fatal("Save must not modify input")
			}
			found, ok, err := repo.FindByID(context.Background(), got.ID)
			if err != nil || !ok || !found.Equal(got) {
				t.Fatalf("FindByID after save = %v,%v,%v; want %v", found, ok, err, got)
			}
			if n, _ := repo.Count(context.Background()); n != tc.count {
				t.Fatalf("count = %d, want %d", n, tc.count)
			}
		})
	}
}

func TestInsertAndUpdate(t *testing.T) {
	t.Run("insert nil", func(t *testing.T) {
		repo, _ := newFakeRepo(t)
		if err := repo.Insert(context.Background(), nil); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("insert assigns id", func(t *testing.T) {
		repo, s := newFakeRepo(t)
		setSeq(s, 4)
		u := model.NewUserBuilder().Name("a").Build()
		if err := repo.Insert(context.Background(), u); err != nil {
			t.Fatal(err)
		}
		if u.ID != 4 || *s.seq != 5 || s.commits != 1 {
			t.Fatalf("id=%d seq=%d commits=%d", u.ID, *s.seq, s.commits)
		}
	})
	t.Run("insert commit failure", func(t *testing.T) {
		repo, s := newFakeRepo(t)
		s.failCommit = errBoom
		if err := repo.Insert(context.Background(), model.NewEmptyUser()); !errors.Is(err, errBoom) {
			t.Fatalf("err = %v", err)
		}
	})

	tests := []struct {
		name    string
		input   *model.User
		wantErr error
		anyErr  bool
	}{
		{name: "nil", input: nil, anyErr: true},
		{name: "missing", input: model.NewUserBuilder().ID(42).Name("x").Build(), wantErr: ErrUserNotExist},
		{name: "nil fields become NULL", input: model.NewUser(3, sp("hemraj"), nil, nil, nil, nil)},
	}
	for _, tc := range tests {
		t.Run("update "+tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			seedHemraj(s)
			err := repo.Update(context.Background(), tc.input)
			if tc.wantErr != nil || tc.anyErr {
				if err == nil || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, _, _ := repo.FindByID(context.Background(), tc.input.ID)
			if !got.Equal(tc.input) {
				t.Fatalf("got %v want %v", got, tc.input)
			}
		})
	}
}

func TestFindByID(t *testing.T) {
	tests := []struct {
		name      string
		id        int32
		fail      bool
		want      *model.User
		wantFound bool
	}{
		{name: "exists", id: 3, want: hemraj(), wantFound: true},
		{name: "missing", id: 77},
		{name: "error", id: 3, fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			seedHemraj(s)
			if tc.fail {
				s.failOn[sqlSelectByID] = errBoom
			}
			got, found, err := repo.FindByID(context.Background(), tc.id)
			if tc.fail {
				if !errors.Is(err, errBoom) || found || got != nil {
					t.Fatalf("got %v,%v,%v", got, found, err)
				}
				return
			}
			if err != nil || found != tc.wantFound || !got.Equal(tc.want) {
				t.Fatalf("got %v,%v,%v", got, found, err)
			}
			if len(s.execs) != 0 {
				t.Fatal("read-only violated")
			}
		})
	}
}

func TestFindAll(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *fakeStore)
		wantIDs []int32
		wantErr bool
	}{
		{name: "empty", setup: func(*fakeStore) {}, wantIDs: []int32{}},
		{
			name: "ordered by id",
			setup: func(s *fakeStore) {
				seed(s, 5, "e", nil, nil, nil, nil)
				seedHemraj(s)
				seed(s, 1, "a", nil, nil, nil, nil)
			},
			wantIDs: []int32{1, 3, 5},
		},
		{name: "error", setup: func(s *fakeStore) { s.failOn[sqlSelectAll] = errBoom }, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			tc.setup(s)
			got, err := repo.FindAll(context.Background())
			if tc.wantErr {
				if !errors.Is(err, errBoom) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("FindAll returned nil slice")
			}
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.wantIDs))
			}
			for i, id := range tc.wantIDs {
				if got[i].ID != id {
					t.Fatalf("got[%d].ID = %d, want %d", i, got[i].ID, id)
				}
			}
		})
	}
}

func TestDeleteByID(t *testing.T) {
	tests := []struct {
		name    string
		id      int32
		fail    bool
		wantErr error
	}{
		{name: "exists", id: 3},
		{name: "missing", id: 9, wantErr: ErrUserNotExist},
		{name: "exec error", id: 3, fail: true, wantErr: errBoom},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			seedHemraj(s)
			if tc.fail {
				s.failOn[sqlDeleteByID] = errBoom
			}
			err := repo.DeleteByID(context.Background(), tc.id)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, found, _ := repo.FindByID(context.Background(), tc.id); found {
				t.Fatal("user still present after delete")
			}
		})
	}
}

func TestCountAndExistsByID(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(s *fakeStore)
		id         int32
		wantCount  int64
		wantExists bool
		countErr   bool
		existsErr  bool
	}{
		{name: "empty", setup: func(*fakeStore) {}, id: 3},
		{
			name:  "two users existing id",
			setup: func(s *fakeStore) { seedHemraj(s); seed(s, 4, "b", nil, nil, nil, nil) },
			id:    3, wantCount: 2, wantExists: true,
		},
		{name: "missing id", setup: seedHemraj, id: 8, wantCount: 1},
		{
			name: "errors",
			setup: func(s *fakeStore) {
				s.failOn[sqlCount] = errBoom
				s.failOn[sqlExistsByID] = errBoom
			},
			countErr: true, existsErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, s := newFakeRepo(t)
			tc.setup(s)
			n, err := repo.Count(context.Background())
			if tc.countErr {
				if !errors.Is(err, errBoom) {
					t.Fatalf("count err = %v", err)
				}
			} else if err != nil || n != tc.wantCount {
				t.Fatalf("count = %d,%v want %d", n, err, tc.wantCount)
			}
			ex, err := repo.ExistsByID(context.Background(), tc.id)
			if tc.existsErr {
				if !errors.Is(err, errBoom) {
					t.Fatalf("exists err = %v", err)
				}
			} else if err != nil || ex != tc.wantExists {
				t.Fatalf("exists = %v,%v want %v", ex, err, tc.wantExists)
			}
			if len(s.execs) != 0 {
				t.Fatal("read-only violated")
			}
		})
	}
}

func TestNullStringPtr(t *testing.T) {
	tests := []struct {
		name string
		in   sql.NullString
		want *string
	}{
		{name: "null", in: sql.NullString{}},
		{name: "empty valid", in: sql.NullString{Valid: true}, want: sp("")},
		{name: "value", in: sql.NullString{String: "x", Valid: true}, want: sp("x")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nullStringPtr(tc.in)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}