package studio

import (
	"strings"
	"testing"

	"campusclaw/internal/chat"
	"campusclaw/internal/retrieval"
)

func TestTurnMessagesSendsOnlyPickedSkillAndHits(t *testing.T) {
	skill := &Skill{Name: "数学解题引导", Description: "只指出下一步。", Tools: []string{"knowledge_search"}}
	history := []chat.Message{{Role: "user", Content: "去括号怎么做"}}
	hits := []retrieval.Hit{{Title: "A班-数学自学笔记", ChunkIndex: 2, Excerpt: "括号前面的数要乘到每一项。"}}
	msgs := TurnMessages("你是数学助教。", skill, history, "去括号怎么做", hits)
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("%+v", msgs)
	}
	sys := msgs[0].Content
	for _, want := range []string{"你是数学助教。", "当前技能：数学解题引导", "只指出下一步。", "不要写出最终得数"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("system missing %q: %s", want, sys)
		}
	}
	if !strings.Contains(msgs[1].Content, "[1] 材料《A班-数学自学笔记》切片 2") {
		t.Fatal(msgs[1].Content)
	}
}

func TestSkillWithoutKnowledgeSearchSkipsMaterials(t *testing.T) {
	skill := &Skill{Name: "单位换算", Description: "调用工具。", Tools: []string{"unit_convert@campus-tools"}}
	if NeedsSearch(skill) {
		t.Fatal("should not search")
	}
	if !NeedsSearch(nil) {
		t.Fatal("no skill should still search")
	}
	msgs := TurnMessages("", skill, nil, "3 米是多少厘米", nil)
	if msgs[1].Content != "3 米是多少厘米" {
		t.Fatal(msgs[1].Content)
	}
	if !strings.Contains(msgs[0].Content, DefaultPrompt) {
		t.Fatal("default prompt missing")
	}
}
