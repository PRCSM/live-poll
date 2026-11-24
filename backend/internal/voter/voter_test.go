package voter

import (
	"strings"
	"testing"
	"time"
)

func TestVoterToken(t *testing.T) {
	s := NewSigner([]byte(strings.Repeat("k", 32)))
	tok := s.NewToken()

	id, ok := s.ParseToken(tok)
	if !ok || id == "" {
		t.Fatalf("own token rejected: %q", tok)
	}
	if s.NewToken() == tok {
		t.Error("tokens should be unique")
	}

	forged := id + ".AAAAAAAAAAAAAAAAAAAAAA"
	for _, bad := range []string{"", "nodot", forged, tok + "x", "a.b"} {
		if _, ok := s.ParseToken(bad); ok {
			t.Errorf("accepted bad token %q", bad)
		}
	}
	if _, ok := NewSigner([]byte(strings.Repeat("z", 32))).ParseToken(tok); ok {
		t.Error("token accepted under a different secret")
	}
	if s.Key(id) == s.UserKey(id) {
		t.Error("anonymous and account keys must not collide")
	}
}

func TestStreamToken(t *testing.T) {
	s := NewSigner([]byte(strings.Repeat("k", 32)))
	tok := s.StreamToken("poll1")
	if !s.ValidStreamToken("poll1", tok) {
		t.Fatal("valid stream token rejected")
	}
	if s.ValidStreamToken("poll2", tok) {
		t.Error("token for poll1 accepted for poll2")
	}
	s.now = func() time.Time { return time.Now().Add(StreamTokenTTL + time.Minute) }
	if s.ValidStreamToken("poll1", tok) {
		t.Error("expired stream token accepted")
	}
}
