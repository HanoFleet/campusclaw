package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

const (
	LocalDim     = 256
	GatewayDim   = 2048
	embedBatch   = 16
	embedMaxBody = 16 << 20
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
	Calls() int64
}

type Local struct{}

func (Local) Dim() int     { return LocalDim }
func (Local) Calls() int64 { return 0 }
func (Local) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = ngramVector(text)
	}
	return out, nil
}

func ngramVector(text string) []float32 {
	v := make([]float32, LocalDim)
	runes := []rune(strings.ToLower(text))
	add := func(part []rune, w float32) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(string(part)))
		v[int(h.Sum32()%LocalDim)] += w
	}
	for i, r := range runes {
		if unicode.IsSpace(r) {
			continue
		}
		add(runes[i:i+1], 1)
		if i+1 < len(runes) && !unicode.IsSpace(runes[i+1]) {
			add(runes[i:i+2], 2)
		}
		if i+2 < len(runes) && !unicode.IsSpace(runes[i+1]) && !unicode.IsSpace(runes[i+2]) {
			add(runes[i:i+3], 1)
		}
	}
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return v
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
	return v
}

type HTTP struct {
	url    string
	key    string
	model  string
	dim    int
	client *http.Client
	calls  atomic.Int64
}

func NewHTTP(url, key, model string) *HTTP {
	return &HTTP{
		url:    strings.TrimRight(strings.TrimSpace(url), "/"),
		key:    key,
		model:  model,
		dim:    DimFor(url, model),
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (h *HTTP) Dim() int     { return h.dim }
func (h *HTTP) Calls() int64 { return h.calls.Load() }

func (h *HTTP) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += embedBatch {
		end := start + embedBatch
		if end > len(texts) {
			end = len(texts)
		}
		part, err := h.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		copy(out[start:], part)
	}
	return out, nil
}

func (h *HTTP) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	h.calls.Add(1)
	payload, err := json.Marshal(map[string]any{"model": h.model, "input": texts})
	if err != nil {
		return nil, err
	}
	endpoint := h.url
	if !strings.Contains(endpoint, "/embeddings") {
		endpoint += "/embeddings"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.key != "" {
		req.Header.Set("Authorization", "Bearer "+h.key)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, embedMaxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(out) {
			out[item.Index] = item.Embedding
		}
	}
	for i, vec := range out {
		if len(vec) == 0 {
			return nil, fmt.Errorf("embedding missing for index %d", i)
		}
		if len(vec) != h.dim {
			return nil, fmt.Errorf("embedding dim %d, want %d", len(vec), h.dim)
		}
	}
	return out, nil
}

func isLocalURL(url string) bool {
	u := strings.TrimSpace(url)
	return u == "" || strings.EqualFold(u, "local") || strings.HasPrefix(strings.ToLower(u), "local:")
}

// DimFor 按地址和模型名决定向量维度。本地 ngram 是 256；课程网关 course-embedding 是 2048。
func DimFor(url, model string) int {
	if isLocalURL(url) {
		return LocalDim
	}
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "256") || strings.Contains(m, "ngram") {
		return LocalDim
	}
	return GatewayDim
}

func New(url, key, model string) Embedder {
	if isLocalURL(url) {
		return Local{}
	}
	return NewHTTP(url, key, model)
}
