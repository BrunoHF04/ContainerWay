package appui

import (
	"strings"

	"fyne.io/fyne/v2"
)

// allStrings agrega textos do hub e do explorador por código de idioma.
var allStrings map[string]map[string]string

func mergeLang(dst map[string]string, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func init() {
	allStrings = make(map[string]map[string]string, 3)
	for _, lang := range []string{langPTBR, langEN, langES} {
		m := make(map[string]string, 512)
		mergeLang(m, hubStrings[lang])
		mergeLang(m, explorerStrings[lang])
		mergeLang(m, uiStrings[lang])
		mergeLang(m, manualStrings[lang])
		mergeLang(m, moduleStrings[lang])
		allStrings[lang] = m
	}
}

// tr devolve o texto no idioma guardado nas preferências (toda a UI registada nos mapas de locale).
func tr(key string) string {
	a := fyne.CurrentApp()
	lang := loadUILanguage(a)
	if m, ok := allStrings[lang]; ok {
		if s, ok2 := m[key]; ok2 {
			return s
		}
	}
	if m, ok := allStrings[langPTBR]; ok {
		if s, ok2 := m[key]; ok2 {
			return s
		}
	}
	return key
}

// hubSearchBlob junta palavras-chave de pesquisa nos três idiomas para o filtro do hub.
func hubSearchBlob(field string) string {
	var b strings.Builder
	for _, lang := range []string{langPTBR, langEN, langES} {
		s := hubStrings[lang][field]
		b.WriteString(s)
		b.WriteByte(' ')
	}
	return strings.ToLower(strings.TrimSpace(b.String()))
}
