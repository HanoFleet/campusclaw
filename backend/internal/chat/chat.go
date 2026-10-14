package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Completer interface {
	Complete(ctx context.Context, userQuery string, history []Message, evidence []string) (string, error)
	Calls() int64
}

type Local struct{}

func (Local) Calls() int64 { return 0 }

func (Local) Complete(_ context.Context, userQuery string, _ []Message, evidence []string) (string, error) {
	if len(evidence) == 0 {
		return "资料中未找到相关内容", nil
	}
	return pickAnswer(userQuery, evidence) + "[1]", nil
}

func pickAnswer(query string, evidence []string) string {
	q := []rune(strings.TrimSpace(query))
	best := ""
	bestScore := 0
	for _, item := range evidence {
		for _, sent := range splitSentences(stripLabel(item)) {
			score := overlap(q, []rune(sent))
			if strings.Contains(sent, "不能") || strings.Contains(sent, "不会") {
				score += 2
			}
			if score > bestScore {
				bestScore = score
				best = sent
			}
		}
	}
	if best == "" {
		best = clip(stripLabel(evidence[0]), 40)
	}
	return strings.TrimSpace(best)
}

func stripLabel(s string) string {
	if i := strings.Index(s, "："); i >= 0 && i < 40 {
		return s[i+len("："):]
	}
	return s
}

func splitSentences(s string) []string {
	s = strings.ReplaceAll(s, "\n", "")
	var out []string
	var cur []rune
	for _, r := range s {
		cur = append(cur, r)
		if r == '。' || r == '！' || r == '？' || r == '.' {
			if t := strings.TrimSpace(string(cur)); t != "" {
				out = append(out, t)
			}
			cur = nil
		}
	}
	if t := strings.TrimSpace(string(cur)); t != "" {
		out = append(out, t)
	}
	return out
}

func overlap(query, sent []rune) int {
	if len(query) == 0 || len(sent) == 0 {
		return 0
	}
	set := map[rune]bool{}
	for _, r := range sent {
		set[r] = true
	}
	n := 0
	for _, r := range query {
		if r == '吗' || r == '？' || r == '?' || r == '能' || r == '的' || r == '是' {
			continue
		}
		if set[r] {
			n++
		}
	}
	return n
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
		client: &http.Client{Timeout: 45 * time.Second},
	}
}

func (h *HTTP) Calls() int64 { return h.calls.Load() }

func (h *HTTP) Complete(ctx context.Context, userQuery string, history []Message, evidence []string) (string, error) {
	h.calls.Add(1)
	var sys strings.Builder
	sys.WriteString("你只能根据下面编号资料作答。回答必须使用 [1]、[2] 对应资料顺序。不得使用资料以外的知识。\n")
	for i, item := range evidence {
		fmt.Fprintf(&sys, "[%d] %s\n", i+1, item)
	}
	msgs := []Message{{Role: "system", Content: sys.String()}}
	for _, m := range history {
		if m.Role == "user" || m.Role == "assistant" {
			msgs = append(msgs, Message{Role: m.Role, Content: m.Content})
		}
	}
	msgs = append(msgs, Message{Role: "user", Content: userQuery})
	payload, err := json.Marshal(map[string]any{
		"model":       h.model,
		"temperature": 0,
		"messages":    msgs,
	})
	if err != nil {
		return "", err
	}
	endpoint := h.url
	if !strings.Contains(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.key != "" {
		req.Header.Set("Authorization", "Bearer "+h.key)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("chat gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("empty chat response")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

func New(url, key, model string) Completer {
	u := strings.TrimSpace(url)
	if u == "" || strings.EqualFold(u, "local") || strings.HasPrefix(strings.ToLower(u), "local:") {
		return Local{}
	}
	return NewHTTP(u, key, model)
}

func clip(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n]) + "…"
}
