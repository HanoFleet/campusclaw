package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"campusclaw/internal/chat"
	"campusclaw/internal/embed"
	"campusclaw/internal/qdrant"
)

const (
	ModeKeyword = "keyword"
	ModeVector  = "vector"
	ModeHybrid  = "hybrid"

	NoHitMessage = "资料中未找到相关内容"
)

var (
	ErrUnavailable = errors.New("retrieval unavailable")
	ErrEmptyQuery  = errors.New("empty query")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not found")
)

type Service struct {
	db     *sql.DB
	embed  embed.Embedder
	qdrant *qdrant.Client
	chat   chat.Completer
}

func New(db *sql.DB, emb embed.Embedder, qd *qdrant.Client, completer chat.Completer) *Service {
	return &Service{db: db, embed: emb, qdrant: qd, chat: completer}
}

func (s *Service) ChatCalls() int64 {
	if s.chat == nil {
		return 0
	}
	return s.chat.Calls()
}

func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeHybrid:
		return ModeHybrid
	case ModeKeyword:
		return ModeKeyword
	case ModeVector:
		return ModeVector
	default:
		return ModeHybrid
	}
}

type Hit struct {
	ChunkID    int64    `json:"-"`
	MaterialID int64    `json:"material_id"`
	Title      string   `json:"title"`
	ChunkIndex int      `json:"chunk_index"`
	Start      int      `json:"start"`
	End        int      `json:"end"`
	Excerpt    string   `json:"excerpt"`
	Score      float64  `json:"score,omitempty"`
	Rank       int      `json:"rank,omitempty"`
	Keyword    *float64 `json:"keyword_score,omitempty"`
	Vector     *float64 `json:"vector_score,omitempty"`
}

type SearchResult struct {
	Mode    string `json:"mode"`
	Message string `json:"message,omitempty"`
	Hits    []Hit  `json:"hits"`
}

type AskResult struct {
	Answer    string `json:"answer"`
	Citations []Hit  `json:"citations"`
}

func (s *Service) Search(ctx context.Context, classID int64, query, mode string, limit int) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, ErrEmptyQuery
	}
	mode = NormalizeMode(mode)
	if limit <= 0 {
		limit = 8
	}
	var (
		hits []Hit
		err  error
	)
	switch mode {
	case ModeKeyword:
		hits, err = s.keyword(ctx, classID, query, limit)
	case ModeVector:
		hits, err = s.vector(ctx, classID, query, limit)
	default:
		hits, err = s.hybrid(ctx, classID, query, limit)
	}
	if err != nil {
		return SearchResult{}, err
	}
	if hits == nil {
		hits = []Hit{}
	}
	out := SearchResult{Mode: mode, Hits: hits}
	if len(hits) == 0 {
		out.Message = NoHitMessage
	}
	return out, nil
}

func (s *Service) Ask(ctx context.Context, classID int64, query string, history []chat.Message) (AskResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return AskResult{}, ErrEmptyQuery
	}
	filtered := make([]chat.Message, 0, len(history))
	for _, m := range history {
		if strings.EqualFold(m.Role, "system") {
			continue
		}
		if m.Role == "user" || m.Role == "assistant" {
			filtered = append(filtered, chat.Message{Role: m.Role, Content: m.Content})
		}
	}
	found, err := s.Search(ctx, classID, query, ModeHybrid, 4)
	if err != nil {
		return AskResult{}, err
	}
	if len(found.Hits) == 0 {
		return AskResult{Answer: NoHitMessage, Citations: []Hit{}}, nil
	}
	evidence := make([]string, 0, len(found.Hits))
	for _, h := range found.Hits {
		evidence = append(evidence, fmt.Sprintf("《%s》切片%d：%s", h.Title, h.ChunkIndex, h.Excerpt))
	}
	answer, err := s.chat.Complete(ctx, query, filtered, evidence)
	if err != nil {
		return AskResult{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return AskResult{Answer: answer, Citations: found.Hits}, nil
}

func (s *Service) keyword(ctx context.Context, classID int64, query string, limit int) ([]Hit, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.material_id, m.title, c.chunk_index, c.start_offset, c.end_offset, c.chunk_text,
		MATCH(c.chunk_text) AGAINST(? IN NATURAL LANGUAGE MODE) AS score
		FROM knowledge_chunks c
		JOIN materials m ON m.id = c.material_id
		WHERE c.class_id=? AND c.index_status='ready'
		AND MATCH(c.chunk_text) AGAINST(? IN NATURAL LANGUAGE MODE)
		ORDER BY score DESC, c.id ASC
		LIMIT ?`, query, classID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	rank := 1
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ChunkID, &h.MaterialID, &h.Title, &h.ChunkIndex, &h.Start, &h.End, &h.Excerpt, &h.Score); err != nil {
			return nil, err
		}
		h.Rank = rank
		score := h.Score
		h.Keyword = &score
		hits = append(hits, h)
		rank++
	}
	return hits, rows.Err()
}

func (s *Service) vector(ctx context.Context, classID int64, query string, limit int) ([]Hit, error) {
	if s.qdrant == nil || !s.qdrant.Available(ctx) {
		return nil, ErrUnavailable
	}
	vecs, err := s.embed.Embed(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, ErrUnavailable
	}
	found, err := s.qdrant.Search(ctx, vecs[0], classID, limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	ids := make([]int64, 0, len(found))
	scores := map[int64]float64{}
	for _, item := range found {
		id := int64(item.ID)
		ids = append(ids, id)
		scores[id] = item.Score
	}
	hits, err := s.loadHits(ctx, classID, ids)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		hits[i].Score = scores[hits[i].ChunkID]
		v := hits[i].Score
		hits[i].Vector = &v
		hits[i].Rank = i + 1
	}
	return hits, nil
}

func (s *Service) hybrid(ctx context.Context, classID int64, query string, limit int) ([]Hit, error) {
	if s.qdrant == nil || !s.qdrant.Available(ctx) {
		return nil, ErrUnavailable
	}
	kw, err := s.keyword(ctx, classID, query, limit)
	if err != nil {
		return nil, err
	}
	vec, err := s.vector(ctx, classID, query, limit)
	if err != nil {
		return nil, err
	}
	kwRank := make([]rankedItem, 0, len(kw))
	vecRank := make([]rankedItem, 0, len(vec))
	byID := map[int64]Hit{}
	for _, h := range kw {
		kwRank = append(kwRank, rankedItem{ID: h.ChunkID, Score: h.Score, Rank: h.Rank})
		cp := h
		byID[h.ChunkID] = cp
	}
	for _, h := range vec {
		vecRank = append(vecRank, rankedItem{ID: h.ChunkID, Score: h.Score, Rank: h.Rank})
		if old, ok := byID[h.ChunkID]; ok {
			old.Vector = h.Vector
			byID[h.ChunkID] = old
		} else {
			byID[h.ChunkID] = h
		}
	}
	merged := rrfMerge([][]rankedItem{kwRank, vecRank})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	out := make([]Hit, 0, len(merged))
	for _, item := range merged {
		h := byID[item.ID]
		h.Score = item.Score
		h.Rank = item.Rank
		out = append(out, h)
	}
	return out, nil
}

func (s *Service) loadHits(ctx context.Context, classID int64, ids []int64) ([]Hit, error) {
	if len(ids) == 0 {
		return []Hit{}, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, classID)
	query := `SELECT c.id, c.material_id, m.title, c.chunk_index, c.start_offset, c.end_offset, c.chunk_text
		FROM knowledge_chunks c
		JOIN materials m ON m.id = c.material_id
		WHERE c.id IN (` + placeholders + `) AND c.class_id=? AND c.index_status='ready'`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]Hit{}
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ChunkID, &h.MaterialID, &h.Title, &h.ChunkIndex, &h.Start, &h.End, &h.Excerpt); err != nil {
			return nil, err
		}
		byID[h.ChunkID] = h
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(ids))
	for _, id := range ids {
		if h, ok := byID[id]; ok {
			out = append(out, h)
		}
	}
	return out, nil
}
