package appui

import (
	"strings"
)

// shellStartWithTerminalBanner devolve o comando remoto: cd, MOTD compacto com ANSI
// e bash de login interativo. (Corrige printf: %b só aceita um argumento.)
func shellStartWithTerminalBanner(currentDir, sshHost string) string {
	dir := strings.TrimSpace(currentDir)
	if dir == "" {
		dir = "/"
	}
	dirQ := shellQuote(dir)
	host := strings.TrimSpace(sshHost)

	var b strings.Builder
	b.WriteString("cd -- ")
	b.WriteString(dirQ)
	b.WriteString(" 2>/dev/null || cd / || true\n")
	b.WriteString("export TERM=xterm-256color\n")
	if host == "" {
		b.WriteString("CW_SSH_HOST=$(hostname 2>/dev/null || echo '?')\n")
	} else {
		b.WriteString("CW_SSH_HOST=")
		b.WriteString(shellQuote(host))
		b.WriteString("\n")
	}
	b.WriteString(remoteColorWelcomeShell())
	b.WriteString("\nexec bash -li")
	return "/bin/sh -lc " + shellQuote(b.String())
}

// remoteColorWelcomeShell: POSIX /bin/sh; printf com formatos válidos (%s, sem %b ilegal).
func remoteColorWelcomeShell() string {
	return `N=$(printf '\033[0m')
B=$(printf '\033[1m')
D=$(printf '\033[2m')
g=$(printf '\033[92m')
y=$(printf '\033[93m')
m=$(printf '\033[95m')
c=$(printf '\033[96m')
W=$(printf '\033[37m')

CW_USER=$(id -un 2>/dev/null || whoami 2>/dev/null || echo "?")
CW_NOW=$(date "+%a %d %b %H:%M:%S %z %Y" 2>/dev/null || date)
CW_PRETTY=""
if [ -r /etc/os-release ]; then
  . /etc/os-release 2>/dev/null
  CW_PRETTY="${PRETTY_NAME:-$NAME}"
fi
[ -z "$CW_PRETTY" ] && CW_PRETTY="$(uname -o 2>/dev/null || echo Linux)"
CW_KERN=$(uname -sr 2>/dev/null || echo "")

MT=0 MA=0 MF=0 ST=0 SF=0
while read -r a b _; do
  case $a in MemTotal:) MT=$b;; MemAvailable:) MA=$b;; MemFree:) MF=$b;; SwapTotal:) ST=$b;; SwapFree:) SF=$b;; esac
done < /proc/meminfo
[ "$MA" = 0 ] && MA=$MF
MEM_PCT="?"
if [ "$MT" -gt 0 ] 2>/dev/null; then MEM_PCT=$(( (MT - MA) * 100 / MT )); fi
SWAP_PCT="?"
if [ "$ST" -gt 0 ] 2>/dev/null; then SWAP_PCT=$(( (ST - SF) * 100 / ST )); else SWAP_PCT="-"; fi

L1="?" L2="?" L3="?"
read -r L1 L2 L3 _ </proc/loadavg 2>/dev/null || true
[ -z "$L1" ] && L1="?"
[ -z "$L2" ] && L2="?"
[ -z "$L3" ] && L3="?"

DF_T=0 DF_U=0
set -- $(df -B1 / 2>/dev/null | awk 'NR==2{print $2,$3}')
DF_T=${1:-0} DF_U=${2:-0}
DF_PCT="?"
if [ "$DF_T" -gt 0 ] 2>/dev/null; then DF_PCT=$(( DF_U * 100 / DF_T )); fi
DF_HT=$(df -h / 2>/dev/null | awk 'NR==2{print $2}')
DF_UH=$(df -h / 2>/dev/null | awk 'NR==2{print $3}')

CW_P=$(ls -d /proc/[0-9]* 2>/dev/null | wc -l | tr -d " ")
CW_W=$(who 2>/dev/null | wc -l | tr -d " ")
[ -z "$CW_W" ] && CW_W=0

CW_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
[ -z "$CW_IP" ] && CW_IP=$(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -n 1)

TW=""
if [ -r /sys/class/thermal/thermal_zone0/temp ]; then
  read -r T0 </sys/class/thermal/thermal_zone0/temp 2>/dev/null && [ -n "$T0" ] && TW=$((T0 / 1000))
fi

UP=$(cut -d. -f1 /proc/uptime 2>/dev/null || echo 0)
[ -z "$UP" ] && UP=0
UD=$((UP / 86400))
UH=$(((UP % 86400) / 3600))
UM=$(((UP % 3600) / 60))
UPS="${UD}d ${UH}h ${UM}m"
BOOT=$(uptime -s 2>/dev/null || echo "?")

CW_SHOW=$(pwd 2>/dev/null || echo /)
[ -z "$CW_SHOW" ] && CW_SHOW=/
[ "${#CW_SHOW}" -gt 56 ] && CW_SHOW="$(printf "%s" "$CW_SHOW" | cut -c1-53)..."
CW_LINE1=$(printf "%s" "$CW_SHOW" | tr -d "\r\n" | cut -c1-70)

printf "%b%s%b\n" "$g" " ─── ContainerWay · Terminal SSH ─── " "$N"
printf "%b%s%b@%b%s%b  %b%s%b\n" "$B" "$CW_USER" "$N" "$m" "$CW_SSH_HOST" "$N" "$c" "$CW_LINE1" "$N"
if [ -n "$CW_KERN" ]; then printf "%b%s%b\n" "$D" "$CW_KERN" "$N"; fi
printf "%b%s%b %s\n" "$W" "Sistema:" "$N" "$CW_PRETTY"
printf "%b%s%b %s\n" "$c" "Agora:" "$N" "$CW_NOW"
printf "%b\n" "${D}────────────────────────────────────────${N}"

LINE_A=$(printf "Carga %s %s %s | RAM %s%% | Swap %s%% | Proc %s | who %s" "$L1" "$L2" "$L3" "$MEM_PCT" "$SWAP_PCT" "$CW_P" "$CW_W")
printf "%s\n" "$LINE_A"

if [ -n "$DF_HT" ]; then
  LINE_B=$(printf "Disco / %s%% (%s/%s)" "$DF_PCT" "$DF_UH" "$DF_HT")
else
  LINE_B=$(printf "Disco / %s%%" "$DF_PCT")
fi
if [ -n "$TW" ]; then
  LINE_B="$LINE_B | Temp ${TW}°C"
fi
LINE_B="$LINE_B | IPv4 $CW_IP"
printf "%s\n" "$LINE_B"
printf "Boot %s | Uptime %s\n" "$BOOT" "$UPS"
printf "%b\n" "${D}────────────────────────────────────────${N}"
printf "%b %s %b\n" "$y" ">>> ContainerWay" "$N"
printf "\n"
`
}
