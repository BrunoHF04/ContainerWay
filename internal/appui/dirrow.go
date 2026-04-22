package appui

import (
	"fyne.io/fyne/v2"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// dirListRow é a célula da lista: um toque seleciona; duplo clique do sistema abre a pasta (fyne.DoubleTappable).
type dirListRow struct {
	widget.BaseWidget

	ui     *explorer
	left   bool
	itemID widget.ListItemID
	box    *fyne.Container
}

var (
	_ fyne.Tappable       = (*dirListRow)(nil)
	_ fyne.DoubleTappable = (*dirListRow)(nil)
	_ fyne.SecondaryTappable = (*dirListRow)(nil)
)

func newDirListRow(ui *explorer, left bool) *dirListRow {
	r := &dirListRow{
		ui: ui,
		left: left,
		box: fynecontainer.NewHBox(
			widget.NewIcon(nil),
			widget.NewLabel("Nome"),
			layout.NewSpacer(),
			widget.NewLabel("Tamanho"),
		),
	}
	r.ExtendBaseWidget(r)
	return r
}

func (r *dirListRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.box)
}

func (r *dirListRow) Tapped(_ *fyne.PointEvent) {
	if r.left {
		r.ui.leftList.Select(r.itemID)
	} else {
		r.ui.rightList.Select(r.itemID)
	}
}

func (r *dirListRow) DoubleTapped(_ *fyne.PointEvent) {
	id := r.itemID
	left := r.left
	ui := r.ui
	// Adia para fora do ciclo de evento do duplo clique (evita reentrância na lista).
	fyne.Do(func() {
		if left {
			if id < 0 || int(id) >= len(ui.leftRows) {
				return
			}
			if !ui.leftRows[id].IsDir {
				return
			}
			ui.leftList.Select(id)
			ui.leftSel = int(id)
			ui.onLeftActivate()
			return
		}
		if ui.rightList == nil {
			return
		}
		if id < 0 || int(id) >= len(ui.rightRows) {
			return
		}
		if !ui.rightRows[id].IsDir {
			return
		}
		ui.rightList.Select(id)
		ui.rightSel = int(id)
		ui.onRightActivate()
	})
}

func (r *dirListRow) TappedSecondary(ev *fyne.PointEvent) {
	r.ui.showRowContextMenu(r.left, r.itemID, ev.AbsolutePosition)
}
