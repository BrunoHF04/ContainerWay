package webapp

import (
	"fmt"
	"strings"
)

const servicesListScript = `
if ! command -v systemctl >/dev/null 2>&1; then
  echo "ERR|systemctl não disponível neste sistema"
  exit 0
fi
systemctl list-unit-files --type=service --no-legend --no-pager 2>/dev/null | while read -r unit load _; do
  [ -z "$unit" ] && continue
  echo "FILE|${unit}|${load}"
done
systemctl list-units --type=service --all --no-legend --no-pager 2>/dev/null | while read -r unit _ active sub desc; do
  [ -z "$unit" ] && continue
  echo "UNIT|${unit}|${active}|${sub}|${desc}"
done
`

type serviceRow struct {
	Unit        string `json:"unit"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Enabled     string `json:"enabled"`
	LoadState   string `json:"loadState"`
	Description string `json:"description"`
}

func parseServicesList(raw string) ([]serviceRow, error) {
	if strings.Contains(raw, "ERR|") {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ERR|") {
				msg := strings.TrimPrefix(line, "ERR|")
				if msg == "" {
					msg = "systemctl indisponível"
				}
				return nil, fmt.Errorf("%s", msg)
			}
		}
	}
	files := map[string]string{}
	units := map[string]struct {
		active, sub, desc string
	}{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "FILE|") {
			parts := strings.SplitN(line[5:], "|", 3)
			if len(parts) >= 2 {
				files[parts[0]] = parts[1]
			}
			continue
		}
		if strings.HasPrefix(line, "UNIT|") {
			parts := strings.SplitN(line[5:], "|", 4)
			if len(parts) < 3 {
				continue
			}
			desc := ""
			if len(parts) > 3 {
				desc = parts[3]
			}
			units[parts[0]] = struct {
				active, sub, desc string
			}{parts[1], parts[2], desc}
		}
	}
	if len(files) == 0 && len(units) == 0 {
		return nil, fmt.Errorf("nenhum serviço encontrado (systemd)")
	}
	seen := map[string]struct{}{}
	var out []serviceRow
	add := func(unit string, load string, u struct {
		active, sub, desc string
	}) {
		if unit == "" {
			return
		}
		if _, ok := seen[unit]; ok {
			return
		}
		seen[unit] = struct{}{}
		active := u.active
		sub := u.sub
		desc := u.desc
		if active == "" {
			active = "inactive"
		}
		enabled := load
		if enabled == "" {
			enabled = "—"
		}
		out = append(out, serviceRow{
			Unit:        unit,
			Active:      active,
			Sub:         sub,
			Enabled:     enabled,
			LoadState:   load,
			Description: desc,
		})
	}
	for unit, load := range files {
		add(unit, load, units[unit])
	}
	for unit, u := range units {
		if _, ok := files[unit]; ok {
			continue
		}
		add(unit, "", u)
	}
	return out, nil
}

func scriptServiceAction(unit, action string) string {
	q := shellQuote(unit)
	switch action {
	case "start":
		return "set -e\nsystemctl start " + q + "\n"
	case "stop":
		return "set -e\nsystemctl stop " + q + "\n"
	case "restart":
		return "set -e\nsystemctl restart " + q + "\n"
	case "enable":
		return "set -e\nsystemctl enable " + q + "\n"
	case "disable":
		return "set -e\nsystemctl disable " + q + "\n"
	default:
		return ""
	}
}
