package diskutil

import (
	"strings"
)

// InterpretLVCmdError traduz saída de lvextend/lvreduce/resize2fs para mensagem legível.
func InterpretLVCmdError(stderr, stdout string, err error) string {
	blob := strings.ToLower(stderr + "\n" + stdout)
	switch {
	case strings.Contains(blob, "not reduced"),
		strings.Contains(blob, "cannot be reduced"),
		strings.Contains(blob, "can't reduce"):
		return "o volume lógico não foi reduzido — o tamanho pedido é pequeno demais para os dados em uso"
	case strings.Contains(blob, "filesystem reduce"),
		strings.Contains(blob, "resize2fs") && strings.Contains(blob, "minimum"),
		strings.Contains(blob, "on-line shrinking"):
		return "o sistema de arquivos não pode reduzir até esse tamanho (há mais dados do que o espaço alvo)"
	case strings.Contains(blob, "must be unmounted"),
		strings.Contains(blob, "is mounted on"):
		return "o volume está montado — a redução pode exigir manutenção ou um tamanho mínimo maior"
	case strings.Contains(blob, "insufficient free space"),
		strings.Contains(blob, "not enough free space"):
		return "espaço livre insuficiente no volume group ou no sistema de arquivos"
	case strings.Contains(blob, "invalid argument"):
		return "tamanho inválido para redução — verifique o valor em GB"
	}
	if line := lastMeaningfulLine(stderr, stdout); line != "" {
		return line
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "exited with status") {
			return "comando LVM falhou no servidor (código de saída não zero) — veja os detalhes abaixo"
		}
		return msg
	}
	return "falha ao executar operação LVM no servidor"
}

func lastMeaningfulLine(stderr, stdout string) string {
	combined := strings.TrimSpace(stderr)
	if strings.TrimSpace(stdout) != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += strings.TrimSpace(stdout)
	}
	lines := strings.Split(combined, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "stderr:") {
			continue
		}
		if len(line) > 240 {
			line = line[:237] + "…"
		}
		return line
	}
	return ""
}
