package studio

import (
	"strings"

	"campusclaw/internal/skillfile"
)

// Skill 是路由时用到的一行技能。
type Skill struct {
	ID          int64
	Name        string
	Trigger     string
	Subject     string
	Description string
	Tools       []string
	Enabled     bool
}

// RouteHit 记录这次为什么选中（或没选中）某个技能。
type RouteHit struct {
	Skill   *Skill
	Score   int
	Matched []string
	Note    string
}

// Route 只在已经绑定且启用的技能里计分，最多选一个。编号较小者先被遍历，同分不替换。
func Route(skills []Skill, query string) RouteHit {
	q := strings.ToLower(strings.TrimSpace(query))
	best := RouteHit{Note: "绑定的技能都没有对上适用条件，这次不使用技能，按智能体的提示词直接回答。"}
	for i := range skills {
		s := &skills[i]
		if !s.Enabled {
			continue
		}
		var matched []string
		for _, word := range skillfile.SplitTrigger(s.Trigger) {
			if strings.Contains(q, strings.ToLower(word)) {
				matched = append(matched, word)
			}
		}
		if len(matched) > best.Score {
			cp := *s
			best = RouteHit{Skill: &cp, Score: len(matched), Matched: matched}
		}
	}
	if best.Skill != nil {
		best.Note = "在这个智能体绑定且启用的技能里，按适用条件逐词比较提问，取得分最高的一条。"
		if strings.Contains(best.Skill.Name, "解题引导") {
			best.Note += "名称里有「解题引导」，所以会额外要求不写出最终得数。"
		}
	}
	return best
}
