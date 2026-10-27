package mcphost

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"campusclaw/internal/chat"
)

type BoundTool struct {
	Ref string
	Def chat.ToolDef
}

type CallStatus struct {
	Ref    string `json:"ref"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Text   string `json:"-"`
}

type Host struct {
	DB *sql.DB
}

// Scope 是一次调用的归属：哪个班、谁触发、在哪次对话里。
type Scope struct {
	ClassID        int64
	UserID         int64
	ConversationID int64
}

func (h *Host) Prepare(ctx context.Context, sc Scope, refs []string) ([]BoundTool, []CallStatus, error) {
	var tools []BoundTool
	var denied []CallStatus
	seen := map[string]bool{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || ref == "knowledge_search" || seen[ref] {
			continue
		}
		seen[ref] = true
		name, server, ok := splitRef(ref)
		if !ok {
			denied = append(denied, CallStatus{Ref: ref, Status: "denied", Detail: "引用格式不对"})
			continue
		}
		row, err := h.serverByName(ctx, sc.ClassID, server)
		if err == sql.ErrNoRows {
			denied = append(denied, CallStatus{Ref: ref, Status: "denied", Detail: "服务器不存在"})
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if row.connected == 0 {
			denied = append(denied, CallStatus{Ref: ref, Status: "denied", Detail: "服务器未连接"})
			if err := h.audit(ctx, sc, server, name, ref, "denied", "服务器未连接", 0); err != nil {
				return nil, nil, err
			}
			continue
		}
		var desc, schema string
		err = h.DB.QueryRowContext(ctx, `SELECT description, input_schema FROM mcp_tools WHERE server_id=? AND name=?`, row.id, name).Scan(&desc, &schema)
		if err == sql.ErrNoRows {
			denied = append(denied, CallStatus{Ref: ref, Status: "denied", Detail: "工具不在发现清单里"})
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(schema) == "" {
			schema = `{"type":"object","properties":{}}`
		}
		tools = append(tools, BoundTool{
			Ref: ref,
			Def: chat.ToolDef{Name: name, Description: desc, Parameters: json.RawMessage(schema)},
		})
	}
	return tools, denied, nil
}

func (h *Host) Invoke(ctx context.Context, sc Scope, ref string, args json.RawMessage, round int) CallStatus {
	name, server, ok := splitRef(ref)
	if !ok {
		return CallStatus{Ref: ref, Status: "denied", Detail: "引用格式不对", Text: "这个工具引用无法识别。"}
	}
	row, err := h.serverByName(ctx, sc.ClassID, server)
	if err == sql.ErrNoRows {
		return CallStatus{Ref: ref, Status: "denied", Detail: "服务器不存在", Text: "这台工具服务器已经不存在。"}
	}
	if err != nil {
		return CallStatus{Ref: ref, Status: "error", Detail: "查询服务器失败", Text: "暂时查不到这台工具服务器。"}
	}
	if row.connected == 0 {
		_ = h.audit(ctx, sc, server, name, ref, "denied", "服务器未连接", round)
		return CallStatus{Ref: ref, Status: "denied", Detail: "服务器未连接", Text: "服务器未连接，这次不能调用。"}
	}
	var schema string
	err = h.DB.QueryRowContext(ctx, `SELECT input_schema FROM mcp_tools WHERE server_id=? AND name=?`, row.id, name).Scan(&schema)
	if err != nil {
		_ = h.audit(ctx, sc, server, name, ref, "denied", "工具不在清单里", round)
		return CallStatus{Ref: ref, Status: "denied", Detail: "工具不在清单里", Text: "工具不在最近一次发现的清单里。"}
	}
	if msg := checkArgs(schema, args); msg != "" {
		_ = h.audit(ctx, sc, server, name, ref, "error", msg, round)
		return CallStatus{Ref: ref, Status: "error", Detail: msg, Text: msg}
	}
	text, err := NewClient(row.baseURL, row.token).CallTool(ctx, name, args)
	if err != nil {
		detail := clip(err.Error(), 180)
		_ = h.audit(ctx, sc, server, name, ref, "error", detail, round)
		return CallStatus{Ref: ref, Status: "error", Detail: detail, Text: "工具调用失败：" + detail}
	}
	text = guardToolText(text)
	_ = h.audit(ctx, sc, server, name, ref, "ok", clip(text, 180), round)
	return CallStatus{Ref: ref, Status: "ok", Detail: clip(text, 180), Text: text}
}

// Refuse 记下一次被宿主拦下、没有真正发出去的调用。
func (h *Host) Refuse(ctx context.Context, sc Scope, ref, reason string, round int) CallStatus {
	name, server, ok := splitRef(ref)
	if ok {
		_ = h.audit(ctx, sc, server, name, ref, "denied", reason, round)
	}
	return CallStatus{Ref: ref, Status: "denied", Detail: reason, Text: reason + "，这次没有执行。"}
}

func (h *Host) ValidateSaved(ctx context.Context, classID int64, tools []string) error {
	for _, ref := range tools {
		if ref == "knowledge_search" {
			continue
		}
		name, server, ok := splitRef(ref)
		if !ok {
			return fmt.Errorf("工具写法不对：%s", ref)
		}
		row, err := h.serverByName(ctx, classID, server)
		if err == sql.ErrNoRows {
			return fmt.Errorf("服务器 %s 不存在，或还没有连接", server)
		}
		if err != nil {
			return err
		}
		if row.connected == 0 {
			return fmt.Errorf("服务器 %s 还没有连接，不能绑定 %s", server, ref)
		}
		var n int
		if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_tools WHERE server_id=? AND name=?`, row.id, name).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("工具 %s 不在 %s 最近一次发现的清单里", name, server)
		}
	}
	return nil
}

type serverRow struct {
	id        int64
	baseURL   string
	token     string
	connected int
}

func (h *Host) serverByName(ctx context.Context, classID int64, name string) (serverRow, error) {
	var row serverRow
	err := h.DB.QueryRowContext(ctx, `SELECT id, base_url, auth_token, connected FROM mcp_servers WHERE class_id=? AND name=?`, classID, name).
		Scan(&row.id, &row.baseURL, &row.token, &row.connected)
	return row, err
}

func (h *Host) audit(ctx context.Context, sc Scope, server, tool, ref, status, detail string, round int) error {
	_, err := h.DB.ExecContext(ctx, `INSERT INTO mcp_calls
		(class_id, user_id, conversation_id, server_name, tool_name, ref_text, status, detail, round_no, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sc.ClassID, sc.UserID, sc.ConversationID, server, tool, ref, status, clip(detail, 500), round, time.Now().UTC())
	return err
}

func splitRef(ref string) (tool, server string, ok bool) {
	tool, server, ok = strings.Cut(ref, "@")
	if !ok || tool == "" || server == "" || strings.Contains(tool, "@") {
		return "", "", false
	}
	return tool, server, true
}

// guardToolText 在结果回传给模型之前做检查：像指令的整条丢弃，过长的截断。
func guardToolText(text string) string {
	lower := strings.ToLower(text)
	for _, p := range instructionPatterns {
		if strings.Contains(lower, p) {
			return "工具返回的内容像是在下指令，已经丢弃，不当作资料使用。"
		}
	}
	return clip(text, toolTextRunes)
}

const toolTextRunes = 2000

var instructionPatterns = []string{
	"忽略以上", "忽略之前", "忽略前面", "你现在是", "从现在起你", "请扮演", "系统提示",
	"ignore previous", "ignore all previous", "system prompt", "you are now",
}

func (h *Host) Connect(ctx context.Context, classID, serverID int64) error {
	var base, token string
	err := h.DB.QueryRowContext(ctx, `SELECT base_url, auth_token FROM mcp_servers WHERE id=? AND class_id=?`, serverID, classID).Scan(&base, &token)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	client := NewClient(base, token)
	fail := func(msg string) error {
		_, _ = h.DB.ExecContext(ctx, `UPDATE mcp_servers SET connected=0, last_error=? WHERE id=?`, clip(msg, 255), serverID)
		_, _ = h.DB.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id=?`, serverID)
		return fmt.Errorf("%s", msg)
	}
	if err := client.Initialize(ctx); err != nil {
		return fail("连接失败：" + err.Error())
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return fail("连接失败：" + err.Error())
	}
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id=?`, serverID); err != nil {
		return err
	}
	for _, tool := range tools {
		schema := string(tool.InputSchema)
		if strings.TrimSpace(schema) == "" {
			schema = `{"type":"object","properties":{}}`
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_tools (server_id, class_id, name, description, input_schema) VALUES (?, ?, ?, ?, ?)`,
			serverID, classID, tool.Name, tool.Description, schema); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mcp_servers SET connected=1, last_error='' WHERE id=?`, serverID); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Host) Disconnect(ctx context.Context, classID, serverID int64) error {
	if err := h.owned(ctx, classID, serverID); err != nil {
		return err
	}
	if _, err := h.DB.ExecContext(ctx, `UPDATE mcp_servers SET connected=0, last_error='' WHERE id=?`, serverID); err != nil {
		return err
	}
	_, err := h.DB.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id=?`, serverID)
	return err
}

// Delete 删掉服务器和它的工具清单。之后引用它的技能在提问时记为被拒，调用无处归属，不写记录。
func (h *Host) Delete(ctx context.Context, classID, serverID int64) error {
	if err := h.owned(ctx, classID, serverID); err != nil {
		return err
	}
	if _, err := h.DB.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id=?`, serverID); err != nil {
		return err
	}
	_, err := h.DB.ExecContext(ctx, `DELETE FROM mcp_servers WHERE id=?`, serverID)
	return err
}

func (h *Host) owned(ctx context.Context, classID, serverID int64) error {
	var owner int64
	err := h.DB.QueryRowContext(ctx, `SELECT class_id FROM mcp_servers WHERE id=?`, serverID).Scan(&owner)
	if err == sql.ErrNoRows || (err == nil && owner != classID) {
		return ErrNotFound
	}
	return err
}

var ErrNotFound = fmt.Errorf("not found")

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
