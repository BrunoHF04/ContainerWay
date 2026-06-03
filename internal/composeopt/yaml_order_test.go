package composeopt

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const stackWithCommentedService = `
services:
    libreofficesiplan_228:
        image: 'libre:latest'
        deploy:
            mode: global
            resources:
                limits:
                    memory: 1G
                reservations:
                    memory: 512M
    # orionpro:
         #image: 'orionpro:1'
         #deploy:
         #    mode: global
    mysql:
        image: 'mysql:5.7'
        deploy:
            mode: global
            resources:
                limits:
                    memory: 1G
`

func TestMarshalPreservesCommentsBetweenServices(t *testing.T) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(stackWithCommentedService), &root); err != nil {
		t.Fatal(err)
	}
	services := root["services"].(map[string]any)
	lo := services["libreofficesiplan_228"].(map[string]any)
	deploy := lo["deploy"].(map[string]any)
	res := deploy["resources"].(map[string]any)
	limits := res["limits"].(map[string]any)
	limits["cpus"] = "0.86"
	reservations := res["reservations"].(map[string]any)
	reservations["cpus"] = "0.43"

	out, err := MarshalPreservingOrder(stackWithCommentedService, root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "reservations:\n") && strings.Contains(out, "# orionpro:") {
		// comment must not be nested under reservations
		idxRes := strings.Index(out, "reservations:")
		idxComment := strings.Index(out, "# orionpro:")
		idxCpus := strings.Index(out, "cpus:")
		if idxComment > idxRes && idxComment < idxCpus {
			t.Fatalf("comentário dentro de reservations:\n%s", out)
		}
	}
	if !strings.Contains(out, "# orionpro:") {
		t.Fatalf("comentário orionpro perdido:\n%s", out)
	}
	if !strings.Contains(out, "cpus:") {
		t.Fatalf("cpus não aplicado:\n%s", out)
	}
	var check map[string]any
	if err := yaml.Unmarshal([]byte(out), &check); err != nil {
		t.Fatalf("yaml inválido: %v\n%s", err, out)
	}
}

func TestNormalizeYAMLInputStripsNBSP(t *testing.T) {
	in := "services:\n\u00a0 \n  app:\n    image: x\n"
	out := NormalizeYAMLInput(in)
	if strings.Contains(out, "\u00a0") {
		t.Fatal("NBSP deve ser removido")
	}
}

func TestAnalyzeStackFragmentValidYAML(t *testing.T) {
	content, err := os.ReadFile("testdata/stack_fragment.yml")
	if err != nil {
		t.Fatal(err)
	}
	host := HostSpec{CPUs: 6, MemTotal: 16 * 1024 * 1024 * 1024}
	res, err := Analyze("/opt/siplan/docker-stack-orion.yml", string(content), host, AnalyzeOptions{
		Mode: ModeSwarm, SwarmActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Optimized, "reservations:") {
		lines := strings.Split(res.Optimized, "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "reservations:" {
				for j := i + 1; j < len(lines) && j < i+6; j++ {
					trim := strings.TrimSpace(lines[j])
					if strings.HasPrefix(trim, "#") && strings.Contains(trim, "orionpro") {
						t.Fatalf("comentário orionpro dentro de reservations (linha %d):\n%s", j+1, res.Optimized)
					}
					if trim != "" && !strings.HasPrefix(trim, "#") && !strings.Contains(trim, ":") {
						break
					}
				}
			}
		}
	}
	if strings.Contains(res.Optimized, "\u00a0") {
		t.Fatal("YAML com espaços NBSP")
	}
	var check map[string]any
	if err := yaml.Unmarshal([]byte(res.Optimized), &check); err != nil {
		t.Fatalf("yaml inválido: %v\n%s", err, res.Optimized)
	}
	if !strings.Contains(res.Optimized, "# orionpro:") {
		t.Fatalf("comentário orionpro perdido")
	}
}
