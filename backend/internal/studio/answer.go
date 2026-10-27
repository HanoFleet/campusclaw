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

// 没有配置时的默认上限：带工具的请求最多 3 次，一次提问里最多执行 4 次工具。
const (
	defaultMaxRounds = 3
	defaultMaxCalls  = 4
)

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
	// MaxRounds 是带工具的请求次数上限，MaxCalls 是累计执行工具的次数上限。
	// 到了任一上限就不再提供工具，让模型用已有信息作答。
	MaxRounds int
	MaxCalls  int
}

type Input struct {
	ConversationID int64
	ClassID        int64
	UserID         int64
	Query          string
}

// Round 是一次调用模型接口：Request 是实际发出的请求体，Response 是接口原样返回的内容。
type Round struct {
	Request  json.RawMessage `json:"request"`
	Response json.RawMessage `json:"response"`
}

type Trace struct {
	Skill  string           `json:"skill,omitempty"`
	Tools  []map[string]any `json:"tools,omitempty"`
	Rounds []Round          `json:"rounds"`
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

	hits := []retrieval.Hit{}
	if NeedsSearch(route.Skill) {
		hits, err = r.Retr.SearchBound(ctx, classID, materialIDs, query, 4)
		if err != nil {
			return ErrUnavailable
		}
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
	hits = append([]retrieval.Hit{}, compact...)

	var refs []string
	if route.Skill != nil {
		refs = route.Skill.Tools
	}
	scope := mcphost.Scope{ClassID: classID, UserID: in.UserID, ConversationID: in.ConversationID}
	bound, denied, err := r.Host.Prepare(ctx, scope, refs)
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

	var calls []mcphost.CallStatus
	note := func(c mcphost.CallStatus) error {
		calls = append(calls, c)
		return emit("tool", publicCall(c))
	}
	for _, c := range denied {
		if err := note(c); err != nil {
			return err
		}
	}
	var rounds []Round
	ask := func(tools []chat.ToolDef) (chat.Completion, error) {
		req, _ := json.Marshal(chat.Payload(r.Model, messages, tools))
		comp, err := r.Chat.Round(ctx, messages, tools)
		if err != nil {
			return comp, ErrUnavailable
		}
		resp := comp.Raw
		if len(resp) == 0 {
			resp, _ = json.Marshal(comp)
		}
		rounds = append(rounds, Round{Request: req, Response: resp})
		r.saveExchange(ctx, in, len(rounds), req, resp)
		return comp, nil
	}
	content := ""
	maxRounds, maxCalls := limit(r.MaxRounds, defaultMaxRounds), limit(r.MaxCalls, defaultMaxCalls)
	used := 0
	for round := 1; ; round++ {
		offer := toolDefs
		if round > maxRounds || used >= maxCalls {
			offer = nil
			if len(toolDefs) > 0 {
				messages = append(messages, chat.Message{Role: "system", Content: "工具调用已经到上限。请根据已有信息直接回答，不要再调用工具。"})
			}
		}
		comp, err := ask(offer)
		if err != nil {
			return err
		}
		if len(comp.ToolCalls) == 0 || len(offer) == 0 {
			content = comp.Content
			break
		}
		messages = append(messages, chat.Message{Role: "assistant", Content: comp.Content, ToolCalls: comp.ToolCalls})
		for _, call := range comp.ToolCalls {
			ref := refByName[call.Name]
			var result mcphost.CallStatus
			switch {
			case ref == "":
				result = mcphost.CallStatus{Ref: call.Name, Status: "denied", Detail: "不在这次允许的工具里", Text: "这个工具不在本次允许的清单里。"}
			case used >= maxCalls:
				result = r.Host.Refuse(ctx, scope, ref, "超过本次提问的调用次数上限", round)
			default:
				used++
				result = r.Host.Invoke(ctx, scope, ref, json.RawMessage(call.Arguments), round)
			}
			if err := note(result); err != nil {
				return err
			}
			messages = append(messages, chat.Message{Role: "tool", ToolCallID: call.ID, Content: result.Text})
		}
	}
	if strings.TrimSpace(content) == "" {
		comp, err := ask(nil)
		if err != nil {
			return err
		}
		content = comp.Content
	}
	if len(hits) == 0 {
		content = citeRe.ReplaceAllString(content, "")
		content = strings.TrimSpace(content)
	}
	if content == "" {
		content = "这次没有写出可用的回答。"
	}

	trace := Trace{Rounds: rounds, Tools: publicCalls(calls)}
	if route.Skill != nil {
		trace.Skill = route.Skill.Name
	}
	meta := map[string]any{
		"status":    "ok",
		"citations": hits,
	}
	if route.Skill != nil {
		meta["skill"] = map[string]string{"name": route.Skill.Name}
	}
	if len(trace.Tools) > 0 {
		meta["tool_calls"] = trace.Tools
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

func publicCall(c mcphost.CallStatus) map[string]any {
	return map[string]any{"ref": c.Ref, "status": c.Status, "detail": c.Detail}
}

func publicCalls(calls []mcphost.CallStatus) []map[string]any {
	if len(calls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(calls))
	for _, c := range calls {
		out = append(out, publicCall(c))
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

func limit(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
