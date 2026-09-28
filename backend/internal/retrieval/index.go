package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"campusclaw/internal/chunk"
	"campusclaw/internal/qdrant"
)

func (s *Service) Backfill(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT e.material_id
		FROM knowledge_entries e
		LEFT JOIN knowledge_chunks c ON c.knowledge_entry_id = e.id AND c.index_status='ready'
		GROUP BY e.material_id
		HAVING COUNT(c.id)=0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var last error
	for _, id := range ids {
		if err := s.IndexMaterial(ctx, id, chunk.Options{Strategy: chunk.StrategyAuto}); err != nil {
			log.Printf("index material %d: %v", id, err)
			last = err
		}
	}
	return last
}

func (s *Service) Reindex(ctx context.Context, materialID, classID int64, role string, opt chunk.Options) error {
	if role != "teacher" {
		return ErrForbidden
	}
	var ownerClass int64
	err := s.db.QueryRowContext(ctx, `SELECT class_id FROM materials WHERE id=?`, materialID).Scan(&ownerClass)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if ownerClass != classID {
		return ErrNotFound
	}
	return s.IndexMaterial(ctx, materialID, opt)
}

func (s *Service) IndexMaterial(ctx context.Context, materialID int64, opt chunk.Options) error {
	var (
		classID int64
		entryID int64
		body    string
	)
	err := s.db.QueryRowContext(ctx, `SELECT e.id, e.class_id, e.body
		FROM knowledge_entries e WHERE e.material_id=?`, materialID).Scan(&entryID, &classID, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	oldIDs, err := s.chunkIDs(ctx, materialID)
	if err != nil {
		return err
	}
	if len(oldIDs) > 0 {
		_ = s.qdrant.Delete(ctx, oldIDs)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM knowledge_chunks WHERE material_id=?`, materialID); err != nil {
			return err
		}
	}

	parts := chunk.Split(body, opt)
	if len(parts) == 0 {
		return nil
	}
	now := time.Now().UTC()
	ids := make([]int64, 0, len(parts))
	texts := make([]string, 0, len(parts))
	strategy := chunk.NormalizeStrategy(opt.Strategy)
	for _, p := range parts {
		res, err := s.db.ExecContext(ctx, `INSERT INTO knowledge_chunks
			(class_id, material_id, knowledge_entry_id, chunk_index, chunk_text, start_offset, end_offset, index_status, strategy, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
			classID, materialID, entryID, p.Index, p.Text, p.Start, p.End, strategy, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		ids = append(ids, id)
		texts = append(texts, p.Text)
	}

	vecs, err := s.embed.Embed(ctx, texts)
	if err != nil || len(vecs) != len(ids) {
		s.markFailed(ctx, ids)
		return err
	}
	points := make([]qdrant.Point, 0, len(ids))
	for i, id := range ids {
		if len(vecs[i]) == 0 {
			s.markFailed(ctx, ids)
			return errors.New("empty embedding")
		}
		points = append(points, qdrant.Point{
			ID:     uint64(id),
			Vector: vecs[i],
			Payload: map[string]any{
				"class_id":           classID,
				"material_id":        materialID,
				"knowledge_entry_id": entryID,
				"chunk_id":           id,
				"chunk_index":        parts[i].Index,
			},
		})
	}
	if err := s.qdrant.Upsert(ctx, points); err != nil {
		s.markFailed(ctx, ids)
		_ = s.qdrant.Delete(ctx, uint64s(ids))
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE knowledge_chunks SET index_status='ready' WHERE material_id=?`, materialID); err != nil {
		return err
	}
	return nil
}

func (s *Service) chunkIDs(ctx context.Context, materialID int64) ([]uint64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM knowledge_chunks WHERE material_id=?`, materialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, uint64(id))
	}
	return ids, rows.Err()
}

func (s *Service) markFailed(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		_, _ = s.db.ExecContext(ctx, `UPDATE knowledge_chunks SET index_status='failed' WHERE id=?`, id)
	}
}

func uint64s(ids []int64) []uint64 {
	out := make([]uint64, len(ids))
	for i, id := range ids {
		out[i] = uint64(id)
	}
	return out
}
