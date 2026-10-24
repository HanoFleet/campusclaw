package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	token := strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN"))
	if token == "" {
		log.Fatal("MCP_AUTH_TOKEN is required")
	}
	addr := os.Getenv("MCP_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r, token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		handleMCP(w, r)
	})
	log.Printf("mcp listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func authorized(r *http.Request, token string) bool {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	scheme, got, ok := strings.Cut(h, " ")
	return ok && strings.EqualFold(scheme, "Bearer") && got == token
}

func handleMCP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeRPCError(w, nil, -32700, "请求读不出来")
		return
	}
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		writeRPCError(w, req.ID, -32600, "这不是一条合法的 JSON-RPC 请求")
		return
	}
	switch req.Method {
	case "initialize":
		writeRPC(w, req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "campus-tools", "version": "1.0"},
		})
	case "tools/list":
		writeRPC(w, req.ID, map[string]any{"tools": toolList()})
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeRPCError(w, req.ID, -32602, "tools/call 缺少参数")
			return
		}
		text, callErr := callTool(params.Name, params.Arguments)
		if callErr != nil {
			writeRPC(w, req.ID, map[string]any{
				"content": []map[string]string{{"type": "text", "text": callErr.Error()}},
				"isError": true,
			})
			return
		}
		writeRPC(w, req.ID, map[string]any{
			"content": []map[string]string{{"type": "text", "text": text}},
			"isError": false,
		})
	default:
		writeRPCError(w, req.ID, -32601, "不支持的方法："+req.Method)
	}
}

func toolList() []map[string]any {
	return []map[string]any{
		{
			"name":        "formula_hint",
			"description": "学生问该用哪一类公式时使用。只返回检查步骤，不计算最终得数。不要在只是闲聊时使用。参数 topic 是题目里的关键词。",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topic": map[string]string{"type": "string", "description": "题目里出现的关键词，例如面积或行程"},
				},
				"required": []string{"topic"},
			},
		},
		{
			"name":        "unit_convert",
			"description": "把长度或质量换成另一个单位。只在用户明确要求换算米、厘米、毫米、千米、克、千克时使用。value 是数字，from 和 to 是单位符号。",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]string{"type": "number"},
					"from":  map[string]string{"type": "string", "description": "m、cm、mm、km、g、kg"},
					"to":    map[string]string{"type": "string"},
				},
				"required": []string{"value", "from", "to"},
			},
		},
	}
}

func callTool(name string, args json.RawMessage) (string, error) {
	switch name {
	case "formula_hint":
		var in struct {
			Topic string `json:"topic"`
		}
		if err := json.Unmarshal(args, &in); err != nil || strings.TrimSpace(in.Topic) == "" {
			return "", fmt.Errorf("缺少 topic")
		}
		return formulaHint(in.Topic), nil
	case "unit_convert":
		var in struct {
			Value float64 `json:"value"`
			From  string  `json:"from"`
			To    string  `json:"to"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("参数不是对象")
		}
		return unitConvert(in.Value, in.From, in.To)
	default:
		return "", fmt.Errorf("没有这个工具")
	}
}

func formulaHint(topic string) string {
	switch {
	case strings.Contains(topic, "面积"):
		return "先确认是不是矩形，再核对长和宽是不是同一个单位。面积等于长乘宽。这里只提示步骤，不给得数。"
	case strings.Contains(topic, "行程"), strings.Contains(topic, "速度"):
		return "先写出路程、速度、时间里哪一个未知。这里只提示步骤，不给得数。"
	default:
		return "先把已知量和所求量分开，再判断该用哪一种关系。这里只提示步骤，不给得数。题目关键词：" + strings.TrimSpace(topic)
	}
}

func unitConvert(value float64, from, to string) (string, error) {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	scale := map[string]float64{"mm": 0.001, "cm": 0.01, "m": 1, "km": 1000, "g": 1, "kg": 1000}
	length := map[string]bool{"mm": true, "cm": true, "m": true, "km": true}
	a, ok1 := scale[from]
	b, ok2 := scale[to]
	if !ok1 || !ok2 || length[from] != length[to] {
		return "", fmt.Errorf("只支持米、厘米、毫米、千米之间，或克与千克之间")
	}
	out := value * a / b
	return fmt.Sprintf("%g %s = %g %s", value, from, out, to), nil
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"error":   map[string]any{"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
