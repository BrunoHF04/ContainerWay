package diskutil

import (
	"strings"
)

// GrowFSScript devolve script bash para redimensionar o sistema de ficheiros após lvextend.
func GrowFSScript(lv, fs string) string {
	fs = strings.ToLower(strings.TrimSpace(fs))
	switch fs {
	case "ext4", "ext3", "ext2":
		return wrapLVScript(lv,
			"command -v resize2fs >/dev/null 2>&1 || { echo 'resize2fs não encontrado.' >&2; exit 1; }; resize2fs \"$LV\"")
	case "xfs":
		return wrapLVScript(lv,
			"command -v xfs_growfs >/dev/null 2>&1 || { echo 'xfs_growfs não encontrado.' >&2; exit 1; }; "+
				"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); "+
				"if [ -z \"$M\" ]; then echo 'Não foi possível resolver o ponto de montagem para xfs_growfs.' >&2; exit 1; fi; "+
				"xfs_growfs \"$M\"")
	case "btrfs":
		return wrapLVScript(lv,
			"command -v btrfs >/dev/null 2>&1 || { echo 'btrfs não encontrado.' >&2; exit 1; }; "+
				"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); "+
				"if [ -z \"$M\" ]; then echo 'Não foi possível resolver o ponto de montagem para btrfs.' >&2; exit 1; fi; "+
				"btrfs filesystem resize max \"$M\"")
	default:
		return "echo 'Sistema de arquivos não suportado para crescimento automático.' >&2; exit 1"
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// LvextendScript devolve script bash para ampliar um LV.
func LvextendScript(lv string, gib float64) string {
	return wrapLVScript(lv,
		"command -v lvextend >/dev/null 2>&1 || { echo 'lvextend não encontrado.' >&2; exit 1; }; "+
			"lvextend -L +"+FormatLVMSizeG(UserGBToLVMG(gib))+"G \"$LV\"")
}
