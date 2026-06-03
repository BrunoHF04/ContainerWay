package diskutil

import (
	"math"
	"strings"
	"testing"
)

func TestUserGBToLVMG(t *testing.T) {
	got := UserGBToLVMG(10)
	want := 10 * float64(bytesPerDecimalGB) / float64(bytesPerLVMG)
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestShrinkLVScript_ext4_usesLVMConversion(t *testing.T) {
	s := ShrinkLVScript("/dev/mapper/vg-lv", 2, "ext4")
	lvm := FormatLVMSizeG(UserGBToLVMG(2))
	if !strings.Contains(s, "-L -"+lvm+"G") {
		t.Fatalf("expected -L -%sG in %s", lvm, s)
	}
}
