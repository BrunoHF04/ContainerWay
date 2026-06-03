package diskutil

import "strings"

// LvextendAbsScript define tamanho absoluto do LV (GiB).
func LvextendAbsScript(lv string, gib float64) string {
	return wrapLVScript(lv,
		"command -v lvextend >/dev/null 2>&1 || { echo 'lvextend não encontrado.' >&2; exit 1; }; "+
			"lvextend -L "+FormatLVMSizeG(UserGBToLVMG(gib))+"G \"$LV\"")
}

// LvreduceAbsScript reduz LV para tamanho absoluto (GiB).
func LvreduceAbsScript(lv string, targetGiB float64, fs string) string {
	fs = strings.ToLower(strings.TrimSpace(fs))
	t := FormatLVMSizeG(UserGBToLVMG(targetGiB))
	if fs == "btrfs" {
		return wrapLVScript(lv,
			"TARGET="+t+"; command -v lvs >/dev/null 2>&1 || exit 1; "+
				"command -v btrfs >/dev/null 2>&1 || exit 1; command -v lvreduce >/dev/null 2>&1 || exit 1; "+
				"CUR=$(lvs --noheadings -o lv_size --units g --nosuffix \"$LV\" 2>/dev/null | tr -d ' '); "+
				"DELTA=$(awk -v c=\"$CUR\" -v t=\"$TARGET\" 'BEGIN{printf \"%.4g\", c-t}'); "+
				"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null); "+
				"if [ -z \"$M\" ]; then exit 1; fi; "+
				"btrfs filesystem resize -\"${DELTA}\"G \"$M\"; lvreduce -L \"${TARGET}\"G -f \"$LV\"")
	}
	return wrapLVScript(lv,
		ShrinkFSMountGuard()+
			"command -v lvreduce >/dev/null 2>&1 || { echo 'lvreduce não encontrado.' >&2; exit 1; }; "+
			"lvreduce --resizefs -L "+t+"G -f \"$LV\"")
}

// FsckCheckScript verificação read-only do sistema de ficheiros.
func FsckCheckScript(lv, fs string) string {
	fs = strings.ToLower(strings.TrimSpace(fs))
	switch fs {
	case "ext4", "ext3", "ext2":
		return wrapLVScript(lv,
			"command -v e2fsck >/dev/null 2>&1 || { echo 'e2fsck não encontrado.' >&2; exit 1; }; e2fsck -fn \"$LV\"")
	case "xfs":
		return wrapLVScript(lv,
			"command -v xfs_info >/dev/null 2>&1 || { echo 'xfs_info não encontrado.' >&2; exit 1; }; "+
				"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); xfs_info \"$LV\"; "+
				"echo '(XFS: redução não suportada; verificação completa xfs_repair exige manutenção offline.)'")
	case "btrfs":
		return wrapLVScript(lv,
			"command -v btrfs >/dev/null 2>&1 || { echo 'btrfs não encontrado.' >&2; exit 1; }; "+
				"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); "+
				"if [ -z \"$M\" ]; then echo 'Montagem não encontrada.' >&2; exit 1; fi; btrfs check --readonly \"$M\"")
	default:
		return "echo 'Verificação automática não suportada para este FS.' >&2; exit 1"
	}
}

// SnapshotCreateScript cria snapshot LVM (-s).
func SnapshotCreateScript(originLV, snapName string, gib float64) string {
	g := FormatLVMSizeG(UserGBToLVMG(gib))
	n := shellQuote(snapName)
	return LVResolveShellPreamble() + "set -e; ORIGIN=$(resolve_lv_device " + shellQuote(originLV) + "); NAME=" + n + "; " +
		"command -v lvcreate >/dev/null 2>&1 || { echo 'lvcreate não encontrado.' >&2; exit 1; }; " +
		"lvcreate -s -n \"$NAME\" -L " + g + "G \"$ORIGIN\""
}

// SnapshotRemoveScript remove snapshot (-f).
func SnapshotRemoveScript(snapPath string) string {
	return LVResolveShellPreamble() + "set -e; SNAP=$(resolve_lv_device " + shellQuote(snapPath) + "); " +
		"command -v lvremove >/dev/null 2>&1 || { echo 'lvremove não encontrado.' >&2; exit 1; }; " +
		"lvremove -f \"$SNAP\""
}

// LVCreateScript cria LV e opcionalmente formata.
func LVCreateScript(vg, lvName string, gib float64, fs string, mkfs bool) string {
	fs = strings.ToLower(strings.TrimSpace(fs))
	script := "set -e; VG=" + shellQuote(vg) + "; LV=" + shellQuote(lvName) + "; " +
		"command -v lvcreate >/dev/null 2>&1 || { echo 'lvcreate não encontrado.' >&2; exit 1; }; " +
		"lvcreate -L " + FormatLVMSizeG(UserGBToLVMG(gib)) + "G -n \"$LV\" \"$VG\"; " +
		"DEV=\"/dev/$VG/$LV\"; " +
		"if [ ! -e \"$DEV\" ]; then DEV=\"/dev/mapper/${VG}-$(echo \"$LV\" | tr '-' '--')\"; fi; " +
		"echo \"LV criado: $DEV\""
	if !mkfs || fs == "" {
		return script
	}
	switch fs {
	case "ext4", "ext3", "ext2":
		script += "; command -v mkfs." + fs + " >/dev/null 2>&1 || { echo 'mkfs não encontrado.' >&2; exit 1; }; mkfs." + fs + " -F \"$DEV\""
	case "xfs":
		script += "; command -v mkfs.xfs >/dev/null 2>&1 || { echo 'mkfs.xfs não encontrado.' >&2; exit 1; }; mkfs.xfs -f \"$DEV\""
	case "btrfs":
		script += "; command -v mkfs.btrfs >/dev/null 2>&1 || { echo 'mkfs.btrfs não encontrado.' >&2; exit 1; }; mkfs.btrfs -f \"$DEV\""
	default:
		script += "; echo 'Formatação não suportada para " + fs + ".' >&2; exit 1"
	}
	return script
}

// ResizeFSScriptOnly redimensiona só o FS: grow=true amplia; grow=false encolhe delta GiB.
func ResizeFSScriptOnly(lv, fs string, gib float64, grow bool) string {
	fs = strings.ToLower(strings.TrimSpace(fs))
	g := FormatLVMSizeG(UserGBToLVMG(gib))
	if grow {
		switch fs {
		case "ext4", "ext3", "ext2":
			return wrapLVScript(lv, "command -v resize2fs >/dev/null 2>&1 || exit 1; resize2fs \"$LV\"")
		case "xfs", "btrfs":
			return GrowFSScript(lv, fs)
		default:
			return "echo 'FS não suportado.' >&2; exit 1"
		}
	}
	switch fs {
	case "ext4", "ext3", "ext2":
		return wrapLVScript(lv,
			ShrinkFSMountGuard()+
				"GIB="+g+"; command -v resize2fs >/dev/null 2>&1 || exit 1; "+
				"command -v lvs >/dev/null 2>&1 || exit 1; "+
				"CUR=$(lvs --noheadings -o lv_size --units g --nosuffix \"$LV\" 2>/dev/null | tr -d ' '); "+
				"NEW=$(awk -v c=\"$CUR\" -v d=\"$GIB\" 'BEGIN{printf \"%.4g\", c-d}'); "+
				"resize2fs \"$LV\" \"${NEW}G\"")
	case "btrfs":
		return wrapLVScript(lv,
			"GIB="+g+"; M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null); "+
				"if [ -z \"$M\" ]; then exit 1; fi; btrfs filesystem resize -\"${GIB}\"G \"$M\"")
	default:
		return "echo 'Encolher FS não suportado para " + fs + ".' >&2; exit 1"
	}
}

// LVRenameScript renomeia LV no VG.
func LVRenameScript(vg, oldName, newName string) string {
	return "set -e; VG=" + shellQuote(vg) + "; OLD=" + shellQuote(oldName) + "; NEW=" + shellQuote(newName) + "; " +
		"command -v lvrename >/dev/null 2>&1 || exit 1; lvrename \"$VG\" \"$OLD\" \"$NEW\""
}

// VgchangeScript activa ou desactiva VG (-ay / -an).
func VgchangeScript(vg string, activate bool) string {
	flag := "-an"
	if activate {
		flag = "-ay"
	}
	return "set -e; VG=" + shellQuote(vg) + "; command -v vgchange >/dev/null 2>&1 || exit 1; vgchange " + flag + " \"$VG\""
}

// FstrimScript executa fstrim no ponto de montagem do LV.
func FstrimScript(lv string) string {
	return wrapLVScript(lv,
		"command -v fstrim >/dev/null 2>&1 || exit 1; "+
			"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); "+
			"if [ -z \"$M\" ]; then echo 'Não montado.' >&2; exit 1; fi; fstrim -av \"$M\"")
}

// SmartctlScript consulta SMART (tenta com e sem sudo no host).
func SmartctlScript(dev string) string {
	d := shellQuote(dev)
	return "set +e; DEV=" + d + "; " +
		"if command -v smartctl >/dev/null 2>&1; then smartctl -a \"$DEV\" 2>&1 || smartctl -x \"$DEV\" 2>&1; " +
		"elif command -v smartctl.scsi >/dev/null 2>&1; then smartctl.scsi -a \"$DEV\" 2>&1; " +
		"else echo 'smartctl não instalado no servidor.' >&2; exit 1; fi"
}

// LVNameAndVG extrai nome LV e VG a partir do caminho e mapa lvs.
func LVNameAndVG(lvPath string, lvsBlock string) (vg, lvName string) {
	lvPath = strings.TrimSpace(lvPath)
	m := ParseLVPathToVG(lvsBlock)
	if v, ok := m[lvPath]; ok {
		vg = v
	}
	for _, r := range ParseLVSBlock(lvsBlock) {
		if r.Path == lvPath {
			if r.VG != "" {
				vg = r.VG
			}
			if r.LVName != "" {
				return vg, r.LVName
			}
		}
	}
	lvName = lvNameFromPath(lvPath, vg)
	return vg, lvName
}
