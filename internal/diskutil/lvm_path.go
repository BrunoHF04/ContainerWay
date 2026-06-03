package diskutil

import "strings"

// MapperPathFromVG devolve o caminho /dev/mapper habitual para VG/LV com hífens.
func MapperPathFromVG(vg, lv string) string {
	vg = strings.TrimSpace(vg)
	lv = strings.TrimSpace(lv)
	if vg == "" || lv == "" {
		return ""
	}
	enc := func(s string) string { return strings.ReplaceAll(s, "-", "--") }
	return "/dev/mapper/" + enc(vg) + "-" + enc(lv)
}

// NormalizeLVDevicePath corrige caminhos de LV (ex.: lsblk /dev/ubuntu--vg-… → /dev/mapper/…).
func NormalizeLVDevicePath(path string, records []LVRecord) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	for _, r := range records {
		if lvPathsMatch(path, r) && strings.TrimSpace(r.Path) != "" {
			return strings.TrimSpace(r.Path)
		}
	}
	if strings.HasPrefix(path, "/dev/") && !strings.HasPrefix(path, "/dev/mapper/") {
		base := strings.TrimPrefix(path, "/dev/")
		if strings.Contains(base, "--") {
			return "/dev/mapper/" + base
		}
	}
	return path
}

func lvPathsMatch(input string, r LVRecord) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}
	if input == strings.TrimSpace(r.Path) {
		return true
	}
	if r.VG != "" && r.LVName != "" {
		direct := "/dev/" + r.VG + "/" + r.LVName
		if input == direct {
			return true
		}
		mp := MapperPathFromVG(r.VG, r.LVName)
		if input == mp {
			return true
		}
		if input == "/dev/"+strings.TrimPrefix(mp, "/dev/mapper/") {
			return true
		}
	}
	return false
}

// LVResolveShellPreamble função bash que resolve o caminho aceite por lvs/lvreduce/lvextend.
func LVResolveShellPreamble() string {
	return `resolve_lv_device() {
  local IN="$1" OUT
  [ -n "$IN" ] || { echo "$IN"; return 1; }
  if command -v lvs >/dev/null 2>&1 && lvs "$IN" >/dev/null 2>&1; then
    OUT=$(lvs --noheadings -o lv_path "$IN" 2>/dev/null | awk '{$1=$1};1' | head -n1)
    if [ -n "$OUT" ]; then echo "$OUT"; return 0; fi
  fi
  case "$IN" in
    /dev/mapper/*) ;;
    /dev/*--*)
      local MP="/dev/mapper/${IN#/dev/}"
      if [ -e "$MP" ]; then
        if command -v lvs >/dev/null 2>&1 && lvs "$MP" >/dev/null 2>&1; then
          OUT=$(lvs --noheadings -o lv_path "$MP" 2>/dev/null | awk '{$1=$1};1' | head -n1)
          if [ -n "$OUT" ]; then echo "$OUT"; return 0; fi
        fi
        echo "$MP"; return 0
      fi
      ;;
  esac
  echo "$IN"
  return 0
}
`
}

func wrapLVScript(lv string, body string) string {
	q := shellQuote(lv)
	return LVResolveShellPreamble() + "set -e; LV=$(resolve_lv_device " + q + "); " + body
}
