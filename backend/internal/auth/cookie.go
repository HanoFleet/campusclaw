package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func NewSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func Seal(secret, sessionID string) string {
	return sessionID + "." + hex.EncodeToString(sign(secret, sessionID))
}

func Open(secret, cookie string) (string, bool) {
	sessionID, sigHex, ok := strings.Cut(cookie, ".")
	if !ok || sessionID == "" || sigHex == "" {
		return "", false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return "", false
	}
	if !hmac.Equal(sig, sign(secret, sessionID)) {
		return "", false
	}
	return sessionID, true
}

func sign(secret, sessionID string) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte(sessionID))
	return m.Sum(nil)
}
