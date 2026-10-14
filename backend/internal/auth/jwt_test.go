package auth

import (
	"strings"
	"testing"
	"time"
)

func TestJWTRoundTrip(t *testing.T) {
	tok, err := IssueJWT("0123456789abcdef", "sess-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "eyJ") {
		t.Fatalf("not a JWT: %s", tok)
	}
	sid, ok := ParseJWT("0123456789abcdef", tok)
	if !ok || sid != "sess-1" {
		t.Fatalf("got %q %v", sid, ok)
	}
	if _, ok := ParseJWT("wrong-secret-xxxx", tok); ok {
		t.Fatal("wrong secret accepted")
	}
}

func TestJWTRejectsExpired(t *testing.T) {
	tok, err := IssueJWT("0123456789abcdef", "sess-1", -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ParseJWT("0123456789abcdef", tok); ok {
		t.Fatal("expired token accepted")
	}
}
