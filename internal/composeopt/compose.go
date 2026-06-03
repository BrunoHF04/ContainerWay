package composeopt

// analyzeComposeExtras regras só para Docker Compose (não Swarm).
func analyzeComposeExtras(name string, svc, optSvc map[string]any, findings *[]Finding) {
	deploy, _ := svc["deploy"].(map[string]any)
	if deploy != nil {
		if _, ok := deploy["replicas"]; ok {
			*findings = append(*findings, Finding{
				Service: name, Severity: SeverityWarn, Category: "compose",
				Message: "deploy.replicas só aplica em Docker Swarm — em `docker compose up` é ignorado.",
				DocRef:  "https://docs.docker.com/compose/compose-file/deploy/#replicas",
			})
		}
		if deployHasResourceLimits(deploy) && !serviceUsesClassicResources(svc) {
			*findings = append(*findings, Finding{
				Service: name, Severity: SeverityWarn, Category: "compose",
				Message: "deploy.resources no Compose standalone não define limites no runtime — use mem_limit/cpus no serviço ou analise em modo Swarm.",
				DocRef:  "https://docs.docker.com/compose/compose-file/#mem_limit",
			})
		}
		for _, key := range []string{"restart_policy", "update_config", "placement", "endpoint_mode"} {
			if _, ok := deploy[key]; ok {
				*findings = append(*findings, Finding{
					Service: name, Severity: SeverityInfo, Category: "compose",
					Message: "deploy." + key + " é ignorado em Compose standalone — relevante só em Swarm.",
					DocRef:  "https://docs.docker.com/compose/compose-file/deploy/",
				})
				break
			}
		}
		// Não copiar políticas Swarm para o YAML optimizado em modo Compose
		if optDeploy, _ := optSvc["deploy"].(map[string]any); optDeploy != nil {
			for _, k := range []string{"replicas", "restart_policy", "update_config", "placement"} {
				delete(optDeploy, k)
			}
		}
	}

	// restart no topo é o canal correcto no Compose
	if r, ok := optSvc["restart"].(string); ok && r != "" {
		if deploy != nil {
			if optDeploy, _ := optSvc["deploy"].(map[string]any); optDeploy != nil {
				delete(optDeploy, "restart_policy")
			}
		}
	}
}
