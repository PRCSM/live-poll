package auth

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/PRCSM/live-poll/backend/internal/textx"
)

const (
	minPasswordBytes = 8
	maxPasswordBytes = 72 // bcrypt ignores everything past 72 bytes
	maxEmailLen      = 254
	maxNameRunes     = 60
)

// normalizeEmail makes "  Me@Example.COM " and "me@example.com" the same account.
func normalizeEmail(s string) string {
	return strings.ToLower(norm.NFC.String(strings.TrimSpace(s)))
}

func validEmail(email string) bool {
	if email == "" || len(email) > maxEmailLen {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && addr.Name == ""
}

type signupInput struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (in *signupInput) validate() map[string]string {
	in.Email = normalizeEmail(in.Email)
	in.Name = textx.Clean(in.Name)
	fields := map[string]string{}
	if !validEmail(in.Email) {
		fields["email"] = "Enter a valid email address."
	}
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > maxNameRunes {
		fields["name"] = "Name must be 1 to 60 characters."
	}
	if len(in.Password) < minPasswordBytes {
		fields["password"] = "Password must be at least 8 characters."
	} else if len(in.Password) > maxPasswordBytes {
		fields["password"] = "Password must be at most 72 bytes."
	}
	return fields
}
