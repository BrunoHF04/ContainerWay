package diskutil

import (
	"errors"
	"strings"
	"testing"
)

func TestInterpretLVCmdError_notReduced(t *testing.T) {
	msg := InterpretLVCmdError(
		"Logical volume vg/lv not reduced.\n",
		"",
		errors.New("Process exited with status 3"),
	)
	if !strings.Contains(msg, "demasiado pequeno") {
		t.Fatalf("got %q", msg)
	}
}

func TestInterpretLVCmdError_resize2fs(t *testing.T) {
	msg := InterpretLVCmdError(
		"resize2fs: On-line shrinking required\n",
		"",
		errors.New("exit status 1"),
	)
	if !strings.Contains(msg, "sistema de arquivos") {
		t.Fatalf("got %q", msg)
	}
}

func TestInterpretLVCmdError_lastLine(t *testing.T) {
	msg := InterpretLVCmdError("first\n", "last error line\n", nil)
	if msg != "last error line" {
		t.Fatalf("got %q", msg)
	}
}
