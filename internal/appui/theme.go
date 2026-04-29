package appui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// modernTheme aplica um visual escuro com acento ciano sobre o tema base.
type modernTheme struct {
	base fyne.Theme
}

// newModernTheme executa parte da logica deste modulo.
func newModernTheme() fyne.Theme {
	return &modernTheme{base: theme.DarkTheme()}
}

// Color executa parte da logica deste modulo.
func (m *modernTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 56, G: 189, B: 248, A: 255}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 125, G: 211, B: 252, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 56, G: 189, B: 248, A: 200}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 8, G: 91, B: 120, A: 160}
	case theme.ColorNameButton:
		return color.NRGBA{R: 38, G: 48, B: 64, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 24, G: 30, B: 40, A: 255}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 22, G: 28, B: 38, A: 250}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 28, G: 35, B: 48, A: 240}
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
