package appui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var (
	modernThemeOnce sync.Once
	modernThemeInst fyne.Theme
	forcedLightOnce sync.Once
	forcedLightInst fyne.Theme
)

// modernTheme aplica visual escuro com camadas (azul‑ardósia), acento teal
// e estados semânticos mais visíveis que o tema base.
type modernTheme struct {
	base fyne.Theme
}

// forcedLightTheme devolve uma única instância do tema claro (theme.LightTheme() alocava de novo a cada SetTheme).
func forcedLightTheme() fyne.Theme {
	forcedLightOnce.Do(func() {
		forcedLightInst = theme.LightTheme()
	})
	return forcedLightInst
}

// newModernTheme devolve sempre a mesma instância (evita SetTheme com ponteiro novo
// a cada frame/listener e reduz trabalho de layout no driver).
func newModernTheme() fyne.Theme {
	modernThemeOnce.Do(func() {
		modernThemeInst = &modernTheme{base: theme.DarkTheme()}
	})
	return modernThemeInst
}

// Color executa parte da logica deste modulo.
func (m *modernTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	// Camadas de superfície
	case theme.ColorNameBackground:
		return color.NRGBA{R: 11, G: 17, B: 32, A: 255}
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 17, G: 24, B: 39, A: 255}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 100, G: 116, B: 139, A: 255}

	// Acentos e ação
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 45, G: 212, B: 191, A: 255}
	case theme.ColorNameForegroundOnPrimary:
		return color.NRGBA{R: 11, G: 17, B: 32, A: 255}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 147, G: 197, B: 253, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 45, G: 212, B: 191, A: 210}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 13, G: 148, B: 136, A: 150}

	// Botões e inputs
	case theme.ColorNameButton:
		return color.NRGBA{R: 30, G: 41, B: 59, A: 255}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 30, G: 41, B: 59, A: 200}
	case theme.ColorNameHover:
		return color.NRGBA{R: 51, G: 65, B: 85, A: 255}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 23, G: 33, B: 52, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 15, G: 23, B: 42, A: 255}
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 51, G: 65, B: 85, A: 255}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 17, G: 24, B: 39, A: 252}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 15, G: 23, B: 42, A: 248}

	// Estrutura e scroll
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 51, G: 65, B: 85, A: 255}
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 71, G: 85, B: 105, A: 220}
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 15, G: 23, B: 42, A: 255}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 110}

	// Feedback
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 52, G: 211, B: 153, A: 255}
	case theme.ColorNameForegroundOnSuccess:
		return color.NRGBA{R: 11, G: 17, B: 32, A: 255}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 251, G: 191, B: 36, A: 255}
	case theme.ColorNameForegroundOnWarning:
		return color.NRGBA{R: 11, G: 17, B: 32, A: 255}
	case theme.ColorNameError:
		return color.NRGBA{R: 248, G: 113, B: 113, A: 255}
	case theme.ColorNameForegroundOnError:
		return color.NRGBA{R: 11, G: 17, B: 32, A: 255}
	}
	return m.base.Color(n, v)
}

// Font executa parte da logica deste modulo.
func (m *modernTheme) Font(style fyne.TextStyle) fyne.Resource {
	return m.base.Font(style)
}

// Icon executa parte da logica deste modulo.
func (m *modernTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return m.base.Icon(n)
}

// Size executa parte da logica deste modulo.
func (m *modernTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding:
		return m.base.Size(n) * 1.05
	case theme.SizeNameInnerPadding:
		return m.base.Size(n) * 1.05
	case theme.SizeNameSeparatorThickness:
		t := m.base.Size(n)
		if t < 1 {
			return 1
		}
		return t
	}
	return m.base.Size(n)
}
