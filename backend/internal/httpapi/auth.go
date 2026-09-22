package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"campusclaw/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.databaseUp(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var req loginRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	key := auth.Key(req.Username, clientIP(r))
	now := time.Now().UTC()
	locked := s.limiter.Locked(key, now)

	var hash string
	var userID int64
	err := s.db.QueryRowContext(r.Context(), `SELECT id, password_hash FROM users WHERE username=?`, req.Username).Scan(&userID, &hash)
	if err != nil {
		if errors.Is(err, errNoRows()) {
			_ = auth.CheckPassword(string(auth.DummyHash()), req.Password)
			if !locked {
				s.limiter.Fail(key, now, s.cfg.LoginFailThreshold, s.cfg.LoginLock)
			}
			writeError(w, http.StatusUnauthorized, "invalid_credentials")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}

	if locked {
		_ = auth.CheckPassword(hash, req.Password)
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if auth.CheckPassword(hash, req.Password) != nil {
		s.limiter.Fail(key, now, s.cfg.LoginFailThreshold, s.cfg.LoginLock)
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	s.limiter.Reset(key)
	if c, cerr := r.Cookie(cookieName); cerr == nil {
		if oldID, ok := auth.Open(s.cfg.SessionSecret, c.Value); ok {
			_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE id=?`, oldID)
		}
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, userID); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	sessionID, err := auth.NewSessionID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	expires := now.Add(s.cfg.SessionTTL)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		sessionID, userID, expires, now); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	s.setSessionCookie(w, auth.Seal(s.cfg.SessionSecret, sessionID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	c, _ := r.Cookie(cookieName)
	if c != nil {
		if sessionID, opened := auth.Open(s.cfg.SessionSecret, c.Value); opened {
			_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE id=?`, sessionID)
		}
	}
	_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, u.ID)
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"role":       u.Role,
		"class_id":   u.ClassID,
		"class_name": u.ClassName,
	})
}

func errNoRows() error {
	return sqlErrNoRows
}
