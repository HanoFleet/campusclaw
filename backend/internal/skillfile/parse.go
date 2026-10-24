package skillfile

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// File 是教师上传的技能文本解析结果。空行以上是字段，空行以下是说明。
type File struct {
	Name        string
	Trigger     string
	Subject     string
	Tools       []string
	Enabled     bool
	Description string
	Source      string
}

var toolRef = regexp.MustCompile(`^[a-z][a-z0-9_]*@[A-Za-z0-9][A-Za-z0-9_-]*$`)

func Parse(src string) (File, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.TrimPrefix(src, "\uFEFF")
	if utf8.RuneCountInString(src) > 8000 {
		return File{}, fmt.Errorf("技能文件不能超过 8000 字")
	}
	meta, body, ok := strings.Cut(src, "\n\n")
	if !ok {
		return File{}, fmt.Errorf("技能文件缺少空行，空行下面才是说明")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return File{}, fmt.Errorf("空行下面缺少说明")
	}
	if utf8.RuneCountInString(body) > 2000 {
		return File{}, fmt.Errorf("说明不能超过 2000 字")
	}

	seen := map[string]bool{}
	fields := map[string]string{}
	for _, line := range strings.Split(meta, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, found := cutField(line)
		if !found {
			return File{}, fmt.Errorf("无法识别这一行：%s", line)
		}
		switch key {
		case "名称", "适用条件", "资料", "工具", "启用":
		default:
			return File{}, fmt.Errorf("不认识的字段：%s", key)
		}
		if seen[key] {
			return File{}, fmt.Errorf("字段重复：%s", key)
		}
		seen[key] = true
		fields[key] = strings.TrimSpace(val)
	}
	for _, key := range []string{"名称", "适用条件", "启用"} {
		if strings.TrimSpace(fields[key]) == "" {
			return File{}, fmt.Errorf("缺少字段：%s", key)
		}
	}
	name := fields["名称"]
	if utf8.RuneCountInString(name) > 32 {
		return File{}, fmt.Errorf("名称不能超过 32 字")
	}
	trigger := fields["适用条件"]
	if utf8.RuneCountInString(trigger) > 120 {
		return File{}, fmt.Errorf("适用条件不能超过 120 字")
	}
	subject := fields["资料"]
	if utf8.RuneCountInString(subject) > 32 {
		return File{}, fmt.Errorf("资料不能超过 32 字")
	}
	enabled, err := parseEnabled(fields["启用"])
	if err != nil {
		return File{}, err
	}
	tools, err := parseTools(fields["工具"])
	if err != nil {
		return File{}, err
	}
	return File{
		Name:        name,
		Trigger:     trigger,
		Subject:     subject,
		Tools:       tools,
		Enabled:     enabled,
		Description: body,
		Source:      src,
	}, nil
}

func cutField(line string) (string, string, bool) {
	if i := strings.Index(line, "："); i > 0 {
		return strings.TrimSpace(line[:i]), line[i+len("："):], true
	}
	if i := strings.Index(line, ":"); i > 0 {
		return strings.TrimSpace(line[:i]), line[i+1:], true
	}
	return "", "", false
}

func parseEnabled(v string) (bool, error) {
	switch strings.TrimSpace(v) {
	case "是", "true", "启用":
		return true, nil
	case "否", "false", "停用":
		return false, nil
	default:
		return false, fmt.Errorf("启用只能填「是」或「否」")
	}
}

func parseTools(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"knowledge_search"}, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == '|' || r == '｜'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p != "knowledge_search" && !toolRef.MatchString(p) {
			return nil, fmt.Errorf("工具写法不对：%s。内置检索写 knowledge_search，外部工具写 工具名@服务器名", p)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("工具不能为空")
	}
	return out, nil
}

func SplitTrigger(trigger string) []string {
	parts := strings.FieldsFunc(trigger, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == '|' || r == '｜'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
