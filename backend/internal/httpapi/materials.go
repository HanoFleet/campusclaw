package httpapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"campusclaw/internal/chunk"
	"campusclaw/internal/materials"
)

var storageNameRe = regexp.MustCompile(`^[a-f0-9]{32}\.(md|txt)$`)

type materialRow struct {
	ID          int64
	ClassID     int64
	Title       string
	CreatedAt   time.Time
	StorageName string
	Body        string
}

func (s *Server) listMaterials(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = s.db.QueryContext(r.Context(), `SELECT id, title, created_at FROM materials
			WHERE class_id=? ORDER BY created_at DESC, id DESC`, u.ClassID)
	} else {
		like := likePattern(q)
		rows, err = s.db.QueryContext(r.Context(), `SELECT m.id, m.title, m.created_at
			FROM materials m
			JOIN knowledge_entries k ON k.material_id = m.id
			WHERE m.class_id=? AND (m.title LIKE ? ESCAPE '\\' OR k.body LIKE ? ESCAPE '\\')
			ORDER BY m.created_at DESC, m.id DESC`, u.ClassID, like, like)
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	type item struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		CreatedAt string `json:"created_at"`
	}
	list := []item{}
	for rows.Next() {
		var it item
		var created time.Time
		if err := rows.Scan(&it.ID, &it.Title, &created); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		it.CreatedAt = created.UTC().Format(time.RFC3339)
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"materials": list})
}

func (s *Server) getMaterial(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	row, status := s.loadMaterial(r, u)
	if status != 0 {
		writeError(w, status, statusCode(status))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         row.ID,
		"title":      row.Title,
		"class_id":   row.ClassID,
		"created_at": row.CreatedAt.UTC().Format(time.RFC3339),
		"body":       row.Body,
	})
}

func (s *Server) getMaterialFile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	row, status := s.loadMaterial(r, u)
	if status != 0 {
		writeError(w, status, statusCode(status))
		return
	}
	full, err := s.safeStoragePath(row.StorageName)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	f, err := os.Open(full)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", contentDisposition(row.Title))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func (s *Server) loadMaterial(r *http.Request, u *User) (materialRow, int) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return materialRow{}, http.StatusNotFound
	}
	var row materialRow
	err = s.db.QueryRowContext(r.Context(), `SELECT m.id, m.class_id, m.title, m.created_at, m.storage_name, k.body
		FROM materials m
		JOIN knowledge_entries k ON k.material_id = m.id
		WHERE m.id=?`, id).Scan(&row.ID, &row.ClassID, &row.Title, &row.CreatedAt, &row.StorageName, &row.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return materialRow{}, http.StatusNotFound
	}
	if err != nil {
		return materialRow{}, http.StatusServiceUnavailable
	}
	if row.ClassID != u.ClassID {
		log.Printf("cross-class user=%d class=%d material=%d owner_class=%d", u.ID, u.ClassID, row.ID, row.ClassID)
		return materialRow{}, http.StatusNotFound
	}
	return row, 0
}

func (s *Server) uploadMaterial(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if u.Role != "teacher" {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if r.ContentLength > s.cfg.MaxUploadBytes+64*1024 {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+64*1024)
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(strings.ToLower(err.Error()), "too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_file")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file")
		return
	}
	defer file.Close()
	ext, allowed := materials.AllowedExt(hdr.Filename)
	if !allowed {
		writeError(w, http.StatusBadRequest, "invalid_file")
		return
	}
	buf, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxUploadBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file")
		return
	}
	if int64(len(buf)) > s.cfg.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
		return
	}
	buf = materials.StripBOM(buf)
	if err := materials.ValidateText(buf); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file")
		return
	}

	storage, err := newStorageName(ext)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	full, err := s.safeStoragePath(storage)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if err := os.WriteFile(full, buf, 0o640); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	title := materials.DisplayTitle(hdr.Filename)
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		_ = os.Remove(full)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	res, err := tx.ExecContext(r.Context(), `INSERT INTO materials
		(class_id, uploader_id, title, storage_name, original_ext, byte_size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ClassID, u.ID, title, storage, strings.TrimPrefix(ext, "."), len(buf), now)
	if err != nil {
		_ = tx.Rollback()
		_ = os.Remove(full)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		_ = os.Remove(full)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO knowledge_entries (material_id, class_id, body, created_at) VALUES (?, ?, ?, ?)`,
		id, u.ClassID, string(buf), now); err != nil {
		_ = tx.Rollback()
		_ = os.Remove(full)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if err = tx.Commit(); err != nil {
		_ = os.Remove(full)
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if s.retr != nil {
		if indexErr := s.retr.IndexMaterial(r.Context(), id, chunk.Options{Strategy: chunk.StrategyAuto}); indexErr != nil {
			log.Printf("index material %d after upload: %v", id, indexErr)
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "title": title})
}

func (s *Server) safeStoragePath(name string) (string, error) {
	if !storageNameRe.MatchString(name) {
		return "", errors.New("invalid storage name")
	}
	return filepath.Join(s.cfg.UploadDir, name), nil
}

func newStorageName(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf) + ext, nil
}

func statusCode(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusServiceUnavailable:
		return "unavailable"
	default:
		return "not_found"
	}
}

func likePattern(q string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(q) + "%"
}

func contentDisposition(title string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, title)
	if clean == "" {
		clean = "material"
	}
	return `attachment; filename="` + clean + `"`
}
