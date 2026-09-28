package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const Collection = "campusclaw_chunks"

type Client struct {
	base string
	dim  int
	http *http.Client
}

type Point struct {
	ID      uint64         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type Hit struct {
	ID      uint64
	Score   float64
	Payload map[string]any
}

func New(base string, dim int) *Client {
	return &Client{
		base: strings.TrimRight(strings.TrimSpace(base), "/"),
		dim:  dim,
		http: &http.Client{Timeout: 8 * time.Second},
	}
}

func (c *Client) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/collections/"+Collection, nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode < 500
}

func (c *Client) EnsureCollection(ctx context.Context) error {
	body := map[string]any{
		"vectors": map[string]any{
			"size":     c.dim,
			"distance": "Cosine",
		},
	}
	status, _, err := c.do(ctx, http.MethodPut, "/collections/"+Collection, body)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusCreated || status == http.StatusConflict {
		return nil
	}
	// 集合已存在时 Qdrant 仍可能返回 200；409 也视为成功。
	if status >= 400 && status != http.StatusConflict {
		// 再读一次，已存在即可。
		if c.Available(ctx) {
			return nil
		}
		return fmt.Errorf("create collection status %d", status)
	}
	return nil
}

func (c *Client) Upsert(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	if err := c.EnsureCollection(ctx); err != nil {
		return err
	}
	status, raw, err := c.do(ctx, http.MethodPut, "/collections/"+Collection+"/points?wait=true", map[string]any{
		"points": points,
	})
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("qdrant upsert %d: %s", status, raw)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	status, raw, err := c.do(ctx, http.MethodPost, "/collections/"+Collection+"/points/delete?wait=true", map[string]any{
		"points": ids,
	})
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("qdrant delete %d: %s", status, raw)
	}
	return nil
}

func (c *Client) Search(ctx context.Context, vector []float32, classID int64, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 8
	}
	if err := c.EnsureCollection(ctx); err != nil {
		return nil, err
	}
	body := map[string]any{
		"vector":          vector,
		"limit":           limit,
		"with_payload":    true,
		"score_threshold": 0.35,
		"filter": map[string]any{
			"must": []map[string]any{{
				"key":   "class_id",
				"match": map[string]any{"value": classID},
			}},
		},
	}
	status, raw, err := c.do(ctx, http.MethodPost, "/collections/"+Collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("qdrant search %d: %s", status, raw)
	}
	var parsed struct {
		Result []struct {
			ID      json.Number    `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(parsed.Result))
	for _, item := range parsed.Result {
		id, err := item.ID.Int64()
		if err != nil {
			continue
		}
		if item.Score < 0.35 {
			continue
		}
		out = append(out, Hit{ID: uint64(id), Score: item.Score, Payload: item.Payload})
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, payload any) (int, string, error) {
	var rdr io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, "", err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return 0, "", err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, string(raw), nil
}
