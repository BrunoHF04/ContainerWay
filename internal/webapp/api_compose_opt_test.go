package webapp

import "strings"
import "testing"

func TestComposeDiscoverCmdNoInnerSingleQuotes(t *testing.T) {
	cmd := composeDiscoverCmd("/opt /home", 50, 8)
	if strings.Contains(cmd, "-name '") || strings.Contains(cmd, "grep -q '") {
		t.Fatal("aspas simples no script quebram shellQuote no sh remoto")
	}
}
