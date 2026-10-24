package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"campusclaw/internal/chat"
	"campusclaw/internal/mcphost"
	"campusclaw/internal/retrieval"
)

var citeRe = regexp.MustCompile(`\[[0-9]+\]`)

var (
	ErrNotFound    = errors.New("not found")
	ErrUnavailable = errors.New("unavailable")
	ErrBadQuery    = errors.New("bad query")
)

type Runner struct {
	DB    *sql.DB
	Retr  *retrieval.Service
	Chat  chat.Completer
	Host  *mcphost.Host
	Model string
}

type Input struct {
	ConversationID int64
	ClassID        int64
	UserID         int64
	Query          string
}

type Trace struct {
	Model                  string           `json:"model"`
	Temperature            float64          `json:"temperature"`
	Messages               []chat.Message   `json:"messages"`
	Tools                  []map[string]any `json:"tools,omitempty"`
	PromptTemplate         string           `json:"prompt_template"`
	PromptTemplateIncluded bool             `json:"prompt_template_included"`
	Knowledge              map[string]any   `json:"knowledge"`
	Skill                  map[string]any   `json:"skill"`
	SkillExecution         string           `json:"skill_execution"`
	MCP                    map[string]any   `json:"mcp"`
	Rounds                 int              `json:"rounds"`
}

func (r *Runner) Run(ctx context.Context, in Input, emit func(string, any) error) error {
	query := strings.TrimSpace(in.Query)
	n := utf8.RuneCountInString(query)
	if n < 2 || n > 500 {
		return ErrBadQuery
	}
	var agentID int64
	var title, prompt string
	var owner, classID int64
	err := r.DB.QueryRowContext(ctx, `SELECT c.user_id, c.class_id, c.agent_id, c.title, a.prompt
		FROM conversations c
		JOIN tutor_agents a ON a.id = c.agent_id
		WHERE c.id=?`, in.ConversationID).Scan(&owner, &classID, &agentID, &title, &prompt)
	if err == sql.ErrNoRows || owner != in.UserID || classID != in.ClassID {
		return ErrNotFound
	}
	if err != nil {
		return ErrUnavailable
	}

	skills, err := r.loadSkills(ctx, agentID, classID)
	if err != nil {
		return err
	}
	materialIDs, err := r.loadMaterials(ctx, agentID)
	if err != nil {
		return err
	}
	route := Route(skills, query)
	now := time.Now().UTC()
	_, err = r.DB.ExecContext(ctx, `INSERT INTO messages (conversation_id, role, content, created_at) VALUES (?, 'user', ?, ?)`,
		in.ConversationID, query, now)
	if err != nil {
		return ErrUnavailable
	}
	if title == "新的提问" {
		next := clipRunes(query, 24)
		if next == "" {
			next = "新的提问"
		}
		_, _ = r.DB.ExecContext(ctx, `UPDATE conversations SET title=?, updated_at=? WHERE id=? AND title='新的提问'`, next, now, in.ConversationID)
	} else {
		_, _ = r.DB.ExecContext(ctx, `UPDATE conversations SET updated_at=? WHERE id=?`, now, in.ConversationID)
	}

	hits, err := r.Retr.SearchBound(ctx, classID, materialIDs, query, 4)
	if err != nil {
		if errors.Is(err, retrieval.ErrUnavailable) {
			return ErrUnavailable
		}
		return ErrUnavailable
	}
	if route.Skill != nil && strings.TrimSpace(route.Skill.Subject) != "" {
		filtered := hits[:0]
		for _, h := range hits {
			if strings.Contains(h.Title, route.Skill.Subject) {
				filtered = append(filtered, h)
			}
		}
		hits = append([]retrieval.Hit(nil), filtered...)
	}
	compact := hits[:0]
	for _, h := range hits {
		if strings.TrimSpace(h.Excerpt) != "" {
			compact = append(compact, h)
		}
	}
	hits = append([]retrieval.Hit(nil), compact...)

	var refs []string
	if route.Skill != nil {
		refs = route.Skill.Tools
	}
	bound, denied, err := r.Host.Prepare(ctx, classID, in.UserID, refs)
	if err != nil {
		return ErrUnavailable
	}
	toolDefs := make([]chat.ToolDef, 0, len(bound))
	refByName := map[string]string{}
	for _, item := range bound {
		toolDefs = append(toolDefs, item.Def)
		refByName[item.Def.Name] = item.Ref
	}

	history, err := r.history(ctx, in.ConversationID)
	if err != nil {
		return err
	}
	messages := TurnMessages(prompt, route.Skill, history, query, hits)
	sentPrompt := strings.TrimSpace(prompt)
	if sentPrompt == "" {
		sentPrompt = DefaultPrompt
	}

	var calls []mcphost.CallStatus
	calls = append(calls, denied...)
	content := ""
	rounds := 0
	for round := 1; round <= 3; round++ {
		rounds = round
		req := requestBody(r.Model, messages, toolDefs)
		comp, err := r.Chat.Round(ctx, messages, toolDefs)
		if err != nil {
			return ErrUnavailable
		}
		resp, _ := json.Marshal(comp)
		if len(comp.Raw) > 0 {
			resp = comp.Raw
		}
		r.saveExchange(ctx, in, round, req, resp)
		if len(comp.ToolCalls) == 0 || len(toolDefs) == 0 {
			content = comp.Content
			break
		}
		messages = append(messages, chat.Message{Role: "assistant", Content: comp.Content, ToolCalls: comp.ToolCalls})
		for _, call := range comp.ToolCalls {
			ref := refByName[call.Name]
			var result mcphost.CallStatus
			if ref == "" {
				result = mcphost.CallStatus{Ref: call.Name, Status: "denied", Detail: "不在这次允许的工具里", Text: "这个工具不在本次允许的清单里。"}
			} else {
				result = r.Host.Invoke(ctx, classID, in.UserID, ref, json.RawMessage(call.Arguments), round)
			}
			calls = append(calls, result)
			messages = append(messages, chat.Message{Role: "tool", ToolCallID: call.ID, Content: result.Text})
		}
		toolDefs = nil
		if round == 3 {
			messages = append(messages, chat.Message{Role: "system", Content: "工具调用已经到上限。请根据已有信息直接回答，不要再调用工具。"})
		}
	}
	if strings.TrimSpace(content) == "" {
		comp, err := r.Chat.Round(ctx, messages, nil)
		if err != nil {
			return ErrUnavailable
		}
		content = comp.Content
		rounds++
	}
	if len(hits) == 0 {
		content = citeRe.ReplaceAllString(content, "")
		content = strings.TrimSpace(content)
	}
	if content == "" {
		content = "这次没有写出可用的回答。"
	}

	trace := r.trace(r.Model, sentPrompt, messages, bound, route, materialIDs, hits, calls, rounds, skills)
	meta := map[string]any{
		"status":    "ok",
		"citations": hits,
	}
	if route.Skill != nil {
		meta["skill"] = map[string]string{"name": route.Skill.Name}
	}
	publicCalls := publicCalls(calls)
	if len(publicCalls) > 0 {
		meta["tool_calls"] = publicCalls
	}
	for _, call := range publicCalls {
		if err := emit("tool", call); err != nil {
			return err
		}
	}
	if err := emit("meta", meta); err != nil {
		return err
	}
	for _, piece := range chunks(content, 12) {
		if err := emit("delta", map[string]string{"text": piece}); err != nil {
			return err
		}
	}
	citeJSON, _ := json.Marshal(hits)
	traceJSON, _ := json.Marshal(trace)
	skillName := ""
	if route.Skill != nil {
		skillName = route.Skill.Name
	}
	msg, err := r.DB.ExecContext(ctx, `INSERT INTO messages
		(conversation_id, role, content, citations_json, skill_name, trace_json, created_at)
		VALUES (?, 'assistant', ?, ?, ?, ?, ?)`,
		in.ConversationID, content, string(citeJSON), nullIfEmpty(skillName), string(traceJSON), time.Now().UTC())
	if err != nil {
		return ErrUnavailable
	}
	id, _ := msg.LastInsertId()
	return emit("done", map[string]any{"message_id": id, "trace": trace})
}

func (r *Runner) loadSkills(ctx context.Context, agentID, classID int64) ([]Skill, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT s.id, s.name, s.trigger_text, s.subject_text, s.description, s.tools_json, s.enabled
		FROM tutor_agent_skills b
		JOIN skills s ON s.id = b.skill_id
		WHERE b.agent_id=? AND s.class_id=?
		ORDER BY s.id ASC`, agentID, classID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var out []Skill
	for rows.Next() {
		var s Skill
		var tools string
		var enabled int
		if err := rows.Scan(&s.ID, &s.Name, &s.Trigger, &s.Subject, &s.Description, &tools, &enabled); err != nil {
			return nil, ErrUnavailable
		}
		s.Enabled = enabled == 1
		_ = json.Unmarshal([]byte(tools), &s.Tools)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Runner) loadMaterials(ctx context.Context, agentID int64) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT material_id FROM tutor_agent_materials WHERE agent_id=? ORDER BY material_id`, agentID)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, ErrUnavailable
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Runner) history(ctx context.Context, conversationID int64) ([]chat.Message, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT role, content FROM messages WHERE conversation_id=? ORDER BY id DESC LIMIT ?`, conversationID, historyLimit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var rev []chat.Message
	for rows.Next() {
		var m chat.Message
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, ErrUnavailable
		}
		rev = append(rev, m)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnavailable
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev, nil
}

func (r *Runner) saveExchange(ctx context.Context, in Input, round int, req, resp json.RawMessage) {
	_, _ = r.DB.ExecContext(ctx, `INSERT INTO llm_exchanges
		(class_id, user_id, conversation_id, round_no, request_json, response_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.ClassID, in.UserID, in.ConversationID, round, string(req), string(resp), time.Now().UTC())
}

func (r *Runner) trace(model, prompt string, messages []chat.Message, bound []mcphost.BoundTool, route RouteHit, materialIDs []int64, hits []retrieval.Hit, calls []mcphost.CallStatus, rounds int, all []Skill) Trace {
	sentChars := 0
	for _, h := range hits {
		sentChars += utf8.RuneCountInString(clipRunes(h.Excerpt, excerptRunes))
	}
	skillInfo := map[string]any{
		"selected":                   nil,
		"sent_full_skill_file":       false,
		"sent_description_only":      route.Skill != nil,
		"other_bound_skills_omitted": omitted(all, route.Skill),
	}
	if route.Skill != nil {
		skillInfo["selected"] = route.Skill.Name
		skillInfo["description_chars"] = utf8.RuneCountInString(route.Skill.Description)
		skillInfo["matched_words"] = route.Matched
	}
	toolPayload := make([]map[string]any, 0, len(bound))
	for _, item := range bound {
		toolPayload = append(toolPayload, map[string]any{
			"ref":         item.Ref,
			"name":        item.Def.Name,
			"description": item.Def.Description,
			"parameters":  json.RawMessage(item.Def.Parameters),
		})
	}
	return Trace{
		Model:                  model,
		Temperature:            0,
		Messages:               messages,
		Tools:                  toolPayload,
		PromptTemplate:         prompt,
		PromptTemplateIncluded: true,
		Knowledge: map[string]any{
			"bound_material_count": len(materialIDs),
			"sent_full_text":       false,
			"hit_count":            len(hits),
			"sent_excerpt_chars":   sentChars,
			"note":                 "绑定知识库后，每次只把检索到的前 4 条片段放进用户消息，不会把材料全文传给模型。",
		},
		Skill:          skillInfo,
		SkillExecution: executionText(route),
		MCP: map[string]any{
			"declarations_sent": toolPayload,
			"calls":             publicCalls(calls),
			"note":              "发给模型的是工具名称、说明和参数结构。令牌和工具实现留在服务端，模型只返回工具名和参数。",
		},
		Rounds: rounds,
	}
}

func executionText(route RouteHit) string {
	if route.Skill == nil {
		return "服务端先取出这个智能体绑定且启用的技能，再按适用条件做逐词匹配。这次没有词命中，所以不选技能，系统消息里也不会放任何技能说明。"
	}
	return "服务端不把选择权交给模型。它只在这个智能体绑定且启用的技能里，把适用条件切成词，看提问里出现了哪些，取得分最高的「" + route.Skill.Name + "」。选中之后，只把这一条技能的说明放进系统消息，其他技能的正文不会传。若名称里含有「解题引导」，再追加一条不写最终得数的要求。技能上的 knowledge_search 表示先做检索；写成「工具名@服务器名」的，才会变成发给模型的工具声明。"
}

func omitted(all []Skill, picked *Skill) []string {
	var names []string
	for _, s := range all {
		if !s.Enabled {
			continue
		}
		if picked != nil && s.ID == picked.ID {
			continue
		}
		names = append(names, s.Name)
	}
	if names == nil {
		names = []string{}
	}
	return names
}

func requestBody(model string, messages []chat.Message, tools []chat.ToolDef) json.RawMessage {
	payload := map[string]any{
		"model":       model,
		"temperature": 0,
		"messages":    messages,
	}
	if len(tools) > 0 {
		list := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			params := t.Parameters
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			list = append(list, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  params,
				},
			})
		}
		payload["tools"] = list
	}
	b, _ := json.Marshal(payload)
	return b
}

func publicCalls(calls []mcphost.CallStatus) []map[string]any {
	if len(calls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(calls))
	for _, c := range calls {
		out = append(out, map[string]any{"ref": c.Ref, "status": c.Status, "detail": c.Detail})
	}
	return out
}

func chunks(s string, n int) []string {
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	var out []string
	for i := 0; i < len(r); i += n {
		j := i + n
		if j > len(r) {
			j = len(r)
		}
		out = append(out, string(r[i:j]))
	}
	return out
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
