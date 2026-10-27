package httpapi

import (
	"encoding/json"

	"campusclaw/internal/retrieval"
	"campusclaw/internal/studio"
)

// 这些类型只用来生成接口文档里的入参和出参结构，字段和处理器实际读写的 JSON 一致。

type statusOut struct {
	Status string `json:"status" doc:"固定为 ok"`
}

type loginOut struct {
	Status    string `json:"status"`
	Token     string `json:"token" doc:"之后请求放在 Authorization: Bearer 后面"`
	TokenType string `json:"token_type" doc:"固定为 Bearer"`
}

type meOut struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role" doc:"teacher 或 student"`
	ClassID   int64  `json:"class_id"`
	ClassName string `json:"class_name"`
}

type materialItem struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

type materialsOut struct {
	Materials []materialItem `json:"materials"`
}

type materialOut struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ClassID   int64  `json:"class_id"`
	CreatedAt string `json:"created_at"`
	Body      string `json:"body" doc:"材料正文"`
}

type createdOut struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type materialCreated struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type agentItem struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Prompt  string `json:"prompt" doc:"角色提示词，留空时使用默认角色"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

type agentsOut struct {
	Agents []agentItem `json:"agents"`
}

type agentOut struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Prompt      string  `json:"prompt"`
	Enabled     bool    `json:"enabled"`
	SkillIDs    []int64 `json:"skill_ids"`
	MaterialIDs []int64 `json:"material_ids"`
}

type skillItem struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	TriggerText string   `json:"trigger_text" doc:"适用条件，逗号分隔"`
	SubjectText string   `json:"subject_text" doc:"资料范围，只保留标题含这个词的材料"`
	Description string   `json:"description" doc:"发给模型的技能说明"`
	Tools       []string `json:"tools" doc:"knowledge_search 或 工具名@服务器名"`
	Enabled     bool     `json:"enabled"`
}

type skillsOut struct {
	Skills []skillItem `json:"skills"`
}

type skillOut struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	TriggerText string   `json:"trigger_text"`
	SubjectText string   `json:"subject_text"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
	Enabled     bool     `json:"enabled"`
	SourceText  string   `json:"source_text" doc:"技能文件全文"`
}

type skillBody struct {
	Source string `json:"source" required:"true" doc:"技能文件全文：空行上面写名称、适用条件、资料、工具、启用，空行下面写说明"`
}

type convBody struct {
	AgentID int64 `json:"agent_id" required:"true" doc:"本班已启用的应用编号，会话之后一直用它"`
}

type convItem struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	AgentID   int64  `json:"agent_id"`
	AgentName string `json:"agent_name"`
	UpdatedAt string `json:"updated_at"`
}

type convsOut struct {
	Conversations []convItem `json:"conversations"`
}

type convCreated struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	AgentID int64  `json:"agent_id"`
}

type messageItem struct {
	ID        int64           `json:"id"`
	Role      string          `json:"role" doc:"user 或 assistant"`
	Content   string          `json:"content"`
	CreatedAt string          `json:"created_at"`
	Citations []retrieval.Hit `json:"citations"`
	SkillName string          `json:"skill_name"`
	Trace     studio.Trace    `json:"trace" doc:"这条回答调用模型的记录：每次的请求体和返回体，以及用到的工具"`
}

type convOut struct {
	ID       int64         `json:"id"`
	Messages []messageItem `json:"messages"`
}

type messageBody struct {
	Query string `json:"query" required:"true" doc:"提问，2 到 500 个字"`
}

type exchangeItem struct {
	ID        int64           `json:"id"`
	Round     int             `json:"round" doc:"这次提问里第几次请求模型"`
	Request   json.RawMessage `json:"request" doc:"实际发给 /chat/completions 的请求体"`
	Response  json.RawMessage `json:"response" doc:"模型接口原样返回的内容"`
	CreatedAt string          `json:"created_at"`
}

type exchangesOut struct {
	Exchanges []exchangeItem `json:"exchanges"`
}

type serverItem struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Connected bool   `json:"connected"`
	LastError string `json:"last_error" doc:"上次连接失败的原因"`
	HasToken  bool   `json:"has_token" doc:"是否已配置令牌；令牌本身从不返回"`
}

type serversOut struct {
	Servers []serverItem `json:"servers"`
}

type serverBody struct {
	Name    string `json:"name" required:"true" doc:"服务器名，技能里用 工具名@服务器名 引用"`
	BaseURL string `json:"base_url" required:"true" doc:"例如 http://mcp:8090"`
	Token   string `json:"token" doc:"留空时使用服务端环境变量 MCP_AUTH_TOKEN"`
}

type toolItem struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type toolsOut struct {
	Tools []toolItem `json:"tools"`
}

type auditItem struct {
	ID             int64  `json:"id"`
	Ref            string `json:"ref" doc:"工具名@服务器名"`
	Server         string `json:"server"`
	Tool           string `json:"tool"`
	Status         string `json:"status" doc:"ok、denied 或 error"`
	Detail         string `json:"detail"`
	ConversationID int64  `json:"conversation_id"`
	Round          int    `json:"round"`
	Username       string `json:"username"`
	CreatedAt      string `json:"created_at"`
}

type auditsOut struct {
	Audits []auditItem `json:"audits"`
	Limit  int         `json:"limit"`
}
