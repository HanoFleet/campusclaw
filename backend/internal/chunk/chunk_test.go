package chunk

import (
	"strings"
	"testing"
)

func titledDoc() string {
	return "# 第一章 函数\n\n当自变量增大时函数值增大，称为单调递增。\n\n## 第二节 应用\n\n这一节讲几个课堂例题。\n"
}

func TestAutoDefaultWindow(t *testing.T) {
	got := Split(titledDoc(), Options{})
	if len(got) == 0 {
		t.Fatal("empty")
	}
	again := Split(titledDoc(), Options{Strategy: "auto"})
	if len(got) != len(again) {
		t.Fatalf("unspecified should match auto: %d vs %d", len(got), len(again))
	}
}

func TestStrategiesDifferOnTitledDoc(t *testing.T) {
	long := titledDoc() + strings.Repeat("课堂例题补充说明。", 20)
	autoN := len(Split(long, Options{Strategy: StrategyAuto}))
	hierN := len(Split(long, Options{Strategy: StrategyHierarchy}))
	customN := len(Split(long, Options{Strategy: StrategyCustom, MaxLen: 100, OverlapRatio: 0}))
	if autoN == hierN && hierN == customN {
		t.Fatalf("expected different counts, all %d", autoN)
	}
	if customN < 2 {
		t.Fatalf("custom 100-char window should yield more slices, got %d", customN)
	}
}

func TestOffsetsOnSource(t *testing.T) {
	src := "ABCDEFGHIJ"
	got := Split(src, Options{Strategy: StrategyCustom, MaxLen: 100, OverlapRatio: 0})
	if len(got) != 1 || got[0].Text != src || got[0].Start != 0 || got[0].End != 10 {
		t.Fatalf("%+v", got)
	}
}

func TestPreprocessDoesNotChangeCaller(t *testing.T) {
	src := "见 https://example.com 空   白"
	_ = Split(src, Options{Strategy: StrategyCustom, MaxLen: 100, StripURL: true, CollapseSpace: true})
	if src != "见 https://example.com 空   白" {
		t.Fatal("caller string changed")
	}
}
