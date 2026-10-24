package retrieval

import (
	"context"
	"strings"
)

// SearchBound 只在指定材料里做混合检索。材料列表为空时直接没有命中，不去扫全班。
func (s *Service) SearchBound(ctx context.Context, classID int64, materialIDs []int64, query string, limit int) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}
	if len(materialIDs) == 0 {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = 4
	}
	if s.qdrant == nil || !s.qdrant.Available(ctx) {
		return nil, ErrUnavailable
	}
	kw, err := s.keywordIDs(ctx, classID, materialIDs, query, limit)
	if err != nil {
		return nil, err
	}
	vec, err := s.vectorIDs(ctx, classID, materialIDs, query, limit)
	if err != nil {
		return nil, err
	}
	kwRank := make([]rankedItem, 0, len(kw))
	vecRank := make([]rankedItem, 0, len(vec))
	byID := map[int64]Hit{}
	for _, h := range kw {
		kwRank = append(kwRank, rankedItem{ID: h.ChunkID, Score: h.Score, Rank: h.Rank})
		byID[h.ChunkID] = h
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

func (s *Service) keywordIDs(ctx context.Context, classID int64, materialIDs []int64, query string, limit int) ([]Hit, error) {
	ph := strings.Repeat("?,", len(materialIDs))
	ph = ph[:len(ph)-1]
	args := []any{query, classID}
	for _, id := range materialIDs {
		args = append(args, id)
	}
	args = append(args, query, limit)
	q := `SELECT c.id, c.material_id, m.title, c.chunk_index, c.start_offset, c.end_offset, c.chunk_text,
		MATCH(c.chunk_text) AGAINST(? IN NATURAL LANGUAGE MODE) AS score
		FROM knowledge_chunks c
		JOIN materials m ON m.id = c.material_id
		WHERE c.class_id=? AND c.index_status='ready' AND c.material_id IN (` + ph + `)
		AND MATCH(c.chunk_text) AGAINST(? IN NATURAL LANGUAGE MODE)
		ORDER BY score DESC, c.id ASC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, args...)
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
	if hits == nil {
		hits = []Hit{}
	}
	return hits, rows.Err()
}

func (s *Service) vectorIDs(ctx context.Context, classID int64, materialIDs []int64, query string, limit int) ([]Hit, error) {
	vecs, err := s.embed.Embed(ctx, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil, ErrUnavailable
	}
	found, err := s.qdrant.SearchMaterials(ctx, vecs[0], classID, materialIDs, limit)
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
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		ok := false
		for _, id := range materialIDs {
			if h.MaterialID == id {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		h.Score = scores[h.ChunkID]
		v := h.Score
		h.Vector = &v
		h.Rank = len(out) + 1
		out = append(out, h)
	}
	return out, nil
}
