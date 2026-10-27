// Package repository implements persistence for the application's domain
// types on top of database/sql and MySQL.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"migrated-app/internal/model"
)

// ErrUserNotExist is returned by Update and DeleteByID when no row has the
// requested id. It mirrors Spring Data's EmptyResultDataAccessException for
// deleteById (which the original application surfaced as an HTTP 500).
var ErrUserNotExist = errors.New("repository: no user with the given id exists")

// ErrNonUniqueResult is returned by FindByName when more than one user has
// the requested name. It mirrors Spring Data's
// IncorrectResultSizeDataAccessException for single-result derived queries.
var ErrNonUniqueResult = errors.New("repository: query did not return a unique result")

// UserRepository is the persistence contract for model.User, replacing the
// Spring Data JPA UserDao (JpaRepository<User, Integer> + findByName).
type UserRepository interface {
	// Insert persists a new user, assigning it an id from hibernate_sequence.
	// The assigned id is written back into u.
	Insert(ctx context.Context, u *model.User) error
	// Update overwrites every column of the existing row with u's values
	// (nil fields become NULL). Returns ErrUserNotExist if no row has u.ID.
	Update(ctx context.Context, u *model.User) error
	// Save reproduces JpaRepository.save: a zero id (or an id with no row)
	// is inserted with a freshly generated id; otherwise the row is updated.
	Save(ctx context.Context, u *model.User) (*model.User, error)
	// FindAll returns every user ordered by id. Never returns a nil slice.
	FindAll(ctx context.Context) ([]*model.User, error)
	// FindByID looks up a user by id. The bool is false when none exists.
	FindByID(ctx context.Context, id int32) (*model.User, bool, error)
	// FindByName looks up the single user whose name exactly equals name.
	// The bool is false when none exists; ErrNonUniqueResult is returned
	// when several users share the name.
	FindByName(ctx context.Context, name string) (*model.User, bool, error)
	// DeleteByID removes the user with the given id, returning
	// ErrUserNotExist when there is none.
	DeleteByID(ctx context.Context, id int32) error
	// Count returns the total number of users.
	Count(ctx context.Context) (int64, error)
	// ExistsByID reports whether a user with the given id exists.
	ExistsByID(ctx context.Context, id int32) (bool, error)
}

// queryer is the subset of *sql.DB / *sql.Tx used by the scan helpers.
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// MySQLUserRepository is the MySQL-backed UserRepository.
type MySQLUserRepository struct {
	db *sql.DB
}

// Compile-time interface check.
var _ UserRepository = (*MySQLUserRepository)(nil)

// NewMySQLUserRepository returns a UserRepository backed by db.
func NewMySQLUserRepository(db *sql.DB) *MySQLUserRepository {
	return &MySQLUserRepository{db: db}
}

// SQL statements. `user` is back-quoted because USER is a MySQL keyword.
const (
	userSelectColumns = "user_id, user_name, user_email, user_password, user_role, user_about"

	sqlSelectAll      = "SELECT " + userSelectColumns + " FROM `user` ORDER BY user_id"
	sqlSelectByID     = "SELECT " + userSelectColumns + " FROM `user` WHERE user_id = ?"
	sqlSelectByName   = "SELECT " + userSelectColumns + " FROM `user` WHERE user_name = ? LIMIT 2"
	sqlLockByID       = "SELECT user_id FROM `user` WHERE user_id = ? FOR UPDATE"
	sqlInsert         = "INSERT INTO `user` (" + userSelectColumns + ") VALUES (?, ?, ?, ?, ?, ?)"
	sqlUpdate         = "UPDATE `user` SET user_name = ?, user_email = ?, user_password = ?, user_role = ?, user_about = ? WHERE user_id = ?"
	sqlDeleteByID     = "DELETE FROM `user` WHERE user_id = ?"
	sqlCount          = "SELECT COUNT(*) FROM `user`"
	sqlExistsByID     = "SELECT EXISTS(SELECT 1 FROM `user` WHERE user_id = ?)"
	sqlSeqSelectLock  = "SELECT next_val FROM hibernate_sequence FOR UPDATE"
	sqlSeqIncrement   = "UPDATE hibernate_sequence SET next_val = ? WHERE next_val = ?"
	sqlSeqInitialise  = "INSERT INTO hibernate_sequence (next_val) VALUES (?)"
	hibernateSeqStart = int64(1)
)

// Insert implements UserRepository.
func (r *MySQLUserRepository) Insert(ctx context.Context, u *model.User) error {
	if u == nil {
		return errors.New("repository: insert nil user")
	}
	return r.withTx(ctx, func(tx *sql.Tx) error {
		return insertTx(ctx, tx, u)
	})
}

// Update implements UserRepository.
func (r *MySQLUserRepository) Update(ctx context.Context, u *model.User) error {
	if u == nil {
		return errors.New("repository: update nil user")
	}
	return r.withTx(ctx, func(tx *sql.Tx) error {
		exists, err := lockByID(ctx, tx, u.ID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrUserNotExist
		}
		return updateTx(ctx, tx, u)
	})
}

// Save implements UserRepository.
//
// MIGRATION_NOTE: Hibernate's merge() on a detached entity whose id has no
// row inserts it under a newly generated id (GenerationType.AUTO ignores the
// supplied id). That behaviour is reproduced here. The returned user is a
// copy of u carrying the persisted id; u itself is not modified.
func (r *MySQLUserRepository) Save(ctx context.Context, u *model.User) (*model.User, error) {
	if u == nil {
		return nil, errors.New("repository: save nil user")
	}
	saved := *u
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		if saved.ID != 0 {
			exists, err := lockByID(ctx, tx, saved.ID)
			if err != nil {
				return err
			}
			if exists {
				return updateTx(ctx, tx, &saved)
			}
		}
		return insertTx(ctx, tx, &saved)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// FindAll implements UserRepository.
func (r *MySQLUserRepository) FindAll(ctx context.Context) ([]*model.User, error) {
	rows, err := r.db.QueryContext(ctx, sqlSelectAll)
	if err != nil {
		return nil, fmt.Errorf("repository: find all users: %w", err)
	}
	defer rows.Close()

	users := make([]*model.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("repository: find all users: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repository: find all users: %w", err)
	}
	return users, nil
}

// FindByID implements UserRepository.
func (r *MySQLUserRepository) FindByID(ctx context.Context, id int32) (*model.User, bool, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, sqlSelectByID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("repository: find user %d: %w", id, err)
	}
	return u, true, nil
}

// FindByName implements UserRepository.
func (r *MySQLUserRepository) FindByName(ctx context.Context, name string) (*model.User, bool, error) {
	rows, err := r.db.QueryContext(ctx, sqlSelectByName, name)
	if err != nil {
		return nil, false, fmt.Errorf("repository: find user by name: %w", err)
	}
	defer rows.Close()

	var found *model.User
	for rows.Next() {
		if found != nil {
			return nil, false, ErrNonUniqueResult
		}
		u, err := scanUser(rows)
		if err != nil {
			return nil, false, fmt.Errorf("repository: find user by name: %w", err)
		}
		found = u
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("repository: find user by name: %w", err)
	}
	return found, found != nil, nil
}

// DeleteByID implements UserRepository.
func (r *MySQLUserRepository) DeleteByID(ctx context.Context, id int32) error {
	res, err := r.db.ExecContext(ctx, sqlDeleteByID, id)
	if err != nil {
		return fmt.Errorf("repository: delete user %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repository: delete user %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("repository: delete user %d: %w", id, ErrUserNotExist)
	}
	return nil
}

// Count implements UserRepository.
func (r *MySQLUserRepository) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, sqlCount).Scan(&n); err != nil {
		return 0, fmt.Errorf("repository: count users: %w", err)
	}
	return n, nil
}

// ExistsByID implements UserRepository.
func (r *MySQLUserRepository) ExistsByID(ctx context.Context, id int32) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, sqlExistsByID, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("repository: exists user %d: %w", id, err)
	}
	return exists, nil
}

// withTx runs fn inside a transaction, committing on success and rolling
// back on error.
func (r *MySQLUserRepository) withTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("repository: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("repository: commit tx: %w", err)
	}
	return nil
}

// nextSequenceValue reserves the next id from hibernate_sequence, the table
// Hibernate 5 uses for GenerationType.AUTO on MySQL.
func nextSequenceValue(ctx context.Context, q queryer) (int64, error) {
	var next int64
	err := q.QueryRowContext(ctx, sqlSeqSelectLock).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		// Hibernate seeds the table with initial_value = 1.
		if _, err := q.ExecContext(ctx, sqlSeqInitialise, hibernateSeqStart); err != nil {
			return 0, fmt.Errorf("repository: initialise hibernate_sequence: %w", err)
		}
		next = hibernateSeqStart
	} else if err != nil {
		return 0, fmt.Errorf("repository: read hibernate_sequence: %w", err)
	}
	if _, err := q.ExecContext(ctx, sqlSeqIncrement, next+1, next); err != nil {
		return 0, fmt.Errorf("repository: advance hibernate_sequence: %w", err)
	}
	return next, nil
}

func insertTx(ctx context.Context, tx *sql.Tx, u *model.User) error {
	id, err := nextSequenceValue(ctx, tx)
	if err != nil {
		return err
	}
	if id > int64(^uint32(0)>>1) {
		return fmt.Errorf("repository: hibernate_sequence value %d overflows Integer id", id)
	}
	if _, err := tx.ExecContext(ctx, sqlInsert,
		int32(id), u.Name, u.Email, u.Password, u.Role, u.About); err != nil {
		return fmt.Errorf("repository: insert user: %w", err)
	}
	u.ID = int32(id)
	return nil
}

func updateTx(ctx context.Context, tx *sql.Tx, u *model.User) error {
	if _, err := tx.ExecContext(ctx, sqlUpdate,
		u.Name, u.Email, u.Password, u.Role, u.About, u.ID); err != nil {
		return fmt.Errorf("repository: update user %d: %w", u.ID, err)
	}
	return nil
}

// lockByID reports whether a row with id exists, taking a row lock on it so
// a concurrent delete cannot interleave before the subsequent write.
func lockByID(ctx context.Context, tx *sql.Tx, id int32) (bool, error) {
	var got int32
	err := tx.QueryRowContext(ctx, sqlLockByID, id).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("repository: lock user %d: %w", id, err)
	}
	return true, nil
}

// rowScanner is satisfied by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(s rowScanner) (*model.User, error) {
	var (
		id                                 int32
		name, email, password, role, about sql.NullString
	)
	if err := s.Scan(&id, &name, &email, &password, &role, &about); err != nil {
		return nil, err
	}
	return model.NewUser(id,
		nullStringPtr(name), nullStringPtr(email), nullStringPtr(password),
		nullStringPtr(role), nullStringPtr(about)), nil
}

func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}
