// Package service contains the business layer of the application. It sits
// between the HTTP transport (internal/httpapi) and persistence
// (internal/repository).
package service

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/model"
	"migrated-app/internal/repository"
)

// userNotAvailableMsg is the exact message the Java service passed to
// UserNotFoundException. The 404 ErrorMessage body shows it to clients.
const userNotAvailableMsg = "User are not available"

// errNoEntity is returned by DeleteUser when no user has the requested id.
//
// MIGRATION_NOTE: Spring Data's deleteById threw
// EmptyResultDataAccessException for a missing id. The original
// application did not map that exception, so it produced an HTTP 500.
// This error deliberately does NOT satisfy errors.Is(err, ErrUserNotFound),
// so the HTTP layer keeps the 500 (not 404) quirk for parity.
var errNoEntity = errors.New("service: no user entity with the given id exists")

// UserService is the business-layer contract for User CRUD operations and
// lookup by name. HTTP handlers depend on this interface. NewUserService
// returns the default implementation (Java: UserService + UserServiceImp).
type UserService interface {
	// SaveUser persists u and returns the saved instance, including the
	// generated id (Java: saveUser -> UserDao.save).
	SaveUser(ctx context.Context, u *model.User) (*model.User, error)
	// FetchUserList returns all stored users. The slice is never nil.
	FetchUserList(ctx context.Context) ([]*model.User, error)
	// FetchUserByID returns the user with the given id, or an error
	// satisfying errors.Is(err, ErrUserNotFound) when none exists.
	FetchUserByID(ctx context.Context, id int) (*model.User, error)
	// DeleteUser removes the user with the given id. A missing id yields an
	// internal (non-404) error, mirroring the original behaviour.
	DeleteUser(ctx context.Context, id int) error
	// UpdateUser forces u's id to id and saves it with upsert (JPA merge)
	// semantics. It returns the mutated input.
	UpdateUser(ctx context.Context, id int, u *model.User) (*model.User, error)
	// GetUserNameByName returns the single user whose name equals name.
	// found is false when no user matches. More than one match is an error.
	GetUserNameByName(ctx context.Context, name string) (*model.User, bool, error)
}

// userService is the default UserService backed by a UserRepository.
type userService struct {
	repo repository.UserRepository
}

// NewUserService returns a UserService that uses repo for persistence.
// It replaces Spring's @Service component scanning and @Autowired field
// injection with explicit constructor injection.
func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

// SaveUser applies JPA save/merge semantics:
//   - id == 0: insert with a generated id;
//   - id exists: update the row;
//   - nonzero id that does not exist: insert with a newly generated id
//     (the supplied id is discarded, as Hibernate's merge does).
//
// Constraint violations, such as a duplicate email, return a wrapped
// database error. The HTTP layer maps that error to a 500.
func (s *userService) SaveUser(ctx context.Context, u *model.User) (*model.User, error) {
	if u == nil {
		return nil, errors.New("service: save user: nil user")
	}
	if err := s.save(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// save performs the merge logic and leaves u holding the persisted id.
//
// MIGRATION_NOTE: the Java service had no @Transactional. Each repository
// call ran in its own transaction. This port keeps that: update-then-insert
// is not atomic. Manual review: decide whether concurrent upserts need a
// single transaction.
func (s *userService) save(ctx context.Context, u *model.User) error {
	if u.GetID() != 0 {
		err := s.repo.Update(ctx, u)
		if err == nil {
			return nil
		}
		if !errors.Is(err, repository.ErrUserNotExist) {
			return fmt.Errorf("service: update user %d: %w", u.GetID(), err)
		}
		// The id is missing, so Hibernate's merge inserts a new row with a
		// generated id.
		u.SetID(0)
	}
	if err := s.repo.Insert(ctx, u); err != nil {
		return fmt.Errorf("service: insert user: %w", err)
	}
	return nil
}

// FetchUserList returns all users. The result is never nil.
func (s *userService) FetchUserList(ctx context.Context) ([]*model.User, error) {
	users, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: fetch user list: %w", err)
	}
	if users == nil {
		users = []*model.User{}
	}
	return users, nil
}

// FetchUserByID looks up a user by id. It returns a *UserNotFoundError
// (errors.Is(err, ErrUserNotFound)) with the message
// "User are not available" when the user does not exist.
func (s *userService) FetchUserByID(ctx context.Context, id int) (*model.User, error) {
	u, found, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service: fetch user %d: %w", id, err)
	}
	if !found || u == nil {
		return nil, NewUserNotFoundErrorMsg(userNotAvailableMsg)
	}
	return u, nil
}

// DeleteUser deletes the user with the given id. A missing id returns an
// error wrapping errNoEntity, which the HTTP layer maps to a 500.
func (s *userService) DeleteUser(ctx context.Context, id int) error {
	err := s.repo.DeleteByID(ctx, id)
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrUserNotExist) {
		return fmt.Errorf("service: delete user %d: %w", id, errNoEntity)
	}
	return fmt.Errorf("service: delete user %d: %w", id, err)
}

// UpdateUser forces u's id to id and saves it with the same upsert
// semantics as SaveUser. It returns the mutated input.
//
// MIGRATION_NOTE: Java's updateUser returned void and discarded the result
// of save. Returning the input lets callers see the final id. On a merge
// insert, that id can differ from the path id.
func (s *userService) UpdateUser(ctx context.Context, id int, u *model.User) (*model.User, error) {
	if u == nil {
		return nil, errors.New("service: update user: nil user")
	}
	u.SetID(id)
	if err := s.save(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserNameByName returns the user whose name equals name.
//   - No match: found is false and err is nil. The HTTP layer renders an
//     empty 200, matching Spring returning null.
//   - More than one match: an error wrapping repository.ErrNonUniqueResult,
//     mapped to a 500.
func (s *userService) GetUserNameByName(ctx context.Context, name string) (*model.User, bool, error) {
	u, found, err := s.repo.FindByName(ctx, name)
	if err != nil {
		return nil, false, fmt.Errorf("service: get user by name %q: %w", name, err)
	}
	if !found || u == nil {
		return nil, false, nil
	}
	return u, true, nil
}
