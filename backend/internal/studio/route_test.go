package studio

import "testing"

func TestRoutePicksHigherScore(t *testing.T) {
	skills := []Skill{
		{ID: 1, Name: "数学解题引导", Trigger: "方程,去括号,面积", Description: "只引导", Enabled: true},
		{ID: 2, Name: "公式提示", Trigger: "公式,面积", Description: "只提示公式", Enabled: true},
		{ID: 3, Name: "科学观察", Trigger: "磁铁", Description: "科学", Enabled: true},
	}
	hit := Route(skills, "矩形面积该用什么公式")
	if hit.Skill == nil || hit.Skill.Name != "公式提示" || hit.Score != 2 {
		t.Fatalf("%+v", hit)
	}
}

func TestRouteIgnoresUnboundByNotPassingThem(t *testing.T) {
	skills := []Skill{{ID: 8, Name: "语文解题引导", Trigger: "阅读", Enabled: false}}
	hit := Route(skills, "这篇阅读的中心是什么")
	if hit.Skill != nil {
		t.Fatal("disabled skill should not win")
	}
}

func TestTieKeepsSmallerID(t *testing.T) {
	skills := []Skill{
		{ID: 2, Name: "甲", Trigger: "方程", Enabled: true},
		{ID: 9, Name: "乙", Trigger: "方程", Enabled: true},
	}
	hit := Route(skills, "方程")
	if hit.Skill.Name != "甲" {
		t.Fatal(hit.Skill.Name)
	}
}
