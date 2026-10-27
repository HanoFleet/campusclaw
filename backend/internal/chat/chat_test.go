package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPayloadAddsToolsOnlyWhenGiven(t *testing.T) {
	msgs := []Message{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}}
	plain := Payload("m", msgs, nil)
	if _, ok := plain["tools"]; ok {
		t.Fatal("no tools expected")
	}
	if _, ok := plain["tool_choice"]; ok {
		t.Fatal("no tool_choice expected")
	}
	withTools := Payload("m", msgs, []ToolDef{{Name: "unit_convert", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}})
	if withTools["tool_choice"] != "auto" {
		t.Fatal(withTools["tool_choice"])
	}
	b, _ := json.Marshal(withTools)
	if !strings.Contains(string(b), `"type":"function"`) || !strings.Contains(string(b), `"temperature":0`) {
		t.Fatal(string(b))
	}
}

func TestLocalConvertUsesNumberFromQuestion(t *testing.T) {
	tools := []ToolDef{{Name: "unit_convert"}}
	comp, err := Local{}.Round(context.Background(), []Message{{Role: "user", Content: "3 米是多少厘米"}}, tools)
	if err != nil || len(comp.ToolCalls) != 1 {
		t.Fatal(err, comp)
	}
	if !strings.Contains(comp.ToolCalls[0].Arguments, `"value":3`) {
		t.Fatal(comp.ToolCalls[0].Arguments)
	}
	if !strings.Contains(string(comp.Raw), `"choices"`) {
		t.Fatal(string(comp.Raw))
	}
}

func TestPickAnswerMagnet(t *testing.T) {
	got := pickAnswer("磁铁能吸木块吗", []string{
		"《A班-小学科学课程.md》切片0：水在零摄氏度结成冰。磁铁能吸铁钉，不能吸木块。本资料发给 A 班科学课使用。",
	})
	if !strings.Contains(got, "磁铁") || !strings.Contains(got, "木块") {
		t.Fatalf("got %q", got)
	}
}
