package diskutil

import "strings"

// ProbeScript sonda lsblk, df, mapa de dispositivos e LVM no host remoto.
const ProbeScript = `set +e
printf '%s\n' '===LSBLK_JSON==='
lsblk -J -b 2>/dev/null || printf '%s\n' '{"blockdevices":[]}'
printf '\n%s\n' '===DF==='
df -B1 -T 2>/dev/null || true
printf '\n%s\n' '===DF_DEVMAP==='
df -B1 -T 2>/dev/null | awk 'NR>1 && $1 ~ /^\/dev\// {print $1}' | sort -u | while IFS= read -r dev; do
  [ -z "$dev" ] && continue
  c=$(readlink -f "$dev" 2>/dev/null || printf '%s\n' "$dev")
  printf '%s\t%s\n' "$dev" "$c"
done
printf '\n%s\n' '===LVS==='
lvs -a -o lv_path,vg_name,lv_size,lv_attr,origin,lv_name --separator ';' --noheadings --units g 2>&1 || true
printf '\n%s\n' '===VGS==='
vgs -o vg_name,vg_size,vg_free --separator ';' --noheadings --units g 2>&1 || true
printf '\n%s\n' '===PVS==='
pvs 2>&1 || true
printf '\n%s\n' '===FIM==='
`

// SplitProbe separa secções da saída do script de sondagem.
func SplitProbe(raw string) (lsblkJSON, dfBlock, dfDevMapBlock, lvsBlock, vgsBlock, pvsBlock string) {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	between := func(startTag, endTag string) string {
		i := strings.Index(s, startTag)
		if i < 0 {
			return ""
		}
		rest := s[i+len(startTag):]
		rest = strings.TrimLeft(rest, "\n")
		if endTag == "" {
			return strings.TrimSpace(rest)
		}
		j := strings.Index(rest, endTag)
		if j < 0 {
			return strings.TrimSpace(rest)
		}
		return strings.TrimSpace(rest[:j])
	}
	lsblkJSON = between("===LSBLK_JSON===", "===DF===")
	if strings.Contains(s, "===DF_DEVMAP===") {
		dfBlock = between("===DF===", "===DF_DEVMAP===")
		dfDevMapBlock = between("===DF_DEVMAP===", "===LVS===")
	} else {
		dfBlock = between("===DF===", "===LVS===")
		dfDevMapBlock = ""
	}
	lvsBlock = between("===LVS===", "===VGS===")
	vgsBlock = between("===VGS===", "===PVS===")
	pvsBlock = between("===PVS===", "===FIM===")
	return lsblkJSON, dfBlock, dfDevMapBlock, lvsBlock, vgsBlock, pvsBlock
}

// BuildTechnicalText monta a saída bruta para o separador «Detalhe técnico».
func BuildTechnicalText(dfBlock, lvsBlock, vgsBlock, pvsBlock string) string {
	var b strings.Builder
	b.WriteString("===DF===\n")
	b.WriteString(strings.TrimSpace(dfBlock))
	b.WriteString("\n\n===LVS===\n")
	b.WriteString(strings.TrimSpace(lvsBlock))
	b.WriteString("\n\n===VGS===\n")
	b.WriteString(strings.TrimSpace(vgsBlock))
	if strings.TrimSpace(pvsBlock) != "" {
		b.WriteString("\n\n===PVS===\n")
		b.WriteString(strings.TrimSpace(pvsBlock))
	}
	b.WriteString("\n\n===FIM===\n")
	return b.String()
}
