package webapp

import "strings"
import "testing"

func TestComposeDiscoverCmdNoInnerSingleQuotes(t *testing.T) {
	cmd := composeDiscoverCmd("/opt /home", 50, 8)
	if strings.Contains(cmd, "-name '") || strings.Contains(cmd, "grep -q '") {
		t.Fatal("aspas simples no script quebram shellQuote no sh remoto")
	}
}

func TestComposeValidateCmdUsesDockerStackConfig(t *testing.T) {
	cmd := composeValidateCmd("services:\n  web:\n    image: nginx\n", "swarm")
	if !strings.Contains(cmd, "docker stack config") {
		t.Fatal("swarm deve usar docker stack config")
	}
	if !strings.Contains(cmd, shellQuote("swarm")) {
		t.Fatal("modo swarm deve ir no script remoto")
	}
}

func TestComposeValidateCmdUsesDockerComposeConfig(t *testing.T) {
	cmd := composeValidateCmd("services:\n  web:\n    image: nginx\n", "compose")
	if !strings.Contains(cmd, "docker compose") {
		t.Fatal("compose deve usar docker compose config")
	}
}
