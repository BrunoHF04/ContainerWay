package composeopt

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Severity de um achado.
const (
	SeverityInfo    = "info"
	SeverityWarn    = "warn"
	SeveritySuggest = "suggest"
)

// Finding descreve uma melhoria ou alerta.
type Finding struct {
	Service   string `json:"service"`
	Severity  string `json:"severity"`
	Category  string `json:"category"`
	Message   string `json:"message"`
	DocRef    string `json:"docRef,omitempty"`
	Current   string `json:"current,omitempty"`
	Suggested string `json:"suggested,omitempty"`
}

// AnalyzeResult resultado da análise.
type AnalyzeResult struct {
	Path         string    `json:"path"`
	Original     string    `json:"original"`
	Optimized    string    `json:"optimized"`
	Findings     []Finding `json:"findings"`
	Host         HostSpec  `json:"host"`
	ServiceCount int       `json:"serviceCount"`
	Mode         string    `json:"mode"`
	DetectedKind string    `json:"detectedKind"`
}

// Analyze lê o YAML Compose/Swarm e gera sugestões com base no hardware do host.
func Analyze(path, content string, host HostSpec, opts AnalyzeOptions) (AnalyzeResult, error) {
	content = strings.TrimSpace(NormalizeYAMLInput(content))
	if content == "" {
		return AnalyzeResult{}, fmt.Errorf("arquivo vazio")
	}
	var root map[string]any
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return AnalyzeResult{}, fmt.Errorf("YAML inválido: %w", err)
	}
	services, _ := root["services"].(map[string]any)
	if len(services) == 0 {
		return AnalyzeResult{}, fmt.Errorf("nenhum serviço em services:")
	}

	n := len(services)
	if host.CPUs < 1 {
		host.CPUs = 1
	}
	mode := opts.EffectiveMode(content)
	kind := ClassifyFileKind(path, content)
	fairMem, fairCPU := fairShare(host, n)
	if mode == ModeSwarm && opts.SwarmNodes > 0 {
		fairMem, fairCPU = fairShareSwarm(host, n, opts.SwarmNodes)
	}
	optimized := cloneMap(root)
	optServices, _ := optimized["services"].(map[string]any)

	memAvail := hostMemAvailable(host)
	totalMem := totalDeclaredMemLimits(services, mode)

	totalWeight := 0.0
	weights := make(map[string]float64, n)
	for name, raw := range services {
		svc, _ := raw.(map[string]any)
		w := serviceResourceWeight(name, svc)
		weights[name] = w
		totalWeight += w
	}
	avgWeight := totalWeight / float64(n)

	var findings []Finding
	for name, raw := range services {
		svc, _ := raw.(map[string]any)
		if svc == nil {
			continue
		}
		optSvc, _ := optServices[name].(map[string]any)
		if optSvc == nil {
			optSvc = map[string]any{}
			optServices[name] = optSvc
		}
		w := weights[name]
		svcFairMem, svcFairCPU := scaleFairShare(fairMem, fairCPU, w, avgWeight)
		fs := analyzeService(name, svc, optSvc, host, svcFairMem, svcFairCPU, isJavaService(svc), mode, opts)
		findings = append(findings, fs...)
	}

	if totalMem > memAvail {
		findings = append(findings, Finding{
			Severity: SeverityWarn,
			Category: "memory",
			Message: fmt.Sprintf(
				"Soma dos limites de memória (%s) excede a RAM disponível estimada no host (%s). Não reduzimos limites já definidos — revise manualmente.",
				FormatBytes(totalMem), FormatBytes(memAvail),
			),
			DocRef: "https://docs.docker.com/compose/compose-file/deploy/#resources",
			Current:   FormatBytes(totalMem),
			Suggested: FormatBytes(memAvail),
		})
	}

	optYAML, err := MarshalPreservingOrder(content, optimized)
	if err != nil {
		return AnalyzeResult{}, fmt.Errorf("falha ao gerar YAML preservando original: %w", err)
	}

	return AnalyzeResult{
		Path:         path,
		Original:     content,
		Optimized:    string(optYAML),
		Findings:     findings,
		Host:         host,
		ServiceCount: n,
		Mode:         mode,
		DetectedKind: kind,
	}, nil
}

func fairShareSwarm(host HostSpec, serviceCount, swarmNodes int) (mem int64, cpu float64) {
	mem, cpu = fairShare(host, serviceCount)
	if swarmNodes > 1 {
		cpu = cpu / float64(swarmNodes)
		if cpu < 0.1 {
			cpu = 0.1
		}
	}
	return mem, cpu
}

func fairShare(host HostSpec, serviceCount int) (mem int64, cpu float64) {
	if serviceCount < 1 {
		serviceCount = 1
	}
	reserve := host.MemTotal / 6
	if reserve < 512*1024*1024 {
		reserve = 512 * 1024 * 1024
	}
	avail := host.MemTotal - reserve
	if avail < 256*1024*1024 {
		avail = host.MemTotal / 2
	}
	if avail < 128*1024*1024 {
		avail = 128 * 1024 * 1024
	}
	mem = avail / int64(serviceCount)
	if mem < 128*1024*1024 {
		mem = 128 * 1024 * 1024
	}
	cpu = float64(host.CPUs) / float64(serviceCount)
	if cpu < 0.25 {
		cpu = 0.25
	}
	if cpu > float64(host.CPUs) {
		cpu = float64(host.CPUs)
	}
	return mem, cpu
}

func analyzeService(name string, svc, optSvc map[string]any, host HostSpec, fairMem int64, fairCPU float64, isJava bool, mode string, opts AnalyzeOptions) []Finding {
	var out []Finding
	if mode == ModeSwarm {
		analyzeSwarmExtras(name, svc, optSvc, host, opts.SwarmNodes, &out)
	} else {
		analyzeComposeExtras(name, svc, optSvc, &out)
	}
	curMem, hasMem := serviceMemLimitForMode(svc, mode)
	curCPU, hasCPU := serviceCPULimitForMode(svc, mode)
	memRef := memoryDocRef(mode)
	cpuRef := cpuDocRef(mode)

	if !hasMem {
		suggestMem := FormatBytes(fairMem)
		patchMemoryIfMissing(optSvc, mode, fairMem/2, fairMem)
		msg := "Sem limite de memória — risco de consumir toda a RAM do host (OOM)."
		if mode == ModeCompose && resourceWriteTarget(optSvc, mode) == resTargetClassic {
			msg = "Sem mem_limit — no Compose defina mem_limit (deploy.resources não aplica em compose up)."
		}
		out = append(out, Finding{
			Service: name, Severity: SeveritySuggest, Category: "memory",
			Message:   msg,
			DocRef:    memRef,
			Current:   "—",
			Suggested: suggestMem,
		})
		curMem = fairMem
		hasMem = true
	} else if curMem > fairMem*2 {
		out = append(out, Finding{
			Service: name, Severity: SeverityInfo, Category: "memory",
			Message: fmt.Sprintf(
				"Limite (%s) acima da quota média (~%s para %d serviços). Mantido — não reduzimos automaticamente serviços já dimensionados (ex.: PostgreSQL, WildFly).",
				FormatBytes(curMem), FormatBytes(fairMem), countHint(host),
			),
			DocRef: memRef,
			Current: FormatBytes(curMem),
		})
	} else if curMem < fairMem/2 && isJava && curMem < 512*1024*1024 {
		out = append(out, Finding{
			Service: name, Severity: SeverityWarn, Category: "memory",
			Message:   "Memória baixa para aplicação Java — considere aumentar o limite e alinhar -Xmx.",
			DocRef:    "https://docs.oracle.com/en/java/javase/21/gctuning/",
			Current:   FormatBytes(curMem),
			Suggested: FormatBytes(fairMem),
		})
	} else if curMem < fairMem {
		out = append(out, Finding{
			Service: name, Severity: SeverityInfo, Category: "memory",
			Message: fmt.Sprintf(
				"Limite (%s) abaixo da quota média estimada (%s). Mantido — aumente manualmente se o serviço precisar de mais RAM.",
				FormatBytes(curMem), FormatBytes(fairMem),
			),
			DocRef: memRef,
			Current: FormatBytes(curMem),
		})
	}

	if !hasCPU {
		if patchCPUsIfMissing(optSvc, mode, fairCPU*0.5, fairCPU) {
			cpuMsg := "Sem limite de CPU — pode monopolizar núcleos do host."
			if mode == ModeCompose && resourceWriteTarget(optSvc, mode) == resTargetClassic {
				cpuMsg = "Sem cpus — no Compose defina cpus no serviço."
			}
			out = append(out, Finding{
				Service: name, Severity: SeveritySuggest, Category: "cpu",
				Message:   cpuMsg,
				DocRef:    cpuRef,
				Current:   "—",
				Suggested: FormatCPUs(fairCPU),
			})
		}
	} else if curCPU > float64(host.CPUs) {
		out = append(out, Finding{
			Service: name, Severity: SeverityWarn, Category: "cpu",
			Message:   fmt.Sprintf("Limite de CPU (%s) superior aos %d núcleos do host — revise manualmente.", FormatCPUs(curCPU), host.CPUs),
			DocRef:    cpuRef,
			Current:   FormatCPUs(curCPU),
		})
	}

	if isJava {
		lim := effectiveMemLimitForMode(optSvc, mode, fairMem)
		xmx, envKey, envVal := javaXmxFromEnv(svc)
		target := SuggestXmx(lim)
		if xmx > lim {
			newEnv := patchJavaXmx(envVal, target)
			patchEnv(optSvc, envKey, newEnv)
			out = append(out, Finding{
				Service: name, Severity: SeverityWarn, Category: "java",
				Message: fmt.Sprintf(
					"-Xmx (%s) excede o limite de memória do contêiner (%s) — risco de OOM. Ajustado para %s.",
					FormatBytes(xmx), FormatBytes(lim), target,
				),
				DocRef:    "https://docs.docker.com/config/containers/resource_constraints/",
				Current:   envVal,
				Suggested: newEnv,
			})
		} else if xmx == 0 {
			newEnv := patchJavaXmx(envVal, target)
			patchEnv(optSvc, envKey, newEnv)
			out = append(out, Finding{
				Service: name, Severity: SeveritySuggest, Category: "java",
				Message:   fmt.Sprintf("Defina -Xmx (~65%% do limite %s). Sugestão: %s.", FormatBytes(lim), target),
				DocRef:    "https://docs.docker.com/config/containers/resource_constraints/",
				Current:   envVal,
				Suggested: newEnv,
			})
		} else if xmx > int64(float64(lim)*0.85) {
			out = append(out, Finding{
				Service: name, Severity: SeverityWarn, Category: "java",
				Message: fmt.Sprintf(
					"-Xmx (%s) muito próximo do limite do contêiner (%s) — risco de OOM por metaspace/threads. Reduza -Xmx ou aumente o limite.",
					FormatBytes(xmx), FormatBytes(lim),
				),
				DocRef: "https://docs.docker.com/config/containers/resource_constraints/",
				Current: envVal,
			})
		}
		if _, ok := svc["shm_size"]; !ok {
			optSvc["shm_size"] = "256m"
			out = append(out, Finding{
				Service: name, Severity: SeverityInfo, Category: "java",
				Message:   "shm_size não definido — recomendado para JVM (heap / metaspace).",
				DocRef:    "https://docs.docker.com/compose/compose-file/#shm_size",
				Suggested: "256m",
			})
		}
	}

	if _, ok := svc["healthcheck"]; !ok {
		out = append(out, Finding{
			Service: name, Severity: SeverityInfo, Category: "healthcheck",
			Message: "Sem healthcheck — o orquestrador não detecta falhas silenciosas.",
			DocRef:  "https://docs.docker.com/compose/compose-file/healthcheck/",
			Suggested: "test: [\"CMD\", \"curl\", \"-f\", \"http://localhost/\"]",
		})
	}

	if mode == ModeCompose {
		restart, _ := svc["restart"].(string)
		if restart == "" || restart == "no" {
			optSvc["restart"] = "unless-stopped"
			out = append(out, Finding{
				Service: name, Severity: SeverityInfo, Category: "restart",
				Message:   "Política de reinício ausente ou desativada.",
				DocRef:    "https://docs.docker.com/compose/compose-file/#restart",
				Current:   restart,
				Suggested: "unless-stopped",
			})
		}
	}

	return out
}

func countHint(host HostSpec) int {
	if host.CPUs <= 2 {
		return 2
	}
	return host.CPUs
}

func serviceMemLimit(svc map[string]any) (int64, bool) {
	if lim, ok := svc["mem_limit"]; ok {
		if b, ok := parseMemField(lim); ok {
			return b, true
		}
	}
	return serviceMemLimitFromDeploy(svc)
}

func serviceCPULimit(svc map[string]any) (float64, bool) {
	if cpus, ok := svc["cpus"]; ok {
		if f, ok := ParseCPUs(cpus); ok {
			return f, true
		}
	}
	return serviceCPULimitFromDeploy(svc)
}

func isJavaService(svc map[string]any) bool {
	img, _ := svc["image"].(string)
	low := strings.ToLower(img)
	for _, k := range []string{"java", "openjdk", "temurin", "jdk", "jre", "spring", "tomcat", "wildfly", "jetty"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	env := envMap(svc)
	for _, key := range []string{"JAVA_OPTS", "JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS"} {
		if v, ok := env[key]; ok && strings.Contains(strings.ToLower(v), "xmx") {
			return true
		}
	}
	return false
}

func envMap(svc map[string]any) map[string]string {
	out := map[string]string{}
	switch e := svc["environment"].(type) {
	case map[string]any:
		for k, v := range e {
			out[k] = fmt.Sprint(v)
		}
	case []any:
		for _, item := range e {
			s, _ := item.(string)
			if i := strings.IndexByte(s, '='); i > 0 {
				out[s[:i]] = s[i+1:]
			}
		}
	}
	return out
}

func javaXmxFromEnv(svc map[string]any) (xmx int64, key, val string) {
	env := envMap(svc)
	for _, k := range []string{"JAVA_OPTS", "JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS"} {
		if v, ok := env[k]; ok {
			if n, ok := ParseXmx(v); ok {
				return n, k, v
			}
		}
	}
	return 0, "JAVA_OPTS", ""
}

func patchJavaXmx(current, target string) string {
	if current == "" {
		return "-Xmx" + target
	}
	if xmxRe.MatchString(current) {
		return xmxRe.ReplaceAllString(current, "-Xmx"+target)
	}
	return strings.TrimSpace(current) + " -Xmx" + target
}

func patchEnv(svc map[string]any, key, val string) {
	switch e := svc["environment"].(type) {
	case map[string]any:
		e[key] = val
	case []any:
		found := false
		for i, item := range e {
			s, _ := item.(string)
			if strings.HasPrefix(s, key+"=") {
				e[i] = key + "=" + val
				found = true
				break
			}
		}
		if !found {
			svc["environment"] = append(e, key+"="+val)
		}
	default:
		svc["environment"] = map[string]any{key: val}
	}
}

func cloneMap(m map[string]any) map[string]any {
	b, err := yaml.Marshal(m)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	_ = yaml.Unmarshal(b, &out)
	return out
}
