package diskutil

import (
	"strings"
	"testing"
)

func TestNormalizeLVDevicePath_mapperFix(t *testing.T) {
	in := "/dev/ubuntu--vg-ubuntu--lv"
	want := "/dev/mapper/ubuntu--vg-ubuntu--lv"
	got := NormalizeLVDevicePath(in, nil)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeLVDevicePath_fromLVS(t *testing.T) {
	records := []LVRecord{{
		Path:   "/dev/ubuntu-vg/ubuntu-lv",
		VG:     "ubuntu-vg",
		LVName: "ubuntu-lv",
	}}
	got := NormalizeLVDevicePath("/dev/ubuntu--vg-ubuntu--lv", records)
	if got != "/dev/ubuntu-vg/ubuntu-lv" {
		t.Fatalf("got %q", got)
	}
}

func TestMapperPathFromVG(t *testing.T) {
	got := MapperPathFromVG("ubuntu-vg", "ubuntu-lv")
	want := "/dev/mapper/ubuntu--vg-ubuntu--lv"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestShrinkLVScript_resolvesPath(t *testing.T) {
	s := ShrinkLVScript("/dev/ubuntu--vg-ubuntu--lv", 1, "ext4")
	if !strings.Contains(s, "resolve_lv_device") {
		t.Fatal("missing resolve_lv_device")
	}
}
