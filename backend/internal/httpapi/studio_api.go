package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"campusclaw/internal/chat"
	"campusclaw/internal/mcphost"
	"campusclaw/internal/skillfile"
	"campusclaw/internal/studio"
)

type agentBody struct {
	Name        string  `json:"name" required:"true" doc:"1 到 64 字，班内不重复"`
	Prompt      string  `json:"prompt" doc:"角色提示词，最多 2000 字"`
	Enabled     *bool   `json:"enabled" doc:"省略时为 true"`
	SkillIDs    []int64 `json:"skill_ids" doc:"绑定的本班技能"`
	MaterialIDs []int64 `json:"material_ids" doc:"绑定的本班材料，只在这些材料里检索；留空则检索不到任何材料"`
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	q := `SELECT id, name, prompt, enabled, created_at FROM tutor_agents WHERE class_id=?`
	if u.Role != "teacher" {
		q += ` AND enabled=1`
	}
	q += ` ORDER BY id ASC`
	rows, err := s.db.QueryContext(r.Context(), q, u.ClassID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, prompt string
		var enabled int
		var created time.Time
		if err := rows.Scan(&id, &name, &prompt, &enabled, &created); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		list = append(list, map[string]any{
			"id": id, "name": name, "prompt": prompt, "enabled": enabled == 1,
			"created_at": created.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": list})
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var body agentBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if msg := validateAgent(body); msg != "" {
		writeDetail(w, http.StatusBadRequest, "invalid_request", msg)
		return
	}
	if err := s.bindingsExist(r, u.ClassID, body.SkillIDs, body.MaterialIDs); err != nil {
		writeStatus(w, err)
		return
	}
	enabled := 1
	if body.Enabled != nil && !*body.Enabled {
		enabled = 0
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO tutor_agents (class_id, name, prompt, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ClassID, strings.TrimSpace(body.Name), strings.TrimSpace(body.Prompt), enabled, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeDetail(w, http.StatusBadRequest, "duplicate_name", "本班已经有同名应用")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	id, _ := res.LastInsertId()
	if err := replaceBindings(r, tx, id, body.SkillIDs, body.MaterialIDs); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": strings.TrimSpace(body.Name)})
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var classID int64
	var name, prompt string
	var enabled int
	err := s.db.QueryRowContext(r.Context(), `SELECT class_id, name, prompt, enabled FROM tutor_agents WHERE id=?`, id).Scan(&classID, &name, &prompt, &enabled)
	if err == sql.ErrNoRows || classID != u.ClassID || (u.Role != "teacher" && enabled == 0) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	skillIDs, _ := s.idList(r, `SELECT skill_id FROM tutor_agent_skills WHERE agent_id=? ORDER BY skill_id`, id)
	materialIDs, _ := s.idList(r, `SELECT material_id FROM tutor_agent_materials WHERE agent_id=? ORDER BY material_id`, id)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "name": name, "prompt": prompt, "enabled": enabled == 1,
		"skill_ids": skillIDs, "material_ids": materialIDs,
	})
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var classID int64
	err := s.db.QueryRowContext(r.Context(), `SELECT class_id FROM tutor_agents WHERE id=?`, id).Scan(&classID)
	if err == sql.ErrNoRows || classID != u.ClassID {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var body agentBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if msg := validateAgent(body); msg != "" {
		writeDetail(w, http.StatusBadRequest, "invalid_request", msg)
		return
	}
	if err := s.bindingsExist(r, u.ClassID, body.SkillIDs, body.MaterialIDs); err != nil {
		writeStatus(w, err)
		return
	}
	enabled := 1
	if body.Enabled != nil && !*body.Enabled {
		enabled = 0
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(r.Context(), `UPDATE tutor_agents SET name=?, prompt=?, enabled=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(body.Name), strings.TrimSpace(body.Prompt), enabled, time.Now().UTC(), id)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeDetail(w, http.StatusBadRequest, "duplicate_name", "本班已经有同名应用")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if err := replaceBindings(r, tx, id, body.SkillIDs, body.MaterialIDs); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, trigger_text, subject_text, description, tools_json, enabled
		FROM skills WHERE class_id=? ORDER BY id ASC`, u.ClassID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		item, err := scanSkill(rows)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": list})
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	source, err := readSkillSource(r)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, "invalid_skill", err.Error())
		return
	}
	parsed, err := skillfile.Parse(source)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, "invalid_skill", err.Error())
		return
	}
	if err := (&mcphost.Host{DB: s.db}).ValidateSaved(r.Context(), u.ClassID, parsed.Tools); err != nil {
		writeDetail(w, http.StatusBadRequest, "invalid_skill", err.Error())
		return
	}
	tools, _ := json.Marshal(parsed.Tools)
	enabled := 0
	if parsed.Enabled {
		enabled = 1
	}
	res, err := s.db.ExecContext(r.Context(), `INSERT INTO skills
		(name, class_id, trigger_text, description, tools_json, source_text, subject_text, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		parsed.Name, u.ClassID, parsed.Trigger, parsed.Description, string(tools), parsed.Source, parsed.Subject, enabled)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeDetail(w, http.StatusBadRequest, "duplicate_name", "本班已经有同名技能")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": parsed.Name})
}

func (s *Server) getSkill(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var classID int64
	var name, trigger, subject, desc, tools, source string
	var enabled int
	err := s.db.QueryRowContext(r.Context(), `SELECT class_id, name, trigger_text, subject_text, description, tools_json, source_text, enabled FROM skills WHERE id=?`, id).
		Scan(&classID, &name, &trigger, &subject, &desc, &tools, &source, &enabled)
	if err == sql.ErrNoRows || classID != u.ClassID {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var toolList []string
	_ = json.Unmarshal([]byte(tools), &toolList)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "name": name, "trigger_text": trigger, "subject_text": subject,
		"description": desc, "tools": toolList, "source_text": source, "enabled": enabled == 1,
	})
}

func (s *Server) updateSkill(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var classID int64
	err := s.db.QueryRowContext(r.Context(), `SELECT class_id FROM skills WHERE id=?`, id).Scan(&classID)
	if err == sql.ErrNoRows || classID != u.ClassID {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	source, err := readSkillSource(r)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, "invalid_skill", err.Error())
		return
	}
	parsed, err := skillfile.Parse(source)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, "invalid_skill", err.Error())
		return
	}
	if err := (&mcphost.Host{DB: s.db}).ValidateSaved(r.Context(), u.ClassID, parsed.Tools); err != nil {
		writeDetail(w, http.StatusBadRequest, "invalid_skill", err.Error())
		return
	}
	tools, _ := json.Marshal(parsed.Tools)
	enabled := 0
	if parsed.Enabled {
		enabled = 1
	}
	_, err = s.db.ExecContext(r.Context(), `UPDATE skills SET name=?, trigger_text=?, description=?, tools_json=?, source_text=?, subject_text=?, enabled=? WHERE id=?`,
		parsed.Name, parsed.Trigger, parsed.Description, string(tools), parsed.Source, parsed.Subject, enabled, id)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeDetail(w, http.StatusBadRequest, "duplicate_name", "本班已经有同名技能")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM skills WHERE id=? AND class_id=?`, id, u.ClassID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id, c.title, c.agent_id, a.name, c.updated_at
		FROM conversations c JOIN tutor_agents a ON a.id=c.agent_id
		WHERE c.user_id=? AND c.class_id=?
		ORDER BY c.updated_at DESC, c.id DESC`, u.ID, u.ClassID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, agentID int64
		var title, agentName string
		var updated time.Time
		if err := rows.Scan(&id, &title, &agentID, &agentName, &updated); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		list = append(list, map[string]any{
			"id": id, "title": title, "agent_id": agentID, "agent_name": agentName,
			"updated_at": updated.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		AgentID int64 `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AgentID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var classID int64
	var enabled int
	err := s.db.QueryRowContext(r.Context(), `SELECT class_id, enabled FROM tutor_agents WHERE id=?`, body.AgentID).Scan(&classID, &enabled)
	if err == sql.ErrNoRows || classID != u.ClassID || enabled == 0 {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	var n int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tutor_agents WHERE class_id=? AND enabled=1`, u.ClassID).Scan(&n); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	if n == 0 {
		writeDetail(w, http.StatusBadRequest, "no_agent", "本班暂无可用的智能体")
		return
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(r.Context(), `INSERT INTO conversations (class_id, user_id, agent_id, title, created_at, updated_at) VALUES (?, ?, ?, '新的提问', ?, ?)`,
		u.ClassID, u.ID, body.AgentID, now, now)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "title": "新的提问", "agent_id": body.AgentID})
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	conv, ok := s.ownConversation(w, r, u)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, role, content, citations_json, skill_name, trace_json, created_at
		FROM messages WHERE conversation_id=? ORDER BY id ASC`, conv)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	msgs := []map[string]any{}
	for rows.Next() {
		var id int64
		var role, content string
		var citations, skill, trace sql.NullString
		var created time.Time
		if err := rows.Scan(&id, &role, &content, &citations, &skill, &trace, &created); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		item := map[string]any{
			"id": id, "role": role, "content": content, "created_at": created.UTC().Format(time.RFC3339),
		}
		if citations.Valid && citations.String != "" {
			var parsed any
			if json.Unmarshal([]byte(citations.String), &parsed) == nil {
				item["citations"] = parsed
			}
		}
		if skill.Valid {
			item["skill_name"] = skill.String
		}
		if trace.Valid && trace.String != "" {
			var parsed any
			if json.Unmarshal([]byte(trace.String), &parsed) == nil {
				item["trace"] = parsed
			}
		}
		msgs = append(msgs, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": conv, "messages": msgs})
}

func (s *Server) postConversationMessage(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	conv, ok := s.ownConversation(w, r, u)
	if !ok {
		return
	}
	var body struct {
		Query   string `json:"query"`
		Content string `json:"content"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	query := strings.TrimSpace(body.Query)
	if query == "" {
		query = strings.TrimSpace(body.Content)
	}
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		writeError(w, http.StatusInternalServerError, "unavailable")
		return
	}
	runner := studio.Runner{
		DB:        s.db,
		Retr:      s.retr,
		Chat:      chat.New(s.cfg.ChatURL, s.cfg.ChatAPIKey, s.cfg.ChatModel),
		Host:      &mcphost.Host{DB: s.db},
		Model:     s.cfg.ChatModel,
		MaxRounds: s.cfg.ToolMaxRounds,
		MaxCalls:  s.cfg.ToolMaxCalls,
	}
	started := false
	err := runner.Run(r.Context(), studio.Input{
		ConversationID: conv, ClassID: u.ClassID, UserID: u.ID, Query: query,
	}, func(event string, data any) error {
		if !started {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})
	if err == nil {
		return
	}
	if !started {
		switch {
		case errors.Is(err, studio.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found")
		case errors.Is(err, studio.ErrBadQuery):
			writeDetail(w, http.StatusBadRequest, "invalid_query", "提问需要 2 到 500 个字")
		default:
			writeError(w, http.StatusServiceUnavailable, "unavailable")
		}
		return
	}
	raw, _ := json.Marshal(map[string]string{"message": "回答中断了"})
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", raw)
	flusher.Flush()
}

func (s *Server) listExchanges(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	conv, ok := s.ownConversation(w, r, u)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, round_no, request_json, response_json, created_at
		FROM llm_exchanges WHERE conversation_id=? AND user_id=? ORDER BY id ASC`, conv, u.ID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var round int
		var req, resp string
		var created time.Time
		if err := rows.Scan(&id, &round, &req, &resp, &created); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		item := map[string]any{"id": id, "round": round, "created_at": created.UTC().Format(time.RFC3339)}
		var parsed any
		if json.Unmarshal([]byte(req), &parsed) == nil {
			item["request"] = parsed
		} else {
			item["request"] = req
		}
		if json.Unmarshal([]byte(resp), &parsed) == nil {
			item["response"] = parsed
		} else {
			item["response"] = resp
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"exchanges": list})
}

func (s *Server) listMCPServers(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, base_url, connected, last_error, auth_token<>'' FROM mcp_servers WHERE class_id=? ORDER BY id`, u.ClassID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, base, last string
		var connected, has int
		if err := rows.Scan(&id, &name, &base, &connected, &last, &has); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		list = append(list, map[string]any{
			"id": id, "name": name, "base_url": base, "connected": connected == 1,
			"last_error": last, "has_token": has == 1,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": list})
}

func (s *Server) createMCPServer(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	var body struct {
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
		Token   string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	name := strings.TrimSpace(body.Name)
	base := strings.TrimRight(strings.TrimSpace(body.BaseURL), "/")
	token := strings.TrimSpace(body.Token)
	if token == "" {
		token = strings.TrimSpace(s.cfg.MCPAuthToken)
	}
	if name == "" || utf8.RuneCountInString(name) > 64 || base == "" || !strings.HasPrefix(base, "http") {
		writeDetail(w, http.StatusBadRequest, "invalid_request", "需要填写服务器名称和 http 地址")
		return
	}
	if token == "" {
		writeDetail(w, http.StatusBadRequest, "invalid_request", "需要填写令牌，或在服务端环境变量 MCP_AUTH_TOKEN 中配置")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `INSERT INTO mcp_servers (class_id, name, base_url, auth_token, connected, created_at) VALUES (?, ?, ?, ?, 0, ?)`,
		u.ClassID, name, base, token, time.Now().UTC())
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeDetail(w, http.StatusBadRequest, "duplicate_name", "本班已经有同名服务器")
			return
		}
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name, "base_url": base, "connected": false, "has_token": true})
}

func (s *Server) connectMCPServer(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := (&mcphost.Host{DB: s.db}).Connect(r.Context(), u.ClassID, id)
	if errors.Is(err, mcphost.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeDetail(w, http.StatusBadRequest, "connect_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) disconnectMCPServer(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := (&mcphost.Host{DB: s.db}).Disconnect(r.Context(), u.ClassID, id)
	if errors.Is(err, mcphost.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listMCPTools(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var classID int64
	err := s.db.QueryRowContext(r.Context(), `SELECT class_id FROM mcp_servers WHERE id=?`, id).Scan(&classID)
	if err == sql.ErrNoRows || classID != u.ClassID {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT name, description, input_schema FROM mcp_tools WHERE server_id=? ORDER BY name`, id)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var name, desc, schema string
		if err := rows.Scan(&name, &desc, &schema); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		var parsed any
		_ = json.Unmarshal([]byte(schema), &parsed)
		list = append(list, map[string]any{"name": name, "description": desc, "input_schema": parsed})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": list})
}

func (s *Server) deleteMCPServer(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := (&mcphost.Host{DB: s.db}).Delete(r.Context(), u.ClassID, id)
	if errors.Is(err, mcphost.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listMCPAudits(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireTeacher(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			writeDetail(w, http.StatusBadRequest, "invalid_request", "limit 需要是 1 到 200 的整数")
			return
		}
		limit = n
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id, c.ref_text, c.server_name, c.tool_name, c.status, c.detail,
		c.conversation_id, c.round_no, c.created_at, u.username
		FROM mcp_calls c LEFT JOIN users u ON u.id=c.user_id
		WHERE c.class_id=? ORDER BY c.id DESC LIMIT ?`, u.ClassID, limit)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, conv int64
		var ref, server, tool, status, detail string
		var round int
		var created time.Time
		var username sql.NullString
		if err := rows.Scan(&id, &ref, &server, &tool, &status, &detail, &conv, &round, &created, &username); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		list = append(list, map[string]any{
			"id": id, "ref": ref, "server": server, "tool": tool, "status": status, "detail": detail,
			"conversation_id": conv, "round": round,
			"username": username.String, "created_at": created.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"audits": list, "limit": limit})
}

func (s *Server) requireTeacher(w http.ResponseWriter, r *http.Request) (*User, bool) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return nil, false
	}
	if u.Role != "teacher" {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil, false
	}
	return u, true
}

func (s *Server) ownConversation(w http.ResponseWriter, r *http.Request, u *User) (int64, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return 0, false
	}
	var owner, classID int64
	err := s.db.QueryRowContext(r.Context(), `SELECT user_id, class_id FROM conversations WHERE id=?`, id).Scan(&owner, &classID)
	if err == sql.ErrNoRows || owner != u.ID || classID != u.ClassID {
		writeError(w, http.StatusNotFound, "not_found")
		return 0, false
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return 0, false
	}
	return id, true
}

func (s *Server) bindingsExist(r *http.Request, classID int64, skills, materials []int64) error {
	for _, id := range skills {
		var n int
		if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM skills WHERE id=? AND class_id=?`, id, classID).Scan(&n); err != nil {
			return errUnavailable
		}
		if n == 0 {
			return errMissing
		}
	}
	for _, id := range materials {
		var n int
		if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM materials WHERE id=? AND class_id=?`, id, classID).Scan(&n); err != nil {
			return errUnavailable
		}
		if n == 0 {
			return errMissing
		}
	}
	return nil
}

func (s *Server) idList(r *http.Request, query string, id int64) ([]int64, error) {
	rows, err := s.db.QueryContext(r.Context(), query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func replaceBindings(r *http.Request, tx *sql.Tx, agentID int64, skills, materials []int64) error {
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM tutor_agent_skills WHERE agent_id=?`, agentID); err != nil {
		return err
	}
	for _, id := range skills {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO tutor_agent_skills (agent_id, skill_id) VALUES (?, ?)`, agentID, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM tutor_agent_materials WHERE agent_id=?`, agentID); err != nil {
		return err
	}
	for _, id := range materials {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO tutor_agent_materials (agent_id, material_id) VALUES (?, ?)`, agentID, id); err != nil {
			return err
		}
	}
	return nil
}

func validateAgent(body agentBody) string {
	name := strings.TrimSpace(body.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "名称需要 1 到 64 个字"
	}
	if utf8.RuneCountInString(body.Prompt) > 2000 {
		return "提示词不能超过 2000 字"
	}
	return ""
}

func readSkillSource(r *http.Request) (string, error) {
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return "", fmt.Errorf("文件无法读取")
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			return "", fmt.Errorf("请上传技能文件")
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, 1<<20))
		if err != nil {
			return "", fmt.Errorf("文件无法读取")
		}
		return string(b), nil
	}
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("请提交技能全文")
	}
	if strings.TrimSpace(body.Source) == "" {
		return "", fmt.Errorf("技能全文不能为空")
	}
	return body.Source, nil
}

type skillScanner interface {
	Scan(dest ...any) error
}

func scanSkill(rows skillScanner) (map[string]any, error) {
	var id int64
	var name, trigger, subject, desc, tools string
	var enabled int
	if err := rows.Scan(&id, &name, &trigger, &subject, &desc, &tools, &enabled); err != nil {
		return nil, err
	}
	var toolList []string
	_ = json.Unmarshal([]byte(tools), &toolList)
	return map[string]any{
		"id": id, "name": name, "trigger_text": trigger, "subject_text": subject,
		"description": desc, "tools": toolList, "enabled": enabled == 1,
	}, nil
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not_found")
		return 0, false
	}
	return id, true
}

func writeDetail(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

func writeStatus(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errMissing):
		writeError(w, http.StatusNotFound, "not_found")
	default:
		writeError(w, http.StatusServiceUnavailable, "unavailable")
	}
}

var (
	errMissing     = errors.New("missing")
	errUnavailable = errors.New("unavailable")
)
