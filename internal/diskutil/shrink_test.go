package diskutil

import (
	"strings"
	"testing"
)

func TestShrinkLVScript_ext4(t *testing.T) {
	s := ShrinkLVScript("/dev/mapper/vg-lv", 2, "ext4")
	if s == "" {
		t.Fatal("empty script")
	}
	for _, want := range []string{"lvreduce", "--resizefs", "-L -2G", "/dev/mapper/vg-lv"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestShrinkLVScript_btrfs(t *testing.T) {
	s := ShrinkLVScript("/dev/vg/lv", 1.5, "btrfs")
	for _, want := range []string{"btrfs filesystem resize", "lvreduce", `-"${GIB}"G`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestShrinkLVScript_xfsUnsupported(t *testing.T) {
	s := ShrinkLVScript("/dev/vg/lv", 2, "xfs")
	if !strings.Contains(s, "não suportada") {
		t.Fatalf("expected unsupported message, got %s", s)
	}
}
