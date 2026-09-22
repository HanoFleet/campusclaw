package materials

import "testing"

func TestAllowedExt(t *testing.T) {
	if _, ok := AllowedExt("notes.md"); !ok {
		t.Fatal("md")
	}
	if _, ok := AllowedExt("notes.MD"); !ok {
		t.Fatal("MD")
	}
	if _, ok := AllowedExt("notes.md.exe"); ok {
		t.Fatal("double ext must use the final suffix")
	}
	if _, ok := AllowedExt("../x.exe"); ok {
		t.Fatal("exe")
	}
}

func TestDisplayTitleStripsPath(t *testing.T) {
	if got := DisplayTitle(`..\..\secret.md`); got != "secret.md" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateText(t *testing.T) {
	if err := ValidateText([]byte("  ")); err == nil {
		t.Fatal("blank")
	}
	if err := ValidateText([]byte{0xff, 0xfe}); err == nil {
		t.Fatal("binary")
	}
	if err := ValidateText([]byte("你好")); err != nil {
		t.Fatal(err)
	}
}
