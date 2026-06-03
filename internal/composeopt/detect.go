package composeopt

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Modos de análise.
const (
	ModeAuto    = "auto"
	ModeCompose = "compose"
	ModeSwarm   = "swarm"
)

// AnalyzeOptions parâmetros da análise.
type AnalyzeOptions struct {
	Mode        string
	SwarmActive bool
	SwarmNodes  int
}

// EffectiveMode resolve auto → compose ou swarm.
func (o AnalyzeOptions) EffectiveMode(content string) string {
	m := strings.ToLower(strings.TrimSpace(o.Mode))
	if m != ModeAuto && m != "" {
		if m == ModeSwarm {
			return ModeSwarm
		}
		return ModeCompose
	}
	if o.SwarmActive && fileLooksSwarm(content) {
		return ModeSwarm
	}
	if fileLooksSwarm(content) {
		return ModeSwarm
	}
	return ModeCompose
}

func fileLooksSwarm(content string) bool {
	low := strings.ToLower(content)
	if strings.Contains(low, "replicas:") && strings.Contains(low, "deploy:") {
		return true
	}
	if strings.Contains(low, "placement:") && strings.Contains(low, "deploy:") {
		return true
	}
	if strings.Contains(low, "docker stack") {
		return true
	}
	return false
}

// ClassifyFileKind estima o tipo pelo conteúdo ou nome.
func ClassifyFileKind(path, content string) string {
	base := strings.ToLower(path)
	if strings.Contains(base, "stack") || strings.Contains(base, "swarm") {
		return "swarm"
	}
	if content != "" && fileLooksSwarm(content) {
		return "swarm"
	}
	var root map[string]any
	if err := yaml.Unmarshal([]byte(content), &root); err == nil {
		if services, _ := root["services"].(map[string]any); services != nil {
			for _, raw := range services {
				svc, _ := raw.(map[string]any)
				if deploy, _ := svc["deploy"].(map[string]any); deploy != nil {
					if _, ok := deploy["replicas"]; ok {
						return "swarm"
					}
					if _, ok := deploy["placement"]; ok {
						return "swarm"
					}
				}
			}
		}
	}
	return "compose"
}
