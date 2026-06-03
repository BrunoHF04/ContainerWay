package diskutil

import (
	"fmt"
	"strconv"
)

// Gigabytes decimais (UI) vs sufixo G do LVM (potência de 2, 1024³ bytes).
const (
	bytesPerDecimalGB = 1_000_000_000
	bytesPerLVMG      = 1024 * 1024 * 1024
)

// UserGBToLVMG converte GB da interface para o valor usado em lvextend/lvreduce -LG.
func UserGBToLVMG(gb float64) float64 {
	if gb <= 0 {
		return 0
	}
	return gb * float64(bytesPerDecimalGB) / float64(bytesPerLVMG)
}

// LVMGToUserGB converte tamanho reportado pelo LVM (--units g) para GB decimais na UI.
func LVMGToUserGB(lvmG float64) float64 {
	if lvmG <= 0 {
		return 0
	}
	return lvmG * float64(bytesPerLVMG) / float64(bytesPerDecimalGB)
}

// BytesToUserGB converte bytes para gigabytes decimais.
func BytesToUserGB(bytes uint64) float64 {
	if bytes == 0 {
		return 0
	}
	return float64(bytes) / float64(bytesPerDecimalGB)
}

// FormatLVMSizeG formata um valor para o sufixo G do LVM (já em unidades LVM).
func FormatLVMSizeG(g float64) string {
	return strconv.FormatFloat(g, 'f', -1, 64)
}

// FormatUserGB formata gigabytes decimais com sufixo «GB».
func FormatUserGB(gb float64) string {
	if gb <= 0 {
		return "—"
	}
	return FormatLVMSizeG(gb) + " GB"
}

// FormatBytesDecimal formata bytes em KB/MB/GB decimais (1000).
func FormatBytesDecimal(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	const u = 1000.0
	switch {
	case n < 1_000_000:
		return fmt.Sprintf("%.1f KB", f/u)
	case n < 1_000_000_000:
		return fmt.Sprintf("%.1f MB", f/(u*u))
	default:
		return fmt.Sprintf("%.1f GB", f/(u*u*u))
	}
}
