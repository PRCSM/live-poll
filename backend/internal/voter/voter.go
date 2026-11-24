// Package voter issues and checks the signed tokens that identify anonymous voters
// and unlock hidden results on the live stream. Nothing here uses cookies: the
// frontend and API live on different sites, where browsers block third-party cookies.
package voter

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const StreamTokenTTL = 24 * time.Hour

type Signer struct {
	secret []byte
	now    func() time.Time
}

func NewSigner(secret []byte) *Signer { return &Signer{secret: secret, now: time.Now} }

func (s *Signer) mac(parts ...string) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(strings.Join(parts, "|")))
	return m.Sum(nil)
}

var b64 = base64.RawURLEncoding

// NewToken returns a fresh voter token: a random id plus its signature, so a client
// cannot pick an id that collides with someone else's.
func (s *Signer) NewToken() string {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	idStr := b64.EncodeToString(id)
	return idStr + "." + b64.EncodeToString(s.mac("vt", idStr)[:16])
}

// ParseToken returns the voter id if the token was issued by this server.
func (s *Signer) ParseToken(tok string) (string, bool) {
	id, sig, ok := strings.Cut(tok, ".")
	if !ok || len(id) != 22 {
		return "", false
	}
	got, err := b64.DecodeString(sig)
	if err != nil || !hmac.Equal(got, s.mac("vt", id)[:16]) {
		return "", false
	}
	return id, true
}

// Key is what gets stored for dedupe. It is derived, so the raw token never sits in the database.
func (s *Signer) Key(voterID string) string { return hex.EncodeToString(s.mac("vk", voterID)[:16]) }

// UserKey dedupes by account when a poll requires login.
func (s *Signer) UserKey(userID string) string {
	return hex.EncodeToString(s.mac("vk-user", userID)[:16])
}

// IPHash lets abuse be investigated without storing raw IP addresses.
func (s *Signer) IPHash(ip string) string { return hex.EncodeToString(s.mac("ip", ip)[:8]) }

// StreamToken lets a viewer who is allowed to see results open a stream that includes them.
// EventSource cannot send headers, so this travels as a query parameter.
func (s *Signer) StreamToken(pollID string) string {
	exp := strconv.FormatInt(s.now().Add(StreamTokenTTL).Unix(), 10)
	return exp + "." + b64.EncodeToString(s.mac("st", pollID, exp)[:16])
}

func (s *Signer) ValidStreamToken(pollID, tok string) bool {
	exp, sig, ok := strings.Cut(tok, ".")
	if !ok {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || s.now().Unix() >= unix {
		return false
	}
	got, err := b64.DecodeString(sig)
	return err == nil && hmac.Equal(got, s.mac("st", pollID, exp)[:16])
}
