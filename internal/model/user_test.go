package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func sp(s string) *string { return &s }

func TestNewEmptyUser(t *testing.T) {
	u := NewEmptyUser()
	if u == nil {
		t.Fatal("nil user")
	}
	if u.ID != 0 || u.Name != nil || u.Email != nil || u.Password != nil || u.Role != nil || u.About != nil {
		t.Fatalf("expected zero user, got %v", u)
	}
	if got := u.String(); got != "User(id=0, name=null, email=null, password=null, role=null, about=null)" {
		t.Fatalf("String = %q", got)
	}
}

func TestNewUser(t *testing.T) {
	tests := []struct {
		name                         string
		id                           int32
		n, e, p, r, a                *string
		wantValidateErr              error
	}{
		{"all set", 1, sp("alice"), sp("a@x.com"), sp("pw"), sp("admin"), sp("hi"), nil},
		{"nil name", 2, nil, sp("b@x.com"), nil, nil, nil, ErrUserNameBlank},
		{"empty name", 3, sp(""), nil, nil, nil, nil, ErrUserNameBlank},
		{"blank name", 4, sp(" \t\n "), nil, nil, nil, nil, ErrUserNameBlank},
		{"all nil", 0, nil, nil, nil, nil, nil, ErrUserNameBlank},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := NewUser(tc.id, tc.n, tc.e, tc.p, tc.r, tc.a)
			if u == nil {
				t.Fatal("constructor returned nil")
			}
			if u.GetID() != tc.id || u.GetName() != tc.n || u.GetEmail() != tc.e ||
				u.GetPassword() != tc.p || u.GetRole() != tc.r || u.GetAbout() != tc.a {
				t.Fatalf("getters mismatch: %v", u)
			}
			err := u.Validate()
			if !errors.Is(err, tc.wantValidateErr) {
				t.Fatalf("Validate = %v, want %v", err, tc.wantValidateErr)
			}
			if err != nil && err.Error() != "please Add the department Name" {
				t.Fatalf("message = %q", err.Error())
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		val     *string
		wantErr bool
	}{
		{"nil", nil, true},
		{"empty", sp(""), true},
		{"spaces", sp("   "), true},
		{"unicode space", sp("\u00a0\u2003"), true},
		{"valid", sp("bob"), false},
		{"padded valid", sp("  bob  "), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := &User{Name: tc.val}
			err := u.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate err=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr && err != ErrUserNameBlank {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
}

func TestBuilder(t *testing.T) {
	tests := []struct {
		name  string
		build func(b *UserBuilder) *UserBuilder
		want  *User
	}{
		{"empty", func(b *UserBuilder) *UserBuilder { return b }, &User{}},
		{"subset", func(b *UserBuilder) *UserBuilder { return b.Name("n").Email("e") },
			&User{Name: sp("n"), Email: sp("e")}},
		{"all", func(b *UserBuilder) *UserBuilder {
			return b.ID(7).Name("n").Email("e").Password("p").Role("r").About("a")
		}, &User{ID: 7, Name: sp("n"), Email: sp("e"), Password: sp("p"), Role: sp("r"), About: sp("a")}},
		{"id only", func(b *UserBuilder) *UserBuilder { return b.ID(3) }, &User{ID: 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build(NewUserBuilder()).Build()
			if !got.Equal(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBuilderReturnsNewInstance(t *testing.T) {
	b := NewUserBuilder().ID(1).Name("x")
	u1 := b.Build()
	u2 := b.Build()
	if u1 == u2 {
		t.Fatal("Build returned same pointer")
	}
	if !u1.Equal(u2) {
		t.Fatal("builds should be equal")
	}
	u1.SetID(99)
	*u1.Name = "changed"
	if u2.ID != 1 {
		t.Fatal("mutating first build affected second")
	}
	b.Name("y")
	if *u2.Name == "y" {
		t.Fatal("builder mutation affected built user")
	}
}

func TestGettersSetters(t *testing.T) {
	tests := []struct {
		name string
		set  func(u *User, v *string)
		get  func(u *User) *string
	}{
		{"name", (*User).SetName, (*User).GetName},
		{"email", (*User).SetEmail, (*User).GetEmail},
		{"password", (*User).SetPassword, (*User).GetPassword},
		{"role", (*User).SetRole, (*User).GetRole},
		{"about", (*User).SetAbout, (*User).GetAbout},
	}
	values := []*string{sp("value"), sp(""), sp("  spaced  "), nil, sp(strings.Repeat("x", UserAboutMaxLength+1))}
	for _, tc := range tests {
		for _, v := range values {
			t.Run(tc.name, func(t *testing.T) {
				u := NewEmptyUser()
				tc.set(u, v)
				if got := tc.get(u); got != v {
					t.Fatalf("got %v want %v", got, v)
				}
			})
		}
	}
	for _, id := range []int32{0, 1, -5, 2147483647} {
		u := NewEmptyUser()
		u.SetID(id)
		if u.GetID() != id {
			t.Fatalf("id got %d want %d", u.GetID(), id)
		}
	}
}

func full() *User {
	return NewUser(1, sp("n"), sp("e"), sp("p"), sp("r"), sp("a"))
}

func TestEqualAndHash(t *testing.T) {
	tests := []struct {
		name  string
		other func() *User
		want  bool
	}{
		{"identical", full, true},
		{"diff id", func() *User { u := full(); u.ID = 2; return u }, false},
		{"diff name", func() *User { u := full(); u.Name = sp("x"); return u }, false},
		{"diff email", func() *User { u := full(); u.Email = sp("x"); return u }, false},
		{"diff password", func() *User { u := full(); u.Password = sp("x"); return u }, false},
		{"diff role", func() *User { u := full(); u.Role = sp("x"); return u }, false},
		{"diff about", func() *User { u := full(); u.About = sp("x"); return u }, false},
		{"nil name", func() *User { u := full(); u.Name = nil; return u }, false},
		{"nil about", func() *User { u := full(); u.About = nil; return u }, false},
		{"nil user", func() *User { return nil }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, b := full(), tc.other()
			if got := a.Equal(b); got != tc.want {
				t.Fatalf("a.Equal(b)=%v want %v", got, tc.want)
			}
			if got := b.Equal(a); got != tc.want {
				t.Fatalf("symmetry broken")
			}
			if tc.want && a.Hash() != b.Hash() {
				t.Fatal("equal users with different hashes")
			}
		})
	}
}

func TestEqualReflexiveAndNil(t *testing.T) {
	u := full()
	if !u.Equal(u) {
		t.Fatal("not reflexive")
	}
	var n1, n2 *User
	if !n1.Equal(n2) {
		t.Fatal("two nils should be equal")
	}
	if n1.Hash() != 0 {
		t.Fatal("nil hash should be 0")
	}
	if !NewEmptyUser().Equal(&User{}) {
		t.Fatal("empty users should be equal")
	}
}

func TestHashJavaParity(t *testing.T) {
	// Lombok: result=1; result=result*59+id; each null -> *59+43
	empty := int32(1)
	empty = empty*59 + 0
	for i := 0; i < 5; i++ {
		empty = empty*59 + 43
	}
	if got := NewEmptyUser().Hash(); got != empty {
		t.Fatalf("empty hash %d want %d", got, empty)
	}
	tests := []struct {
		s    string
		want int32
	}{
		{"", 0},
		{"a", 97},
		{"hello", 99162322},
		{"Aa", 2112},
		{"BB", 2112},
		{"😀", 1772899}, // surrogate pair 0xD83D,0xDE00
	}
	for _, tc := range tests {
		t.Run(tc.s, func(t *testing.T) {
			if got := javaStringHash(tc.s); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
	u := &User{ID: 1, Name: sp("a")}
	want := int32(1)
	want = want*59 + 1
	want = want*59 + 97
	for i := 0; i < 4; i++ {
		want = want*59 + 43
	}
	if u.Hash() != want {
		t.Fatalf("hash %d want %d", u.Hash(), want)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name string
		u    *User
		want string
	}{
		{"nil", nil, "null"},
		{"partial", &User{ID: 1, Name: sp("a")}, "User(id=1, name=a, email=null, password=null, role=null, about=null)"},
		{"full", full(), "User(id=1, name=n, email=e, password=p, role=r, about=a)"},
		{"negative", &User{ID: -3}, "User(id=-3, name=null, email=null, password=null, role=null, about=null)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.u.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	u := &User{ID: 5, Name: sp("n"), Password: sp("p")}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":5,"name":"n","email":null,"password":"p","role":null,"about":null}`
	if string(b) != want {
		t.Fatalf("got %s want %s", b, want)
	}
	var back User
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Equal(u) {
		t.Fatalf("round trip mismatch: %v", &back)
	}
}

func TestSchemaConstants(t *testing.T) {
	tests := []struct{ got, want string }{
		{UserTable, "user"},
		{UserColumnID, "user_id"},
		{UserColumnName, "user_name"},
		{UserColumnEmail, "user_email"},
		{UserColumnPassword, "user_password"},
		{UserColumnRole, "user_role"},
		{UserColumnAbout, "user_about"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("got %q want %q", tc.got, tc.want)
		}
	}
	if UserAboutMaxLength != 500 {
		t.Errorf("UserAboutMaxLength = %d", UserAboutMaxLength)
	}
}