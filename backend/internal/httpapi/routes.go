package httpapi

import (
	"net/http"

	"campusclaw/internal/openapi"
	"campusclaw/internal/retrieval"
)

const (
	notFound    = "not_found：不存在、不属于本班或不属于自己，响应完全一样"
	unavailable = "unavailable：依赖的服务暂时不可用"
)

// routes 注册全部接口。每条路由旁边的说明会生成 /openapi.json，所以这里就是接口文档。
func (s *Server) routes() {
	d := openapi.Mount(s.mux, "CampusClaw", "0.6.0",
		"按班级组织的学习助手：登录、本班材料、检索、应用、技能、流式会话和 MCP 工具。除 /health 和 /api/login 外都要带 Bearer 令牌。")

	d.Handle("GET /health", s.health, openapi.Op{
		Tag: "系统", Summary: "存活检查，不需要登录", Result: statusOut{},
	})

	d.Handle("POST /api/login", s.login, openapi.Op{
		Tag: "登录", Summary: "用账号密码换取 Bearer 令牌，同时写入会话 Cookie",
		Body: loginRequest{}, Result: loginOut{},
		Errors: map[int]string{
			400: "invalid_request：请求体不是 JSON",
			401: "invalid_credentials：账号或密码不对；连续失败会暂时锁定",
		},
	})
	d.Handle("POST /api/logout", s.logout, openapi.Op{
		Tag: "登录", Summary: "退出登录，令牌立即失效", Auth: true, Result: statusOut{},
	})
	d.Handle("GET /api/me", s.me, openapi.Op{
		Tag: "登录", Summary: "当前用户和所在班级", Auth: true, Result: meOut{},
	})

	d.Handle("GET /api/materials", s.listMaterials, openapi.Op{
		Tag: "材料", Summary: "列出本班材料", Auth: true, Result: materialsOut{},
		Query: []openapi.Param{{Name: "q", Desc: "按标题筛选"}},
	})
	d.Handle("POST /api/materials", s.uploadMaterial, openapi.Op{
		Tag: "材料", Summary: "教师上传 .md 或 .txt 材料，入库后切片并建索引", Teacher: true,
		Upload: "file", Result: materialCreated{}, Status: http.StatusCreated,
		Errors: map[int]string{
			400: "invalid_file：文件类型或编码不对",
			413: "payload_too_large：超过大小上限",
		},
	})
	d.Handle("GET /api/materials/{id}", s.getMaterial, openapi.Op{
		Tag: "材料", Summary: "读取一份本班材料的正文", Auth: true, Result: materialOut{},
		Errors: map[int]string{404: notFound},
	})
	d.Handle("GET /api/materials/{id}/file", s.getMaterialFile, openapi.Op{
		Tag: "材料", Summary: "下载材料原文件", Auth: true, File: true,
		Errors: map[int]string{404: notFound},
	})
	d.Handle("POST /api/materials/{id}/reindex", s.reindex, openapi.Op{
		Tag: "材料", Summary: "教师按新的切片参数重建这份材料的索引", Teacher: true,
		Body: reindexRequest{}, Result: statusOut{},
		Errors: map[int]string{404: notFound},
	})

	d.Handle("POST /api/search", s.search, openapi.Op{
		Tag: "检索", Summary: "在本班材料里检索；班级取自登录用户，请求体里的 class_id 不起作用", Auth: true,
		Body: searchRequest{}, Result: retrieval.SearchResult{},
		Errors: map[int]string{400: "invalid_query：检索词为空", 503: unavailable},
	})
	d.Handle("POST /api/ask", s.ask, openapi.Op{
		Tag: "检索", Summary: "单次知识问答：检索本班材料后作答并标出处", Auth: true,
		Body: askRequest{}, Result: retrieval.AskResult{},
		Errors: map[int]string{400: "invalid_query：提问为空", 503: unavailable},
	})

	d.Handle("GET /api/agents", s.listAgents, openapi.Op{
		Tag: "应用", Summary: "列出本班应用；学生只看到已启用的", Auth: true, Result: agentsOut{},
	})
	d.Handle("POST /api/agents", s.createAgent, openapi.Op{
		Tag: "应用", Summary: "教师新建应用，设置提示词并绑定本班技能和材料", Teacher: true,
		Body: agentBody{}, Result: createdOut{}, Status: http.StatusCreated,
		Errors: map[int]string{
			400: "invalid_request / duplicate_name：名称 1 到 64 字、提示词不超过 2000 字、班内名称不能重复",
			404: "not_found：绑定了不存在或他班的技能、材料",
		},
	})
	d.Handle("GET /api/agents/{id}", s.getAgent, openapi.Op{
		Tag: "应用", Summary: "查看一个应用和它绑定的技能、材料；学生看不到已停用的", Auth: true, Result: agentOut{},
		Errors: map[int]string{404: notFound},
	})
	d.Handle("PUT /api/agents/{id}", s.updateAgent, openapi.Op{
		Tag: "应用", Summary: "教师修改应用的名称、提示词、启用状态和绑定", Teacher: true,
		Body: agentBody{}, Result: statusOut{},
		Errors: map[int]string{400: "invalid_request / duplicate_name", 404: notFound},
	})

	d.Handle("GET /api/skills", s.listSkills, openapi.Op{
		Tag: "技能", Summary: "列出本班技能", Auth: true, Result: skillsOut{},
	})
	d.Handle("POST /api/skills", s.createSkill, openapi.Op{
		Tag: "技能", Summary: "教师提交技能全文；也可以用 multipart 上传 file 字段", Teacher: true,
		Body: skillBody{}, Result: createdOut{}, Status: http.StatusCreated,
		Errors: map[int]string{400: "invalid_skill / duplicate_name：字段缺失或重复、工具引用不对、引用的服务器未连接或工具不在清单里、班内重名"},
	})
	d.Handle("GET /api/skills/{id}", s.getSkill, openapi.Op{
		Tag: "技能", Summary: "查看一条技能和它的文件全文", Auth: true, Result: skillOut{},
		Errors: map[int]string{404: notFound},
	})
	d.Handle("PUT /api/skills/{id}", s.updateSkill, openapi.Op{
		Tag: "技能", Summary: "教师用新的全文替换技能；把「启用」改成「否」即停用", Teacher: true,
		Body: skillBody{}, Result: statusOut{},
		Errors: map[int]string{400: "invalid_skill / duplicate_name", 404: notFound},
	})
	d.Handle("DELETE /api/skills/{id}", s.deleteSkill, openapi.Op{
		Tag: "技能", Summary: "教师删除技能，绑定它的应用自动解绑", Teacher: true, Result: statusOut{},
		Errors: map[int]string{404: notFound},
	})

	d.Handle("GET /api/conversations", s.listConversations, openapi.Op{
		Tag: "会话", Summary: "只列出自己的会话", Auth: true, Result: convsOut{},
	})
	d.Handle("POST /api/conversations", s.createConversation, openapi.Op{
		Tag: "会话", Summary: "选定一个本班已启用的应用新建会话", Auth: true,
		Body: convBody{}, Result: convCreated{}, Status: http.StatusCreated,
		Errors: map[int]string{400: "invalid_request：没有给出 agent_id", 404: "not_found：应用不存在、已停用或不属于本班"},
	})
	d.Handle("GET /api/conversations/{id}", s.getConversation, openapi.Op{
		Tag: "会话", Summary: "读取自己的会话和全部消息", Auth: true, Result: convOut{},
		Errors: map[int]string{404: notFound},
	})
	d.Handle("POST /api/conversations/{id}/messages", s.postConversationMessage, openapi.Op{
		Tag: "会话", Summary: "发送一条提问，以 SSE 流式返回回答", Auth: true, Body: messageBody{},
		Stream: true, Events: "tool（每次工具调用的引用和状态，先于正文）、meta（技能、引用列表、工具汇总）、delta（逐段正文）、done（消息编号和调用记录）、error（中途失败）",
		Errors: map[int]string{400: "invalid_query：提问需要 2 到 500 个字", 404: notFound, 503: unavailable},
	})
	d.Handle("GET /api/conversations/{id}/exchanges", s.listExchanges, openapi.Op{
		Tag: "会话", Summary: "这个会话里每次调用模型的请求体和返回内容", Auth: true, Result: exchangesOut{},
		Errors: map[int]string{404: notFound},
	})

	d.Handle("GET /api/mcp/servers", s.listMCPServers, openapi.Op{
		Tag: "工具", Summary: "列出本班登记的 MCP 服务器，只回是否配置了令牌", Auth: true, Result: serversOut{},
	})
	d.Handle("POST /api/mcp/servers", s.createMCPServer, openapi.Op{
		Tag: "工具", Summary: "教师登记 MCP 服务器；令牌只存在服务端", Teacher: true,
		Body: serverBody{}, Result: serverItem{}, Status: http.StatusCreated,
		Errors: map[int]string{400: "invalid_request / duplicate_name：缺名称或地址、没有令牌、班内重名"},
	})
	d.Handle("POST /api/mcp/servers/{id}/connect", s.connectMCPServer, openapi.Op{
		Tag: "工具", Summary: "连接：握手 initialize 后用 tools/list 发现工具，整体替换清单", Teacher: true, Result: statusOut{},
		Errors: map[int]string{400: "connect_failed：握手或发现失败，message 里是原因，服务器停在未连接", 404: notFound},
	})
	d.Handle("POST /api/mcp/servers/{id}/disconnect", s.disconnectMCPServer, openapi.Op{
		Tag: "工具", Summary: "断开：清空工具清单，之后的调用一律被拒", Teacher: true, Result: statusOut{},
		Errors: map[int]string{404: notFound},
	})
	d.Handle("DELETE /api/mcp/servers/{id}", s.deleteMCPServer, openapi.Op{
		Tag: "工具", Summary: "删除服务器和它的工具清单", Teacher: true, Result: statusOut{},
		Errors: map[int]string{404: notFound},
	})
	d.Handle("GET /api/mcp/servers/{id}/tools", s.listMCPTools, openapi.Op{
		Tag: "工具", Summary: "最近一次发现的工具：名称、说明、输入结构", Auth: true, Result: toolsOut{},
		Errors: map[int]string{404: notFound},
	})
	d.Handle("GET /api/mcp/audits", s.listMCPAudits, openapi.Op{
		Tag: "工具", Summary: "本班最近的工具调用记录，按时间倒序", Teacher: true, Result: auditsOut{},
		Query:  []openapi.Param{{Name: "limit", Type: "integer", Desc: "条数，默认 50，最多 200"}},
		Errors: map[int]string{400: "invalid_request：limit 不在 1 到 200 之间"},
	})
}
