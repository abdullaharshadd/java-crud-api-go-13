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
// lookup by name. HTTP handlers depend on this interface.
type UserService interface {
	// SaveUser persists u and returns the saved instance, including the
	// generated id.
	SaveUser(ctx context.Context, u *model.User) (*model.User, error)
	// FetchUserList returns all stored users. The slice is never nil.
	FetchUserList(ctx context.Context) ([]*model.User, error)
	// FetchUserByID returns the user with the given id, or an error
	// satisfying errors.Is(err, ErrUserNotFound) when none exists.
	FetchUserByID(ctx context.Context, id int32) (*model.User, error)
	// DeleteUser removes the user with the given id. A missing id yields an
	// internal (non-404) error, mirroring the original behaviour.
	DeleteUser(ctx context.Context, id int32) error
	// UpdateUser forces u's id to id and saves it with upsert (JPA merge)
	// semantics. u is mutated to hold the persisted id.
	UpdateUser(ctx context.Context, id int32, u *model.User) error
	// GetUserNameByName returns the single user whose name equals name, or
	// (nil, nil) when no user matches. More than one match is an error.
	GetUserNameByName(ctx context.Context, name string) (*model.User, error)
}

// userService is the default UserService backed by a UserRepository.
type userService struct {
	repo repository.UserRepository
}

// NewUserService returns a UserService that uses repo for persistence.
func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

// SaveUser applies JPA save/merge semantics.
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
// call ran in its own transaction; update-then-insert is not atomic.
func (s *userService) save(ctx context.Context, u *model.User) error {
	if u.GetID() != 0 {
		err := s.repo.Update(ctx, u)
		if err == nil {
			return nil
		}
		if !errors.Is(err, repository.ErrUserNotExist) {
			return fmt.Errorf("service: update user %d: %w", u.GetID(), err)
		}
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

// FetchUserByID looks up a user by id.
func (s *userService) FetchUserByID(ctx context.Context, id int32) (*model.User, error) {
	u, found, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service: fetch user %d: %w", id, err)
	}
	if !found || u == nil {
		return nil, NewUserNotFoundErrorMsg(userNotAvailableMsg)
	}
	return u, nil
}

// DeleteUser deletes the user with the given id.
func (s *userService) DeleteUser(ctx context.Context, id int32) error {
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
// semantics as SaveUser.
func (s *userService) UpdateUser(ctx context.Context, id int32, u *model.User) error {
	if u == nil {
		return errors.New("service: update user: nil user")
	}
	u.SetID(id)
	return s.save(ctx, u)
}

// GetUserNameByName returns the user whose name equals name, or (nil, nil)
// when none matches.
func (s *userService) GetUserNameByName(ctx context.Context, name string) (*model.User, error) {
	u, found, err := s.repo.FindByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("service: get user by name %q: %w", name, err)
	}
	if !found || u == nil {
		return nil, nil
	}
	return u, nil
}