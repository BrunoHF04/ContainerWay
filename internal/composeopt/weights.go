package composeopt

import (
	"fmt"
	"strings"
)

// serviceResourceWeight atribui peso relativo à RAM/CPU sugerida quando o serviço ainda não tem limite.
func serviceResourceWeight(name string, svc map[string]any) float64 {
	if svc == nil {
		return 1
	}
	low := strings.ToLower(name)
	img := strings.ToLower(fmt.Sprint(svc["image"]))
	combined := low + " " + img

	switch {
	case strings.Contains(combined, "postgres"),
		strings.Contains(combined, "wildfly"),
		strings.Contains(combined, "orion"),
		strings.Contains(combined, "mysql"),
		strings.Contains(combined, "mariadb"),
		strings.Contains(combined, "mongodb"),
		strings.Contains(combined, "elasticsearch"),
		strings.Contains(combined, "kafka"),
		strings.Contains(combined, "oracle"),
		strings.Contains(combined, "mssql"),
		isJavaService(svc):
		return 2
	case strings.Contains(combined, "nginx"),
		strings.Contains(combined, "redis"),
		strings.Contains(combined, "memcached"),
		strings.Contains(combined, "traefik"),
		strings.Contains(combined, "caddy"):
		return 0.75
	default:
		return 1
	}
}

// scaleFairShare ajusta a quota média pelo peso do serviço (pesados recebem mais ao adicionar limites).
func scaleFairShare(baseMem int64, baseCPU float64, weight, avgWeight float64) (mem int64, cpu float64) {
	mem, cpu = baseMem, baseCPU
	if avgWeight <= 0 || weight <= 0 {
		return mem, cpu
	}
	scale := weight / avgWeight
	mem = int64(float64(baseMem) * scale)
	if mem < 128*1024*1024 {
		mem = 128 * 1024 * 1024
	}
	cpu = baseCPU * scale
	if cpu < 0.25 {
		cpu = 0.25
	}
	return mem, cpu
}
