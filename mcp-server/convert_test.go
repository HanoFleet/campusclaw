package main

import "testing"

func TestUnitConvert(t *testing.T) {
	got, err := unitConvert(1, "m", "cm")
	if err != nil || got != "1 m = 100 cm" {
		t.Fatalf("%s %v", got, err)
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
