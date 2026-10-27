package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

func (Local) Round(_ context.Context, messages []Message, tools []ToolDef) (Completion, error) {
	if len(messages) == 0 {
		return Completion{Content: "请再说具体一点。"}, nil
	}
	question := lastUserText(messages)
	if i := strings.Index(question, "\n\n"); i >= 0 {
		question = question[:i]
	}
	if !hasToolResult(messages) {
		if call, ok := localToolCall(question, tools); ok {
			return Completion{ToolCalls: []ToolCall{call}, Raw: localRaw("", []ToolCall{call})}, nil
		}
	}
	content := localAnswer(messages, question)
	return Completion{Content: content, Raw: localRaw(content, nil)}, nil
}

// localRaw 让本地替身的返回和 OpenAI 兼容接口长得一样，方便同样解析和展示。
func localRaw(content string, calls []ToolCall) json.RawMessage {
	msg := map[string]any{"role": "assistant", "content": content}
	if len(calls) > 0 {
		msg["tool_calls"] = openAIMessages([]Message{{ToolCalls: calls}})[0]["tool_calls"]
	}
	raw, _ := json.Marshal(map[string]any{
		"model":   "local-extractive",
		"choices": []map[string]any{{"index": 0, "message": msg}},
	})
	return raw
}

// Payload 是调用 /chat/completions 时的请求体。记录日志和真正发送都用它。
func Payload(model string, messages []Message, tools []ToolDef) map[string]any {
	payload := map[string]any{
		"model":       model,
		"temperature": 0,
		"messages":    openAIMessages(messages),
	}
	if len(tools) > 0 {
		payload["tools"] = openAITools(tools)
		payload["tool_choice"] = "auto"
	}
	return payload
}

func (h *HTTP) Round(ctx context.Context, messages []Message, tools []ToolDef) (Completion, error) {
	h.calls.Add(1)
	body, err := json.Marshal(Payload(h.model, messages, tools))
	if err != nil {
		return Completion{}, err
	}
	endpoint := h.url
	if !strings.Contains(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Completion{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.key != "" && !strings.EqualFold(h.key, "local") {
		req.Header.Set("Authorization", "Bearer "+h.key)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return Completion{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Completion{}, err
	}
	if resp.StatusCode >= 300 {
		return Completion{}, fmt.Errorf("chat gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Completion{}, err
	}
	if len(parsed.Choices) == 0 {
		return Completion{}, fmt.Errorf("empty chat response")
	}
	msg := parsed.Choices[0].Message
	out := Completion{Content: strings.TrimSpace(msg.Content), Raw: raw}
	for _, c := range msg.ToolCalls {
		id := c.ID
		if id == "" {
			id = "call"
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{ID: id, Name: c.Function.Name, Arguments: c.Function.Arguments})
	}
	if out.Content == "" && len(out.ToolCalls) == 0 {
		return Completion{}, fmt.Errorf("empty chat response")
	}
	return out, nil
}

func openAIMessages(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		item := map[string]any{"role": m.Role, "content": m.Content}
		if m.ToolCallID != "" {
			item["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, c := range m.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   c.ID,
					"type": "function",
					"function": map[string]any{
						"name":      c.Name,
						"arguments": c.Arguments,
					},
				})
			}
			item["tool_calls"] = calls
		}
		out = append(out, item)
	}
	return out
}

func openAITools(tools []ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  json.RawMessage(params),
			},
		})
	}
	return out
}

func lastUserText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return messages[len(messages)-1].Content
}

func hasToolResult(messages []Message) bool {
	for _, m := range messages {
		if m.Role == "tool" {
			return true
		}
	}
	return false
}

func localToolCall(question string, tools []ToolDef) (ToolCall, bool) {
	q := question
	for _, t := range tools {
		switch {
		case strings.Contains(t.Name, "unit_convert") && wantsConvert(q):
			args, _ := json.Marshal(guessConvert(q))
			return ToolCall{ID: "call_unit", Name: t.Name, Arguments: string(args)}, true
		case strings.Contains(t.Name, "formula_hint") && (strings.Contains(q, "公式") || strings.Contains(q, "面积")):
			args, _ := json.Marshal(map[string]string{"topic": clip(q, 40)})
			return ToolCall{ID: "call_formula", Name: t.Name, Arguments: string(args)}, true
		}
	}
	return ToolCall{}, false
}

func wantsConvert(q string) bool {
	return strings.Contains(q, "换算") || strings.Contains(q, "厘米") || strings.Contains(q, "毫米") || strings.Contains(q, "千克") || strings.Contains(q, "千米")
}

func guessConvert(q string) map[string]any {
	from, to := "m", "cm"
	if strings.Contains(q, "厘米") && strings.Contains(q, "毫米") {
		from, to = "cm", "mm"
	} else if strings.Contains(q, "千克") {
		from, to = "kg", "g"
	} else if strings.Contains(q, "千米") {
		from, to = "km", "m"
	}
	value := 1.0
	if m := numberRe.FindString(q); m != "" {
		if v, err := strconv.ParseFloat(m, 64); err == nil {
			value = v
		}
	}
	return map[string]any{"value": value, "from": from, "to": to}
}

var numberRe = regexp.MustCompile(`[0-9]+(\.[0-9]+)?`)

func localAnswer(messages []Message, question string) string {
	var toolText string
	var system string
	for _, m := range messages {
		if m.Role == "tool" && strings.TrimSpace(m.Content) != "" {
			toolText = m.Content
		}
		if m.Role == "system" {
			system = m.Content
		}
	}
	if toolText != "" {
		return strings.TrimSpace(toolText)
	}
	guide := strings.Contains(system, "解题引导") || strings.Contains(system, "不要写出最终得数")
	noHit := strings.Contains(system, "不要写出 [1]")
	if noHit {
		if guide {
			return "先停在你已经写出的那一步，检查括号或单位有没有漏乘。这里不直接给得数。"
		}
		return "这次没有可以引用的材料段落，也没有拿到工具结果，我先不下结论。可以把问题说得更具体一些再问。"
	}
	if guide {
		line := "先检查这一步有没有做对，先不要算最后的得数。"
		if strings.Contains(question, "去括号") {
			line = "去括号时，括号前面的数要乘到每一项，先核对这一步。"
		}
		return line + "[1]"
	}
	excerpts := excerpts(messages)
	if len(excerpts) > 0 {
		return pickAnswer(question, excerpts) + "[1]"
	}
	return "按你的问题直接说，这次没有可引用的段落。"
}

func excerpts(messages []Message) []string {
	var user string
	for _, m := range messages {
		if m.Role == "user" {
			user = m.Content
		}
	}
	if i := strings.Index(user, "下面是检索到的资料"); i >= 0 {
		user = user[i:]
	}
	return []string{user}
}
