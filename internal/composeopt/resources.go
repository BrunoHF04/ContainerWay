package composeopt

import (
	"strings"
)

const (
	resTargetDeploy  = "deploy"
	resTargetClassic = "classic"
)

func deployMode(deploy map[string]any) string {
	if deploy == nil {
		return ""
	}
	m, _ := deploy["mode"].(string)
	return strings.ToLower(strings.TrimSpace(m))
}

func isDeployGlobal(deploy map[string]any) bool {
	return deployMode(deploy) == "global"
}

func serviceHasDeploy(svc map[string]any) bool {
	_, ok := svc["deploy"].(map[string]any)
	return ok
}

func deployHasResourceLimits(deploy map[string]any) bool {
	if deploy == nil {
		return false
	}
	res, _ := deploy["resources"].(map[string]any)
	if res == nil {
		return false
	}
	limits, _ := res["limits"].(map[string]any)
	if limits == nil {
		return false
	}
	_, mem := limits["memory"]
	_, cpu := limits["cpus"]
	return mem || cpu
}

func serviceUsesClassicResources(svc map[string]any) bool {
	if _, ok := svc["mem_limit"]; ok {
		return true
	}
	if _, ok := svc["mem_reservation"]; ok {
		return true
	}
	if _, ok := svc["cpus"]; ok {
		return true
	}
	return false
}

// resourceWriteTarget escolhe onde gravar novos limites (Compose vs Swarm).
func resourceWriteTarget(svc map[string]any, mode string) string {
	if mode == ModeSwarm {
		return resTargetDeploy
	}
	if serviceUsesClassicResources(svc) {
		return resTargetClassic
	}
	if deploy, _ := svc["deploy"].(map[string]any); deploy != nil && deployHasResourceLimits(deploy) {
		return resTargetDeploy
	}
	return resTargetClassic
}

func ensureDeploy(svc map[string]any) map[string]any {
	deploy, _ := svc["deploy"].(map[string]any)
	if deploy == nil {
		deploy = map[string]any{}
		svc["deploy"] = deploy
	}
	return deploy
}

func ensureResources(deploy map[string]any) map[string]any {
	res, _ := deploy["resources"].(map[string]any)
	if res == nil {
		res = map[string]any{}
		deploy["resources"] = res
	}
	return res
}

func ensureLimits(res map[string]any) map[string]any {
	limits, _ := res["limits"].(map[string]any)
	if limits == nil {
		limits = map[string]any{}
		res["limits"] = limits
	}
	return limits
}

func ensureReservations(res map[string]any) map[string]any {
	reservations, _ := res["reservations"].(map[string]any)
	if reservations == nil {
		reservations = map[string]any{}
		res["reservations"] = reservations
	}
	return reservations
}

func parseMemField(v any) (int64, bool) {
	switch t := v.(type) {
	case string:
		return ParseBytes(t)
	case int:
		return int64(t), true
	case int64:
		return t, true
	default:
		return 0, false
	}
}

func serviceMemLimitFromDeploy(svc map[string]any) (int64, bool) {
	deploy, _ := svc["deploy"].(map[string]any)
	if deploy == nil {
		return 0, false
	}
	res, _ := deploy["resources"].(map[string]any)
	if res == nil {
		return 0, false
	}
	limits, _ := res["limits"].(map[string]any)
	if limits == nil {
		return 0, false
	}
	return parseMemField(limits["memory"])
}

func serviceCPULimitFromDeploy(svc map[string]any) (float64, bool) {
	deploy, _ := svc["deploy"].(map[string]any)
	if deploy == nil {
		return 0, false
	}
	res, _ := deploy["resources"].(map[string]any)
	if res == nil {
		return 0, false
	}
	limits, _ := res["limits"].(map[string]any)
	if limits == nil {
		return 0, false
	}
	if cpus, ok := limits["cpus"]; ok {
		return ParseCPUs(cpus)
	}
	return 0, false
}

// serviceMemLimitForMode lê o limite efectivo conforme o runtime (Compose vs Swarm).
func serviceMemLimitForMode(svc map[string]any, mode string) (int64, bool) {
	if mode == ModeSwarm {
		if m, ok := serviceMemLimitFromDeploy(svc); ok {
			return m, true
		}
		return 0, false
	}
	return serviceMemLimit(svc)
}

func serviceCPULimitForMode(svc map[string]any, mode string) (float64, bool) {
	if mode == ModeSwarm {
		if c, ok := serviceCPULimitFromDeploy(svc); ok {
			return c, true
		}
		return 0, false
	}
	return serviceCPULimit(svc)
}

// effectiveMemLimit devolve o limite já presente no serviço optimizado ou o fallback.
func effectiveMemLimit(svc map[string]any, fallback int64) int64 {
	if m, ok := serviceMemLimit(svc); ok && m > 0 {
		return m
	}
	return fallback
}

func effectiveMemLimitForMode(svc map[string]any, mode string, fallback int64) int64 {
	if m, ok := serviceMemLimitForMode(svc, mode); ok && m > 0 {
		return m
	}
	return fallback
}

func totalDeclaredMemLimits(services map[string]any, mode string) int64 {
	var sum int64
	for _, raw := range services {
		svc, _ := raw.(map[string]any)
		if svc == nil {
			continue
		}
		if m, ok := serviceMemLimitForMode(svc, mode); ok {
			sum += m
		}
	}
	return sum
}

func hostMemAvailable(host HostSpec) int64 {
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
	return avail
}

func patchClassicMemoryIfMissing(svc map[string]any, resMem, limMem int64) bool {
	if _, ok := serviceMemLimit(svc); ok {
		return false
	}
	svc["mem_limit"] = FormatBytes(limMem)
	if _, ok := svc["mem_reservation"]; !ok {
		svc["mem_reservation"] = FormatBytes(resMem)
	}
	return true
}

func patchClassicCPUsIfMissing(svc map[string]any, limCPU float64) bool {
	if _, ok := serviceCPULimit(svc); ok {
		return false
	}
	svc["cpus"] = FormatCPUs(limCPU)
	return true
}

func patchDeployMemoryIfMissing(svc map[string]any, resMem, limMem int64) bool {
	if _, ok := serviceMemLimit(svc); ok {
		return false
	}
	deploy := ensureDeploy(svc)
	res := ensureResources(deploy)
	limits := ensureLimits(res)
	limits["memory"] = FormatBytes(limMem)
	reservations := ensureReservations(res)
	if _, ok := reservations["memory"]; !ok {
		reservations["memory"] = FormatBytes(resMem)
	}
	return true
}

func patchDeployCPUsIfMissing(svc map[string]any, resCPU, limCPU float64) bool {
	if _, ok := serviceCPULimit(svc); ok {
		return false
	}
	deploy := ensureDeploy(svc)
	res := ensureResources(deploy)
	limits := ensureLimits(res)
	limits["cpus"] = FormatCPUs(limCPU)
	reservations := ensureReservations(res)
	if _, ok := reservations["cpus"]; !ok {
		reservations["cpus"] = FormatCPUs(resCPU)
	}
	return true
}

// patchMemoryIfMissing grava memória no alvo correcto para o modo (Compose clássico ou Swarm deploy).
func patchMemoryIfMissing(svc map[string]any, mode string, resMem, limMem int64) bool {
	if _, ok := serviceMemLimit(svc); ok {
		return false
	}
	if resourceWriteTarget(svc, mode) == resTargetDeploy {
		return patchDeployMemoryIfMissing(svc, resMem, limMem)
	}
	return patchClassicMemoryIfMissing(svc, resMem, limMem)
}

// patchCPUsIfMissing grava CPU no alvo correcto para o modo.
func patchCPUsIfMissing(svc map[string]any, mode string, resCPU, limCPU float64) bool {
	if _, ok := serviceCPULimit(svc); ok {
		return false
	}
	if resourceWriteTarget(svc, mode) == resTargetDeploy {
		return patchDeployCPUsIfMissing(svc, resCPU, limCPU)
	}
	return patchClassicCPUsIfMissing(svc, limCPU)
}

func memoryDocRef(mode string) string {
	if mode == ModeSwarm {
		return "https://docs.docker.com/compose/compose-file/deploy/#resources"
	}
	return "https://docs.docker.com/compose/compose-file/#mem_limit"
}

func cpuDocRef(mode string) string {
	if mode == ModeSwarm {
		return "https://docs.docker.com/compose/compose-file/deploy/#cpus"
	}
	return "https://docs.docker.com/compose/compose-file/#cpus"
}
