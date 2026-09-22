package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"campusclaw/internal/auth"
	"campusclaw/internal/config"
)

const cookieName = "campusclaw_session"

type User struct {
	ID        int64
	Username  string
	Role      string
	ClassID   int64
	ClassName string
}

type Server struct {
	cfg     config.Config
	db      *sql.DB
	limiter *auth.Limiter
	mux     *http.ServeMux
}

func New(cfg config.Config, db *sql.DB) *Server {
	s := &Server{
		cfg:     cfg,
		db:      db,
		limiter: auth.NewLimiter(),
		mux:     http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("POST /api/login", s.login)
	s.mux.HandleFunc("POST /api/logout", s.logout)
	s.mux.HandleFunc("GET /api/me", s.me)
	s.mux.HandleFunc("GET /api/materials", s.listMaterials)
	s.mux.HandleFunc("POST /api/materials", s.uploadMaterial)
	s.mux.HandleFunc("GET /api/materials/{id}", s.getMaterial)
	s.mux.HandleFunc("GET /api/materials/{id}/file", s.getMaterialFile)
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) currentUser(r *http.Request) (*User, error) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil, errNoSession
	}
	sessionID, ok := auth.Open(s.cfg.SessionSecret, c.Value)
	if !ok {
		return nil, errNoSession
	}
	var u User
	var expires time.Time
	err = s.db.QueryRowContext(r.Context(), `SELECT u.id, u.username, u.role, u.class_id, c.name, s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN classes c ON c.id = u.class_id
		WHERE s.id = ?`, sessionID).Scan(&u.ID, &u.Username, &u.Role, &u.ClassID, &u.ClassName, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoSession
	}
	if err != nil {
		return nil, err
	}
	if !expires.After(time.Now().UTC()) {
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE id=?`, sessionID)
		return nil, errNoSession
	}
	return &u, nil
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (*User, bool) {
	if !s.databaseUp(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return nil, false
	}
	u, err := s.currentUser(r)
	if err == nil {
		return u, true
	}
	if errors.Is(err, errNoSession) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	writeError(w, http.StatusServiceUnavailable, "unavailable")
	return nil, false
}

func (s *Server) setSessionCookie(w http.ResponseWriter, sealed string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sealed,
		Path:     "/",
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) databaseUp(ctx context.Context) bool {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.db.PingContext(pingCtx) == nil
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

var errNoSession = errors.New("no session")
