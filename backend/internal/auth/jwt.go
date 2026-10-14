package auth

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type jwtClaims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func IssueJWT(secret, sessionID string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(jwtClaims{
		Sub: sessionID,
		Iat: now.Unix(),
		Exp: now.Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	signing := b64(header) + "." + b64(payload)
	return signing + "." + b64(sign(secret, signing)), nil
}

func ParseJWT(secret, token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", false
	}
	signing := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", false
	}
	if !hmac.Equal(sig, sign(secret, signing)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims jwtClaims
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Sub == "" {
		return "", false
	}
	if claims.Exp <= time.Now().UTC().Unix() {
		return "", false
	}
	return claims.Sub, true
}

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
