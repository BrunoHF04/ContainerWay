package appui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	dcontainer "github.com/docker/docker/api/types/container"
)

type dockerManagerRow struct {
	ID        string
	Label     string
	State     string
	Running   bool
	Restarting bool
}

func dockerStateLabelPT(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running":
		return "em execução"
	case "exited":
		return "encerrado"
	case "paused":
		return "pausado"
	case "restarting":
		return "reiniciando"
	case "dead":
		return "morto"
	case "created":
		return "criado"
	case "removing":
		return "removendo"
	default:
		if state == "" {
			return "desconhecido"
		}
		return state
	}
}

func buildDockerManagerRows(list []dcontainer.Summary) []dockerManagerRow {
	out := make([]dockerManagerRow, 0, len(list))
	for _, c := range list {
		if c.State != dcontainer.StateRunning && c.State != dcontainer.StateRestarting {
			continue
		}
		id := strings.TrimPrefix(c.ID, "sha256:")
		short := id
		if len(short) > 12 {
			short = short[:12]
		}
		disp := containerDisplayName(c)
		if disp == "" {
			disp = "(sem nome)"
		}
		st := string(c.State)
		stPT := dockerStateLabelPT(st)
		line := fmt.Sprintf("%s  ·  %s  ·  %s", truncateRunes(disp, 40), stPT, short)
		out = append(out, dockerManagerRow{
			ID:         c.ID,
			Label:      line,
			State:      st,
			Running:    c.State == dcontainer.StateRunning,
			Restarting: c.State == dcontainer.StateRestarting,
		})
	}
	return out
}

func (ui *explorer) showDockerContainerManager() {
	if ui.s == nil || ui.s.Docker == nil {
		dialog.ShowError(fmt.Errorf("cliente Docker indisponível"), ui.win)
		return
	}
	appendAuditLog("docker", "Diálogo de contêineres Docker aberto")

	rows := []dockerManagerRow{}
	listWidget := widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			lb := o.(*widget.Label)
			if id < 0 || int(id) >= len(rows) {
				lb.SetText("")
				return
			}
			lb.SetText(rows[id].Label)
		},
	)
	listWidget.HideSeparators = true

	hint := widget.NewLabel("Lista só contêineres em execução no servidor. Selecione um e use \"Reiniciar selecionado\", ou \"Reiniciar todos\" para reiniciar todos os da lista.")
	hint.Wrapping = fyne.TextWrapWord

	statusLbl := widget.NewLabel("")
	statusLbl.Wrapping = fyne.TextWrapWord

	var selectedID widget.ListItemID = -1

	btnRestartSel := widget.NewButtonWithIcon("Reiniciar selecionado", theme.MediaReplayIcon(), nil)
	btnRestartSel.Importance = widget.HighImportance
	btnRestartSel.Disable()

	btnRestartAll := widget.NewButtonWithIcon("Reiniciar todos", theme.ViewRefreshIcon(), nil)
	btnRestartAll.Importance = widget.WarningImportance

	refresh := func() {
		if ui.s == nil || ui.s.Docker == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		list, err := ui.s.Docker.ContainerList(ctx, dcontainer.ListOptions{All: false})
		if err != nil {
			fyne.Do(func() {
				statusLbl.SetText("Erro ao listar: " + err.Error())
				rows = nil
				listWidget.Refresh()
				listWidget.UnselectAll()
				selectedID = -1
				btnRestartSel.Disable()
			})
			return
		}
		next := buildDockerManagerRows(list)
		fyne.Do(func() {
			rows = next
			statusLbl.SetText(fmt.Sprintf("%d contêiner(es) em execução.", len(rows)))
			listWidget.Refresh()
			listWidget.UnselectAll()
			selectedID = -1
			btnRestartSel.Disable()
		})
	}

	btnRefresh := widget.NewButtonWithIcon("Atualizar lista", theme.ViewRefreshIcon(), func() {
		fyne.Do(func() { statusLbl.SetText("Atualizando…") })
		go refresh()
	})

	listWidget.OnSelected = func(id widget.ListItemID) {
		selectedID = id
		if id < 0 || int(id) >= len(rows) {
			btnRestartSel.Disable()
			return
		}
		btnRestartSel.Enable()
	}

	doRestartOne := func(containerID, humanName string, onDone func(err error)) {
		go func() {
			if ui.s == nil || ui.s.Docker == nil {
				fyne.Do(func() { onDone(fmt.Errorf("sessão indisponível")) })
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			err := ui.s.Docker.ContainerRestart(ctx, containerID, dcontainer.StopOptions{})
			fyne.Do(func() { onDone(err) })
			if err == nil {
				appendAuditLog("docker", "Contêiner reiniciado: "+humanName)
			}
		}()
	}

	btnRestartSel.OnTapped = func() {
		if selectedID < 0 || int(selectedID) >= len(rows) {
			return
		}
		r := rows[selectedID]
		dialog.ShowConfirm(
			"Reiniciar contêiner",
			fmt.Sprintf("Reiniciar o contêiner?\n\n%s", r.Label),
			func(ok bool) {
				if !ok {
					return
				}
				fyne.Do(func() { statusLbl.SetText("Reiniciando contêiner selecionado…") })
				doRestartOne(r.ID, r.Label, func(err error) {
					if err != nil {
						dialog.ShowError(fmt.Errorf("falha ao reiniciar: %w", err), ui.win)
					} else {
						dialog.ShowInformation("Docker", "Contêiner reiniciado com sucesso.", ui.win)
					}
					refresh()
				})
			},
			ui.win,
		)
	}

	btnRestartAll.OnTapped = func() {
		targets := append([]dockerManagerRow(nil), rows...)
		if len(targets) == 0 {
			dialog.ShowInformation("Docker", "Nenhum contêiner em execução na lista.", ui.win)
			return
		}
		dialog.ShowConfirm(
			"Reiniciar vários contêineres",
			fmt.Sprintf("Isso vai reiniciar os %d contêiner(es) da lista, um após o outro.\n\nContinuar?", len(targets)),
			func(ok bool) {
				if !ok {
					return
				}
				go func() {
					var errs []string
					for i, t := range targets {
						fyne.Do(func() {
							statusLbl.SetText(fmt.Sprintf("Reiniciando %d de %d…", i+1, len(targets)))
						})
						if ui.s == nil || ui.s.Docker == nil {
							errs = append(errs, "sessão indisponível")
							break
						}
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
						err := ui.s.Docker.ContainerRestart(ctx, t.ID, dcontainer.StopOptions{})
						cancel()
						if err != nil {
							errs = append(errs, fmt.Sprintf("%s: %v", truncateRunes(t.Label, 48), err))
						} else {
							appendAuditLog("docker", "Contêiner reiniciado (lote): "+t.Label)
						}
					}
					fyne.Do(func() {
						if len(errs) == 0 {
							dialog.ShowInformation("Docker", fmt.Sprintf("Concluído: %d contêiner(es) reiniciados.", len(targets)), ui.win)
						} else {
							dialog.ShowError(fmt.Errorf("alguns reinícios falharam:\n%s", strings.Join(errs, "\n")), ui.win)
						}
						refresh()
					})
				}()
			},
			ui.win,
		)
	}

	actions := fynecontainer.NewHBox(btnRefresh, btnRestartSel, btnRestartAll)

	body := fynecontainer.NewBorder(
		nil,
		fynecontainer.NewVBox(actions, statusLbl),
		nil,
		nil,
		fynecontainer.NewBorder(
			fynecontainer.NewPadded(hint),
			nil,
			nil,
			nil,
			listWidget,
		),
	)
	scroll := fynecontainer.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(720, 420))

	d := dialog.NewCustom(
		"Contêineres Docker no servidor",
		"Fechar",
		scroll,
		ui.win,
	)
	d.Resize(fyne.NewSize(760, 480))
	d.SetOnClosed(func() {
		appendAuditLog("docker", "Diálogo de contêineres Docker fechado")
	})
	d.Show()

	go refresh()
}
