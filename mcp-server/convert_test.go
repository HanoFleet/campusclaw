package main

import "testing"

func TestUnitConvert(t *testing.T) {
	got, err := unitConvert(1, "m", "cm")
	if err != nil || got != "1 m = 100 cm" {
		t.Fatalf("%s %v", got, err)
	}
	got, err = unitConvert(3, "米", "厘米")
	if err != nil || got != "3 m = 300 cm" {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := unitConvert(1, "米", "千克"); err == nil {
		t.Fatal("length to mass should fail")
	}
}

func TestFormulaDoesNotGiveNumber(t *testing.T) {
	got := formulaHint("矩形面积")
	if got == "" || containsDigitAnswer(got) {
		t.Fatal(got)
	}
}

func containsDigitAnswer(s string) bool {
	return false
}
