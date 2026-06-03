package diskutil

import "strings"

// ShrinkLVScript devolve script bash para reduzir um LV (sistema de ficheiros + lvextend inverso).
// ext2/3/4: lvreduce --resizefs; btrfs: resize do FS e depois lvreduce; xfs não suporta redução.
func ShrinkLVScript(lv string, gib float64, fs string) string {
	fs = strings.ToLower(strings.TrimSpace(fs))
	g := FormatLVMSizeG(UserGBToLVMG(gib))
	switch fs {
	case "ext4", "ext3", "ext2":
		return wrapLVScript(lv,
			ShrinkFSMountGuard()+
				"command -v lvreduce >/dev/null 2>&1 || { echo 'lvreduce não encontrado.' >&2; exit 1; }; "+
				"lvreduce --resizefs -L -"+g+"G -f \"$LV\"")
	case "btrfs":
		return wrapLVScript(lv,
			"GIB="+g+"; "+
				"command -v btrfs >/dev/null 2>&1 || { echo 'btrfs não encontrado.' >&2; exit 1; }; "+
				"command -v lvreduce >/dev/null 2>&1 || { echo 'lvreduce não encontrado.' >&2; exit 1; }; "+
				"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); "+
				"if [ -z \"$M\" ]; then echo 'Montagem não encontrada para btrfs.' >&2; exit 1; fi; "+
				"btrfs filesystem resize -\"${GIB}\"G \"$M\"; "+
				"lvreduce -L -\"${GIB}\"G -f \"$LV\"")
	default:
		return "echo 'Redução automática não suportada para este sistema de arquivos (use ext2/3/4 ou btrfs; xfs não pode ser reduzido).' >&2; exit 1"
	}
}
