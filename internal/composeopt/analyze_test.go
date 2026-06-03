package composeopt

import (
	"strings"
	"testing"
)

func TestAnalyzeJavaMemory(t *testing.T) {
	yml := `
services:
  api:
    image: eclipse-temurin:21-jre
    environment:
      JAVA_OPTS: "-Xmx2048m"
`
	host := HostSpec{CPUs: 4, MemTotal: 8 * 1024 * 1024 * 1024}
	res, err := Analyze("/opt/app/compose.yml", yml, host, AnalyzeOptions{Mode: ModeCompose})
	if err != nil {
		t.Fatal(err)
	}
	if res.ServiceCount != 1 {
		t.Fatalf("services=%d", res.ServiceCount)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected findings")
	}
	if res.Optimized == "" {
		t.Fatal("expected optimized yaml")
	}
}

func TestParseBytes(t *testing.T) {
	cases := []struct{ in string; want int64 }{
		{"512M", 512 * 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"2G", 2 * 1024 * 1024 * 1024},
	}
	for _, c := range cases {
		got, ok := ParseBytes(c.in)
		if !ok || got != c.want {
			t.Fatalf("%s: got %d ok=%v", c.in, got, ok)
		}
	}
}

func TestAnalyzeSwarmReplicas(t *testing.T) {
	yml := `
services:
  web:
    image: nginx:alpine
    deploy:
      replicas: 99
      resources:
        limits:
          memory: 128M
`
	host := HostSpec{CPUs: 4, MemTotal: 4 * 1024 * 1024 * 1024}
	res, err := Analyze("/stack.yml", yml, host, AnalyzeOptions{Mode: ModeSwarm, SwarmActive: true, SwarmNodes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != ModeSwarm {
		t.Fatalf("mode=%s", res.Mode)
	}
}

func TestAnalyzeKeepsHighMemoryLimits(t *testing.T) {
	yml := `
services:
  postgresql:
    image: postgres:17
    deploy:
      mode: global
      resources:
        limits:
          memory: 4G
        reservations:
          memory: 2G
  app:
    image: nginx:alpine
`
	host := HostSpec{CPUs: 6, MemTotal: 16 * 1024 * 1024 * 1024}
	res, err := Analyze("/stack.yml", yml, host, AnalyzeOptions{Mode: ModeSwarm, SwarmActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Optimized, "memory: 4G") {
		t.Fatalf("não deve reduzir 4G:\n%s", res.Optimized)
	}
}

func TestAnalyzeJavaXmxExceedsLimit(t *testing.T) {
	yml := `
services:
  api:
    image: wildfly
    environment:
      JAVA_OPTS: "-Xmx4096M"
    deploy:
      resources:
        limits:
          memory: 2G
`
	host := HostSpec{CPUs: 4, MemTotal: 8 * 1024 * 1024 * 1024}
	res, err := Analyze("/stack.yml", yml, host, AnalyzeOptions{Mode: ModeSwarm})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Optimized, "Xmx4096M") {
		t.Fatalf("deve corrigir Xmx acima do limite:\n%s", res.Optimized)
	}
}

func TestGlobalModeNoReplicasAdded(t *testing.T) {
	yml := `
services:
  web:
    image: nginx:alpine
    deploy:
      mode: global
`
	host := HostSpec{CPUs: 4, MemTotal: 8 * 1024 * 1024 * 1024}
	res, err := Analyze("/stack.yml", yml, host, AnalyzeOptions{Mode: ModeSwarm})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Optimized, "replicas:") {
		t.Fatalf("global não deve ganhar replicas:\n%s", res.Optimized)
	}
}

func TestComposePatchesMemLimitNotDeploy(t *testing.T) {
	yml := `
services:
  web:
    image: nginx:alpine
`
	host := HostSpec{CPUs: 4, MemTotal: 8 * 1024 * 1024 * 1024}
	res, err := Analyze("/opt/app/compose.yml", yml, host, AnalyzeOptions{Mode: ModeCompose})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Optimized, "mem_limit:") {
		t.Fatalf("compose deve usar mem_limit:\n%s", res.Optimized)
	}
	if strings.Contains(res.Optimized, "deploy:") {
		t.Fatalf("compose simples não deve criar deploy:\n%s", res.Optimized)
	}
}

func TestComposeKeepsMemLimit(t *testing.T) {
	yml := `
services:
  db:
    image: postgres:17
    mem_limit: 2G
`
	host := HostSpec{CPUs: 4, MemTotal: 8 * 1024 * 1024 * 1024}
	res, err := Analyze("/opt/app/compose.yml", yml, host, AnalyzeOptions{Mode: ModeCompose})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Optimized, "mem_limit: 2G") {
		t.Fatalf("deve manter mem_limit:\n%s", res.Optimized)
	}
}

func TestSwarmPatchesDeployNotMemLimit(t *testing.T) {
	yml := `
services:
  web:
    image: nginx:alpine
    deploy:
      mode: global
`
	host := HostSpec{CPUs: 4, MemTotal: 8 * 1024 * 1024 * 1024}
	res, err := Analyze("/stack.yml", yml, host, AnalyzeOptions{Mode: ModeSwarm})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Optimized, "deploy:") || !strings.Contains(res.Optimized, "resources:") {
		t.Fatalf("swarm deve usar deploy.resources:\n%s", res.Optimized)
	}
	if strings.Contains(res.Optimized, "mem_limit:") {
		t.Fatalf("swarm não deve usar mem_limit:\n%s", res.Optimized)
	}
}

func TestMarshalPreservesUnchangedScalars(t *testing.T) {
	orig := "services:\n  app:\n    image: 'registry.local/app:1'\n    ports:\n      - target: 5432\n"
	updated := map[string]any{
		"services": map[string]any{
			"app": map[string]any{
				"image": "registry.local/app:1",
				"ports": []any{
					map[string]any{"target": 5432},
				},
			},
		},
	}
	out, err := MarshalPreservingOrder(orig, updated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "image: 'registry.local/app:1'") {
		t.Fatalf("aspas originais perdidas:\n%s", out)
	}
}

func TestMarshalPreservingOrder(t *testing.T) {
	orig := "services:\n  app:\n    image: x\nnetworks:\n  n:\n    external: true\n"
	updated := map[string]any{
		"services": map[string]any{
			"app": map[string]any{"image": "y"},
		},
		"networks": map[string]any{
			"n": map[string]any{"external": true},
		},
	}
	out, err := MarshalPreservingOrder(orig, updated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "services:") {
		t.Fatalf("ordem perdida:\n%s", out)
	}
	if !strings.Contains(out, "image: y") {
		t.Fatalf("valor não aplicado:\n%s", out)
	}
}

func TestParseDiscoverLines(t *testing.T) {
	files := ParseDiscoverLines("/opt/a/docker-compose.yml\n\n/opt/b/compose.yaml\n")
	if len(files) != 2 || files[0].Dir != "/opt/a" {
		t.Fatalf("%+v", files)
	}
}
