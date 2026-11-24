package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const tokenTTL = 24 * time.Hour

type claims struct {
	TokenVersion int `json:"tv"`
	jwt.RegisteredClaims
}

type Tokens struct {
	secret []byte
	now    func() time.Time
}

func NewTokens(secret []byte) *Tokens { return &Tokens{secret: secret, now: time.Now} }

func (t *Tokens) Issue(u *User) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(tokenTTL)
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		TokenVersion: u.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.Hex(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}).SignedString(t.secret)
	return tok, exp, err
}

// Parse accepts only HS256, so a token re-signed with "none" or another algorithm fails.
func (t *Tokens) Parse(raw string) (bson.ObjectID, int, error) {
	var c claims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(t.now))
	if err != nil {
		return bson.ObjectID{}, 0, err
	}
	id, err := bson.ObjectIDFromHex(c.Subject)
	if err != nil {
		return bson.ObjectID{}, 0, errors.New("bad subject")
	}
	return id, c.TokenVersion, nil
}
