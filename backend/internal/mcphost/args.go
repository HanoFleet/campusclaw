package mcphost

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type argSchema struct {
	Properties map[string]struct {
		Type string `json:"type"`
		Enum []any  `json:"enum"`
	} `json:"properties"`
	Required             []string `json:"required"`
	AdditionalProperties *bool    `json:"additionalProperties"`
}

// checkArgs 按工具发现时拿到的同一份输入结构，核对模型填的参数。
// 返回空串表示通过，否则是给模型和记录看的可读原因。
func checkArgs(schema string, args json.RawMessage) string {
	raw := strings.TrimSpace(string(args))
	if raw == "" {
		raw = "{}"
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil || obj == nil {
		return "参数必须是 JSON 对象"
	}
	var sc argSchema
	if err := json.Unmarshal([]byte(schema), &sc); err != nil {
		return "工具的输入结构无法解析"
	}
	for _, key := range sc.Required {
		v, ok := obj[key]
		if !ok || v == nil || v == "" {
			return "缺少参数 " + key
		}
	}
	for key, v := range obj {
		prop, known := sc.Properties[key]
		if !known {
			if len(sc.Properties) > 0 && (sc.AdditionalProperties == nil || !*sc.AdditionalProperties) {
				return "多了输入结构里没有的参数 " + key
			}
			continue
		}
		if prop.Type != "" && !typeMatches(prop.Type, v) {
			return fmt.Sprintf("参数 %s 应该是 %s", key, typeName(prop.Type))
		}
		if len(prop.Enum) > 0 && !inEnum(prop.Enum, v) {
			return fmt.Sprintf("参数 %s 只能取 %s", key, enumText(prop.Enum))
		}
	}
	return ""
}

func typeMatches(t string, v any) bool {
	switch t {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	}
	return true
}

func typeName(t string) string {
	switch t {
	case "string":
		return "文字"
	case "number":
		return "数字"
	case "integer":
		return "整数"
	case "boolean":
		return "真或假"
	case "object":
		return "对象"
	case "array":
		return "列表"
	}
	return t
}

func inEnum(enum []any, v any) bool {
	for _, e := range enum {
		if fmt.Sprint(e) == fmt.Sprint(v) {
			return true
		}
	}
	return false
}

func enumText(enum []any) string {
	parts := make([]string, 0, len(enum))
	for _, e := range enum {
		parts = append(parts, fmt.Sprint(e))
	}
	return strings.Join(parts, "、")
}
