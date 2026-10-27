package service

import (
	"context"
	"errors"
	"fmt"

	"migrated-app/internal/model"
	"migrated-app/internal/repository"
)

// UserService is the business-layer contract for User CRUD operations and
// lookup by name. HTTP handlers depend on this interface; NewUserService
// returns the default implementation (Java: UserService + UserServiceImp).
type UserService interface {
	// SaveUser persists u and returns the saved instance, including the
	// generated id (Java: saveUser -> UserDao.save).
	SaveUser(ctx context.Context, u *model.User) (*model.User, error)
	// FetchUserList returns all stored users. The slice is never nil.
	FetchUserList(ctx context.Context) ([]*model.User, error)
	// FetchUserByID returns the user with the given id, or an error
	// satisfying errors.Is(err, ErrUserNotFound) when none exists.
	FetchUserByID(ctx context.Context, id int32) (*model.User, error)
	// DeleteUser removes the user with the given id. Deleting a missing
	// user returns an error wrapping repository.ErrUserNotExist (the source
	// surfaced this as an HTTP 500, not a 404).
	DeleteUser(ctx context.Context, id int32) error
	// UpdateUser stores u under the given id using JPA merge semantics:
	// an existing row is overwritten; if no row has that id, the user is
	// inserted under a freshly generated id.
	UpdateUser(ctx context.Context, id int32, u *model.User) error
	// GetUserNameByName looks up the user whose name equals name. It
	// returns (nil, nil) when no such user exists, mirroring the source's
	// null return (rendered as an empty 200 by the HTTP layer).
	GetUserNameByName(ctx context.Context, name string) (*model.User, error)
}

// userService is the default UserService backed by a UserRepository.
type userService struct {
	repo repository.UserRepository
}

// Compile-time interface check.
var _ UserService = (*userService)(nil)

// NewUserService returns a UserService that delegates persistence to repo.
func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

// SaveUser implements UserService.
func (s *userService) SaveUser(ctx context.Context, u *model.User) (*model.User, error) {
	if u == nil {
		return nil, errors.New("service: save user: nil user")
	}
	saved, err := s.repo.Save(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("service: save user: %w", err)
	}
	return saved, nil
}

// FetchUserList implements UserService.
func (s *userService) FetchUserList(ctx context.Context) ([]*model.User, error) {
	users, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("service: fetch user list: %w", err)
	}
	if users == nil {
		users = make([]*model.User, 0)
	}
	return users, nil
}

// FetchUserByID implements UserService.
func (s *userService) FetchUserByID(ctx context.Context, id int32) (*model.User, error) {
	u, ok, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service: fetch user %d: %w", id, err)
	}
	if !ok {
		// Same message the source used: "User are not available".
		return nil, NewUserNotFoundErrorMsg(ErrUserNotFound.Error())
	}
	return u, nil
}

// DeleteUser implements UserService.
func (s *userService) DeleteUser(ctx context.Context, id int32) error {
	if err := s.repo.DeleteByID(ctx, id); err != nil {
		return fmt.Errorf("service: delete user %d: %w", id, err)
	}
	return nil
}

// UpdateUser implements UserService.
//
// MIGRATION_NOTE: the Java version mutated the caller's User via
// user.setId(id) before saving. Here the caller's value is copied so the
// input is not modified; the persisted result is the same.
func (s *userService) UpdateUser(ctx context.Context, id int32, u *model.User) error {
	if u == nil {
		return errors.New("service: update user: nil user")
	}
	toSave := *u
	toSave.ID = id
	if _, err := s.repo.Save(ctx, &toSave); err != nil {
		return fmt.Errorf("service: update user %d: %w", id, err)
	}
	return nil
}

// GetUserNameByName implements UserService.
func (s *userService) GetUserNameByName(ctx context.Context, name string) (*model.User, error) {
	u, ok, err := s.repo.FindByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("service: find user by name: %w", err)
	}
	if !ok {
		return nil, nil
	}
	return u, nil
}
