// Package openapi 在注册路由的同时记下接口说明，并据此生成 OpenAPI 文档。
// 文档和路由来自同一处代码：漏写说明的路由也会出现在文档里，改了路径文档跟着变。
package openapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Op 是写在路由旁边的接口说明。
type Op struct {
	Summary string
	Tag     string
	// Auth 表示需要登录；Teacher 表示只有教师能调，学生得到 403。
	Auth    bool
	Teacher bool
	Query   []Param
	// Body 是请求体的样例值，按它的类型生成结构；Upload 表示 multipart 上传文件。
	Body   any
	Upload string
	// Result 是成功时响应体的样例值；Status 默认 200。
	Result any
	Status int
	// Stream 表示响应是 text/event-stream，Events 写明有哪些事件。
	Stream bool
	Events string
	// File 表示响应是文件下载。
	File bool
	// Errors 是这个接口可能返回的错误：状态码 → 说明。
	Errors map[int]string
}

type Param struct {
	Name     string
	Type     string
	Desc     string
	Required bool
}

type Doc struct {
	mux     *http.ServeMux
	title   string
	version string
	desc    string

	mu    sync.Mutex
	paths map[string]map[string]any
}

// Mount 先挂上 /openapi.json 和 /swagger，之后通过 Handle 注册的业务路由都会进文档。
func Mount(mux *http.ServeMux, title, version, desc string) *Doc {
	d := &Doc{mux: mux, title: title, version: version, desc: desc, paths: map[string]map[string]any{}}
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(d.Spec())
	})
	mux.HandleFunc("GET /swagger", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerPage))
	})
	return d
}

var pathParam = regexp.MustCompile(`\{([a-zA-Z_]+)\}`)

// Handle 注册路由并记下说明。pattern 的写法和 http.ServeMux 一样，例如 "POST /api/login"。
func (d *Doc) Handle(pattern string, h http.HandlerFunc, op Op) {
	d.mux.HandleFunc(pattern, h)
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		panic("openapi: pattern needs a method: " + pattern)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.paths[path] == nil {
		d.paths[path] = map[string]any{}
	}
	d.paths[path][strings.ToLower(method)] = operation(path, op)
}

func operation(path string, op Op) map[string]any {
	out := map[string]any{"summary": op.Summary}
	if op.Tag != "" {
		out["tags"] = []string{op.Tag}
	}
	var params []map[string]any
	for _, m := range pathParam.FindAllStringSubmatch(path, -1) {
		params = append(params, map[string]any{
			"name": m[1], "in": "path", "required": true, "schema": map[string]string{"type": "integer"},
		})
	}
	for _, q := range op.Query {
		t := q.Type
		if t == "" {
			t = "string"
		}
		params = append(params, map[string]any{
			"name": q.Name, "in": "query", "required": q.Required, "description": q.Desc, "schema": map[string]string{"type": t},
		})
	}
	if len(params) > 0 {
		out["parameters"] = params
	}
	switch {
	case op.Upload != "":
		out["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{
				"type":       "object",
				"required":   []string{op.Upload},
				"properties": map[string]any{op.Upload: map[string]string{"type": "string", "format": "binary"}},
			}}},
		}
	case op.Body != nil:
		out["requestBody"] = map[string]any{
			"required": true,
			"content":  map[string]any{"application/json": map[string]any{"schema": Schema(op.Body)}},
		}
	}

	responses := map[string]any{}
	status := op.Status
	if status == 0 {
		status = http.StatusOK
	}
	ok := map[string]any{"description": "成功"}
	switch {
	case op.Stream:
		ok["description"] = "SSE 事件流：" + op.Events
		ok["content"] = map[string]any{"text/event-stream": map[string]any{"schema": map[string]string{"type": "string"}}}
	case op.File:
		ok["description"] = "文件内容"
		ok["content"] = map[string]any{"application/octet-stream": map[string]any{"schema": map[string]string{"type": "string", "format": "binary"}}}
	case op.Result != nil:
		ok["content"] = map[string]any{"application/json": map[string]any{"schema": Schema(op.Result)}}
	}
	responses[strconv.Itoa(status)] = ok
	errs := map[int]string{}
	for k, v := range op.Errors {
		errs[k] = v
	}
	if op.Auth || op.Teacher {
		out["security"] = []map[string][]string{{"bearer": {}}}
		if _, set := errs[http.StatusUnauthorized]; !set {
			errs[http.StatusUnauthorized] = "unauthorized：没有登录或令牌失效"
		}
	}
	if op.Teacher {
		if _, set := errs[http.StatusForbidden]; !set {
			errs[http.StatusForbidden] = "forbidden：只有教师能调用"
		}
	}
	for code, text := range errs {
		responses[strconv.Itoa(code)] = map[string]any{
			"description": text,
			"content":     map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/Error"}}},
		}
	}
	out["responses"] = responses
	return out
}

// Spec 返回完整的 OpenAPI 3 文档。
func (d *Doc) Spec() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	keys := make([]string, 0, len(d.paths))
	for k := range d.paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	paths := map[string]any{}
	for _, k := range keys {
		paths[k] = d.paths[k]
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]string{"title": d.title, "version": d.version, "description": d.desc},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearer": map[string]string{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
			},
			"schemas": map[string]any{
				"Error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error":   map[string]string{"type": "string", "description": "错误码"},
						"message": map[string]string{"type": "string", "description": "可读的原因，部分错误才有"},
					},
				},
			},
		},
		"paths": paths,
	}
}

// Paths 列出文档里所有的「方法 路径」，用于测试路由和文档是否一致。
func (d *Doc) Paths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for p, ops := range d.paths {
		for m := range ops {
			out = append(out, strings.ToUpper(m)+" "+p)
		}
	}
	sort.Strings(out)
	return out
}

// Schema 按样例值的 Go 类型生成 JSON Schema。字段说明写在结构体的 doc 标签里。
func Schema(v any) map[string]any {
	return schemaOf(reflect.TypeOf(v), 0)
}

var rawMessage = reflect.TypeOf(json.RawMessage(nil))

func schemaOf(t reflect.Type, depth int) map[string]any {
	if t == nil || depth > 6 {
		return map[string]any{}
	}
	if t == rawMessage {
		return map[string]any{"type": "object"}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return schemaOf(t.Elem(), depth)
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaOf(t.Elem(), depth+1)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaOf(t.Elem(), depth+1)}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		props := map[string]any{}
		var required []string
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			s := schemaOf(f.Type, depth+1)
			if d := f.Tag.Get("doc"); d != "" {
				s["description"] = d
			}
			props[name] = s
			if f.Tag.Get("required") == "true" && !strings.Contains(opts, "omitempty") {
				required = append(required, name)
			}
		}
		out := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			out["required"] = required
		}
		return out
	}
	return map[string]any{}
}

const swaggerPage = `<!doctype html>
<meta charset="utf-8">
<title>CampusClaw API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
<div id="swagger"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({ url: "/openapi.json", dom_id: "#swagger", persistAuthorization: true })
</script>`
