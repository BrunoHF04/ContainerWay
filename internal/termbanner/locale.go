package termbanner

import "strings"

// Labels textos do MOTD do terminal (PT / EN / ES).
type Labels struct {
	Title       string
	System      string
	Now         string
	LineA       string
	LineBDisk   string
	LineBDiskPc string
	LineBTemp   string
	LineBIP     string
	BootUptime  string
	Prompt      string
}

var labelsByLang = map[string]Labels{
	"pt": {
		Title:       " ─── ContainerWay · Terminal SSH ─── ",
		System:      "Sistema:",
		Now:         "Agora:",
		LineA:       "Carga %s %s %s | RAM %s%% | Swap %s%% | Proc %s | who %s",
		LineBDisk:   "Disco / %s%% (%s/%s)",
		LineBDiskPc: "Disco / %s%%",
		LineBTemp:   " | Temp %s°C",
		LineBIP:     " | IPv4 %s",
		BootUptime:  "Boot %s | Uptime %s",
		Prompt:      ">>> ContainerWay",
	},
	"en": {
		Title:       " ─── ContainerWay · SSH terminal ─── ",
		System:      "System:",
		Now:         "Now:",
		LineA:       "Load %s %s %s | RAM %s%% | Swap %s%% | Proc %s | who %s",
		LineBDisk:   "Disk / %s%% (%s/%s)",
		LineBDiskPc: "Disk / %s%%",
		LineBTemp:   " | Temp %s°C",
		LineBIP:     " | IPv4 %s",
		BootUptime:  "Boot %s | Uptime %s",
		Prompt:      ">>> ContainerWay",
	},
	"es": {
		Title:       " ─── ContainerWay · Terminal SSH ─── ",
		System:      "Sistema:",
		Now:         "Ahora:",
		LineA:       "Carga %s %s %s | RAM %s%% | Swap %s%% | Proc %s | who %s",
		LineBDisk:   "Disco / %s%% (%s/%s)",
		LineBDiskPc: "Disco / %s%%",
		LineBTemp:   " | Temp %s°C",
		LineBIP:     " | IPv4 %s",
		BootUptime:  "Boot %s | Uptime %s",
		Prompt:      ">>> ContainerWay",
	},
}

// NormalizeLang mapeia pt-BR, pt, en, es para chaves do MOTD.
func NormalizeLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch {
	case strings.HasPrefix(l, "en"):
		return "en"
	case strings.HasPrefix(l, "es"):
		return "es"
	default:
		return "pt"
	}
}

// LabelsFor devolve rótulos no idioma pedido (fallback PT).
func LabelsFor(lang string) Labels {
	if lb, ok := labelsByLang[NormalizeLang(lang)]; ok {
		return lb
	}
	return labelsByLang["pt"]
}
