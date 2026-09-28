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

const LocalDim = 256

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
	client *http.Client
	calls  atomic.Int64
}

func NewHTTP(url, key, model string) *HTTP {
	return &HTTP{
		url:    strings.TrimRight(strings.TrimSpace(url), "/"),
		key:    key,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *HTTP) Dim() int     { return LocalDim }
func (h *HTTP) Calls() int64 { return h.calls.Load() }

func (h *HTTP) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
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
	}
	return out, nil
}

func New(url, key, model string) Embedder {
	u := strings.TrimSpace(url)
	if u == "" || strings.EqualFold(u, "local") || strings.HasPrefix(strings.ToLower(u), "local:") {
		return Local{}
	}
	return NewHTTP(u, key, model)
}
