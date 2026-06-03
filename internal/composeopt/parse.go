package composeopt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var xmxRe = regexp.MustCompile(`(?i)-Xmx(\d+)([kKmMgG]?)`)

// ParseBytes interpreta valores Docker Compose (512M, 1g, 1073741824).
func ParseBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	mult := int64(1)
	lower := strings.ToLower(s)
	switch {
	case strings.HasSuffix(lower, "gib"), strings.HasSuffix(lower, "gb"):
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(lower, "mib"), strings.HasSuffix(lower, "mb"):
		mult = 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(lower, "kib"), strings.HasSuffix(lower, "kb"):
		mult = 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(lower, "g"):
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case strings.HasSuffix(lower, "m"):
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case strings.HasSuffix(lower, "k"):
		mult = 1024
		s = s[:len(s)-1]
	}
	s = strings.TrimSpace(s)
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return int64(n * float64(mult)), true
}

// FormatBytes formata bytes para notação Compose (M/G).
func FormatBytes(b int64) string {
	if b <= 0 {
		return "0"
	}
	const gib = 1024 * 1024 * 1024
	const mib = 1024 * 1024
	if b >= gib && b%mib == 0 {
		return fmt.Sprintf("%dG", b/gib)
	}
	if b >= mib {
		mb := b / mib
		if mb > 0 {
			return fmt.Sprintf("%dM", mb)
		}
	}
	return fmt.Sprintf("%d", b)
}

// ParseCPUs interpreta limite de CPUs (string ou número).
func ParseCPUs(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// FormatCPUs formata CPUs para YAML Compose.
func FormatCPUs(f float64) string {
	if f <= 0 {
		return "0.25"
	}
	if f == float64(int(f)) {
		return strconv.Itoa(int(f))
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// ParseXmx extrai -Xmx de variáveis Java (bytes).
func ParseXmx(envLine string) (int64, bool) {
	m := xmxRe.FindStringSubmatch(envLine)
	if len(m) < 3 {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(m[2]) {
	case "g":
		n *= 1024 * 1024 * 1024
	case "m", "":
		n *= 1024 * 1024
	case "k":
		n *= 1024
	}
	return n, true
}

// SuggestXmx sugere -Xmx em notação Java (ex.: 768m).
func SuggestXmx(containerMem int64) string {
	if containerMem <= 0 {
		return "256m"
	}
	heap := int64(float64(containerMem) * 0.65)
	const mib = 1024 * 1024
	if heap < 128*mib {
		heap = 128 * mib
	}
	mb := heap / mib
	return fmt.Sprintf("%dm", mb)
}
