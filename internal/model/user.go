// Package model contains the domain types persisted by the application.
package model

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Physical schema names for the User entity. Hibernate's default physical
// naming strategy lower-cases the @Table/@Column names, so these mirror what
// Hibernate generated in MySQL. The DDL itself lives in
// internal/repository/schema.go.
const (
	UserTable          = "user"
	UserColumnID       = "user_id"
	UserColumnName     = "user_name"
	UserColumnEmail    = "user_email"
	UserColumnPassword = "user_password"
	UserColumnRole     = "user_role"
	UserColumnAbout    = "user_about"

	// UserAboutMaxLength is the declared column length of user_about
	// (@Column(length = 500)). Not validated in Go; the database enforces it.
	UserAboutMaxLength = 500
)

// ErrUserNameBlank is returned by Validate when the name is nil or blank.
// The message is kept verbatim from the source @NotBlank annotation.
var ErrUserNameBlank = errors.New("please Add the department Name")

// User is the persisted user entity.
//
// String fields are pointers so that a JSON null / SQL NULL round-trips
// faithfully, matching the nullable java.lang.String fields of the source.
//
// MIGRATION_NOTE: password is intentionally serialised in JSON responses for
// parity with the original Spring application. Review before production use.
type User struct {
	ID       int32   `json:"id"`
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Password *string `json:"password"`
	Role     *string `json:"role"`
	About    *string `json:"about"`
}

// NewEmptyUser returns a User with every field at its zero value
// (equivalent of the Lombok @NoArgsConstructor).
func NewEmptyUser() *User {
	return &User{}
}

// NewUser returns a User with all fields set, in the order id, name, email,
// password, role, about (equivalent of the Lombok @AllArgsConstructor).
func NewUser(id int32, name, email, password, role, about *string) *User {
	return &User{
		ID:       id,
		Name:     name,
		Email:    email,
		Password: password,
		Role:     role,
		About:    about,
	}
}

// Validate reproduces the @NotBlank constraint on name: it fails when the
// name is nil or consists solely of whitespace.
func (u *User) Validate() error {
	if u.Name == nil || strings.TrimFunc(*u.Name, unicode.IsSpace) == "" {
		return ErrUserNameBlank
	}
	return nil
}

// GetID returns the user id.
func (u *User) GetID() int32 { return u.ID }

// SetID sets the user id.
func (u *User) SetID(id int32) { u.ID = id }

// GetName returns the user name (nil when unset).
func (u *User) GetName() *string { return u.Name }

// SetName sets the user name.
func (u *User) SetName(name *string) { u.Name = name }

// GetEmail returns the user email (nil when unset).
func (u *User) GetEmail() *string { return u.Email }

// SetEmail sets the user email.
func (u *User) SetEmail(email *string) { u.Email = email }

// GetPassword returns the user password (nil when unset).
func (u *User) GetPassword() *string { return u.Password }

// SetPassword sets the user password.
func (u *User) SetPassword(password *string) { u.Password = password }

// GetRole returns the user role (nil when unset).
func (u *User) GetRole() *string { return u.Role }

// SetRole sets the user role.
func (u *User) SetRole(role *string) { u.Role = role }

// GetAbout returns the user "about" text (nil when unset).
func (u *User) GetAbout() *string { return u.About }

// SetAbout sets the user "about" text.
func (u *User) SetAbout(about *string) { u.About = about }

// Equal reports whether u and other hold the same values in every field
// (Lombok @Data equals semantics). Two nil users are equal.
func (u *User) Equal(other *User) bool {
	if u == nil || other == nil {
		return u == other
	}
	return u.ID == other.ID &&
		strPtrEqual(u.Name, other.Name) &&
		strPtrEqual(u.Email, other.Email) &&
		strPtrEqual(u.Password, other.Password) &&
		strPtrEqual(u.Role, other.Role) &&
		strPtrEqual(u.About, other.About)
}

// Hash returns a value-based hash over all fields, computed with the same
// algorithm Lombok generates for hashCode (prime 59, 43 for null, Java
// String.hashCode for strings), so values match the original application.
func (u *User) Hash() int32 {
	if u == nil {
		return 0
	}
	const prime int32 = 59
	result := int32(1)
	result = result*prime + u.ID
	for _, s := range []*string{u.Name, u.Email, u.Password, u.Role, u.About} {
		if s == nil {
			result = result*prime + 43
		} else {
			result = result*prime + javaStringHash(*s)
		}
	}
	return result
}

// String renders the user in the Lombok @Data toString format, e.g.
// "User(id=1, name=a, email=null, password=null, role=null, about=null)".
func (u *User) String() string {
	if u == nil {
		return "null"
	}
	var b strings.Builder
	b.WriteString("User(id=")
	b.WriteString(strconv.FormatInt(int64(u.ID), 10))
	b.WriteString(", name=")
	b.WriteString(strPtrString(u.Name))
	b.WriteString(", email=")
	b.WriteString(strPtrString(u.Email))
	b.WriteString(", password=")
	b.WriteString(strPtrString(u.Password))
	b.WriteString(", role=")
	b.WriteString(strPtrString(u.Role))
	b.WriteString(", about=")
	b.WriteString(strPtrString(u.About))
	b.WriteString(")")
	return b.String()
}

// UserBuilder is a fluent builder for User (equivalent of Lombok @Builder).
type UserBuilder struct {
	user User
}

// NewUserBuilder returns an empty UserBuilder.
func NewUserBuilder() *UserBuilder {
	return &UserBuilder{}
}

// ID sets the id on the user being built.
func (b *UserBuilder) ID(id int32) *UserBuilder {
	b.user.ID = id
	return b
}

// Name sets the name on the user being built.
func (b *UserBuilder) Name(name string) *UserBuilder {
	b.user.Name = &name
	return b
}

// Email sets the email on the user being built.
func (b *UserBuilder) Email(email string) *UserBuilder {
	b.user.Email = &email
	return b
}

// Password sets the password on the user being built.
func (b *UserBuilder) Password(password string) *UserBuilder {
	b.user.Password = &password
	return b
}

// Role sets the role on the user being built.
func (b *UserBuilder) Role(role string) *UserBuilder {
	b.user.Role = &role
	return b
}

// About sets the "about" text on the user being built.
func (b *UserBuilder) About(about string) *UserBuilder {
	b.user.About = &about
	return b
}

// Build returns a new User holding the builder's values. The builder may be
// reused; each call returns an independent copy.
func (b *UserBuilder) Build() *User {
	u := b.user
	return &u
}

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func strPtrString(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}

// javaStringHash reproduces java.lang.String#hashCode over UTF-16 code units
// with 32-bit wrap-around.
func javaStringHash(s string) int32 {
	var h int32
	for _, c := range utf16.Encode([]rune(s)) {
		h = 31*h + int32(c)
	}
	return h
}
