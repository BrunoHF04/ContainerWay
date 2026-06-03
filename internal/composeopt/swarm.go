package composeopt

import (
	"fmt"
)

func replicaCount(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}

func analyzeSwarmExtras(name string, svc, optSvc map[string]any, host HostSpec, swarmNodes int, findings *[]Finding) {
	deploy, _ := svc["deploy"].(map[string]any)
	if deploy == nil {
		optDeploy := map[string]any{}
		optSvc["deploy"] = optDeploy
		deploy = optDeploy
		*findings = append(*findings, Finding{
			Service: name, Severity: SeveritySuggest, Category: "swarm",
			Message:   "Swarm exige secção deploy por serviço (replicas, resources, update_config).",
			DocRef:    "https://docs.docker.com/compose/compose-file/deploy/",
			Suggested: "deploy com replicas e resources",
		})
	}
	optDeploy, _ := optSvc["deploy"].(map[string]any)
	if optDeploy == nil {
		optDeploy = map[string]any{}
		optSvc["deploy"] = optDeploy
	}

	if isDeployGlobal(deploy) {
		if _, ok := optDeploy["replicas"]; ok {
			delete(optDeploy, "replicas")
		}
		if _, ok := deploy["replicas"]; ok {
			*findings = append(*findings, Finding{
				Service: name, Severity: SeverityInfo, Category: "swarm",
				Message:   "deploy.mode: global — replicas é ignorado (uma tarefa por nó).",
				DocRef:    "https://docs.docker.com/compose/compose-file/deploy/#mode",
			})
		}
	} else if _, ok := deploy["replicas"]; !ok {
		optDeploy["replicas"] = 1
		*findings = append(*findings, Finding{
			Service: name, Severity: SeveritySuggest, Category: "swarm",
			Message:   "Sem replicas — em modo replicated defina quantas tarefas executar.",
			DocRef:    "https://docs.docker.com/compose/compose-file/deploy/#replicas",
			Current:   "—",
			Suggested: "1",
		})
	} else if reps := replicaCount(deploy["replicas"]); reps > 1 && swarmNodes > 0 && reps > swarmNodes {
		suggest := swarmNodes
		if suggest < 1 {
			suggest = 1
		}
		optDeploy["replicas"] = suggest
		*findings = append(*findings, Finding{
			Service: name, Severity: SeverityWarn, Category: "swarm",
			Message:   fmt.Sprintf("Replicas (%d) superiores aos nós Swarm (%d).", reps, swarmNodes),
			DocRef:    "https://docs.docker.com/engine/swarm/services/",
			Current:   fmt.Sprintf("%d", reps),
			Suggested: fmt.Sprintf("%d", suggest),
		})
	}

	if _, ok := deploy["restart_policy"]; !ok {
		optDeploy["restart_policy"] = map[string]any{
			"condition": "on-failure",
			"delay":     "5s",
			"max_attempts": 3,
		}
		*findings = append(*findings, Finding{
			Service: name, Severity: SeverityInfo, Category: "swarm",
			Message:   "restart_policy em deploy recomendado (restart no nível do serviço é ignorado no Swarm).",
			DocRef:    "https://docs.docker.com/compose/compose-file/deploy/#restart_policy",
			Suggested: "on-failure",
		})
	}

	if _, ok := deploy["update_config"]; !ok {
		optDeploy["update_config"] = map[string]any{
			"parallelism":       1,
			"delay":             "10s",
			"failure_action":    "rollback",
			"order":             "start-first",
		}
		*findings = append(*findings, Finding{
			Service: name, Severity: SeverityInfo, Category: "swarm",
			Message:   "update_config ausente — atualizações podem derrubar todos os réplicas de uma vez.",
			DocRef:    "https://docs.docker.com/compose/compose-file/deploy/#update_config",
			Suggested: "parallelism: 1, failure_action: rollback",
		})
	}

	if swarmNodes >= 2 {
		if place, _ := deploy["placement"].(map[string]any); place == nil {
			optDeploy["placement"] = map[string]any{
				"preferences": []any{
					map[string]any{"spread": "node.labels.zone"},
				},
			}
			*findings = append(*findings, Finding{
				Service: name, Severity: SeverityInfo, Category: "swarm",
				Message:   "Cluster com vários nós — considere placement.constraints ou preferences.",
				DocRef:    "https://docs.docker.com/compose/compose-file/deploy/#placement",
			})
		}
	}

	// restart no topo é ignorado no Swarm
	if r, ok := svc["restart"].(string); ok && r != "" {
		delete(optSvc, "restart")
		*findings = append(*findings, Finding{
			Service: name, Severity: SeverityInfo, Category: "swarm",
			Message:   "Campo restart no serviço é ignorado no Swarm — use deploy.restart_policy.",
			DocRef:    "https://docs.docker.com/compose/compose-file/deploy/#restart_policy",
			Current:   r,
		})
	}

	// cpus/mem_limit no topo não aplicam em Swarm — resources em deploy
	if serviceUsesClassicResources(svc) {
		*findings = append(*findings, Finding{
			Service: name, Severity: SeverityInfo, Category: "swarm",
			Message:   "mem_limit/cpus no serviço são ignorados no Swarm — use deploy.resources.",
			DocRef:    "https://docs.docker.com/compose/compose-file/deploy/#resources",
		})
	}
}
