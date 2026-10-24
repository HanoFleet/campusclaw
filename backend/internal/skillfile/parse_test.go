package skillfile

import "testing"

func TestParseGuide(t *testing.T) {
	src := "名称：数学解题引导\n适用条件：方程,去括号\n资料：数学\n工具：knowledge_search\n启用：是\n\n先看卡在哪一步，不要算出得数。\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "数学解题引导" || !f.Enabled || f.Subject != "数学" || f.Tools[0] != "knowledge_search" {
		t.Fatalf("%+v", f)
	}
}

func TestParseMCPRef(t *testing.T) {
	src := "名称：单位换算\n适用条件：换算|厘米\n工具：unit_convert@campus-tools\n启用：是\n\n先调用工具，再说明换算结果。\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tools[0] != "unit_convert@campus-tools" {
		t.Fatal(f.Tools)
	}
}

func TestRejectBadTool(t *testing.T) {
	src := "名称：坏工具\n适用条件：测试\n工具：Formula-Hint\n启用：是\n\n说明。\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectDuplicate(t *testing.T) {
	src := "名称：甲\n名称：乙\n适用条件：题\n启用：是\n\n说明。\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected error")
	}
}
