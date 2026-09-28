package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"campusclaw/internal/chat"
	"campusclaw/internal/chunk"
	"campusclaw/internal/retrieval"
)

type searchRequest struct {
	Query   string `json:"query"`
	Mode    string `json:"mode"`
	ClassID any    `json:"class_id"`
}

type askRequest struct {
	Query    string         `json:"query"`
	Messages []chat.Message `json:"messages"`
	ClassID  any            `json:"class_id"`
}

type reindexRequest struct {
	Strategy      string  `json:"strategy"`
	MaxLen        int     `json:"max_len"`
	OverlapRatio  float64 `json:"overlap_ratio"`
	StripURL      bool    `json:"strip_urls"`
	CollapseSpace bool    `json:"collapse_space"`
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req searchRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = req.ClassID
	result, err := s.retr.Search(r.Context(), u.ClassID, req.Query, req.Mode, 8)
	if err != nil {
		writeRetrievalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var req askRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = req.ClassID
	query := req.Query
	if query == "" {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" && req.Messages[i].Content != "" {
				query = req.Messages[i].Content
				break
			}
		}
	}
	result, err := s.retr.Ask(r.Context(), u.ClassID, query, req.Messages)
	if err != nil {
		writeRetrievalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) reindex(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var req reindexRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	err = s.retr.Reindex(r.Context(), id, u.ClassID, u.Role, chunk.Options{
		Strategy:      req.Strategy,
		MaxLen:        req.MaxLen,
		OverlapRatio:  req.OverlapRatio,
		StripURL:      req.StripURL,
		CollapseSpace: req.CollapseSpace,
	})
	if err != nil {
		writeRetrievalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeRetrievalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, retrieval.ErrEmptyQuery):
		writeError(w, http.StatusBadRequest, "invalid_query")
	case errors.Is(err, retrieval.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, retrieval.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, retrieval.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "unavailable")
	default:
		writeError(w, http.StatusServiceUnavailable, "unavailable")
	}
}
