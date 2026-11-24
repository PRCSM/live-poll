package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSignupValidation(t *testing.T) {
	cases := map[string]struct {
		in        signupInput
		badFields []string
	}{
		"valid":             {signupInput{"  Me@Example.COM ", "Paik", "password1"}, nil},
		"bad email":         {signupInput{"not-an-email", "Paik", "password1"}, []string{"email"}},
		"display name":      {signupInput{"Paik <me@example.com>", "Paik", "password1"}, []string{"email"}},
		"empty name":        {signupInput{"me@example.com", " ​ ", "password1"}, []string{"name"}},
		"short password":    {signupInput{"me@example.com", "Paik", "short"}, []string{"password"}},
		"72+ byte password": {signupInput{"me@example.com", "Paik", strings.Repeat("é", 37)}, []string{"password"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fields := tc.in.validate()
			if len(fields) != len(tc.badFields) {
				t.Fatalf("fields = %v, want %v", fields, tc.badFields)
			}
			for _, f := range tc.badFields {
				if _, ok := fields[f]; !ok {
					t.Errorf("missing error for %s", f)
				}
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  Me@Example.COM "); got != "me@example.com" {
		t.Errorf("got %q", got)
	}
}

func TestTokens(t *testing.T) {
	tk := NewTokens([]byte(strings.Repeat("s", 32)))
	u := &User{ID: bson.NewObjectID(), TokenVersion: 3}

	raw, _, err := tk.Issue(u)
	if err != nil {
		t.Fatal(err)
	}
	id, tv, err := tk.Parse(raw)
	if err != nil || id != u.ID || tv != 3 {
		t.Fatalf("round trip: id=%v tv=%d err=%v", id, tv, err)
	}

	if _, _, err := NewTokens([]byte(strings.Repeat("x", 32))).Parse(raw); err == nil {
		t.Error("token signed with another secret was accepted")
	}

	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": u.ID.Hex(), "exp": time.Now().Add(time.Hour).Unix()}).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, _, err := tk.Parse(none); err == nil {
		t.Error("alg=none token was accepted")
	}

	tk.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	if _, _, err := tk.Parse(raw); err == nil {
		t.Error("expired token was accepted")
	}
}
