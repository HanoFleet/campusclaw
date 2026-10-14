package chat

import (
	"strings"
	"testing"
)

func TestPickAnswerMagnet(t *testing.T) {
	got := pickAnswer("磁铁能吸木块吗", []string{
		"《A班-小学科学课程.md》切片0：水在零摄氏度结成冰。磁铁能吸铁钉，不能吸木块。本资料发给 A 班科学课使用。",
	})
	if !strings.Contains(got, "磁铁") || !strings.Contains(got, "木块") {
		t.Fatalf("got %q", got)
	}
}
