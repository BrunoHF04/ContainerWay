package appui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type automationPreset struct {
	Title       string
	Description string
	Trigger     string
	Action      string
	Enabled     bool
}

// showAutomationCenter abre a tela inicial da central de automações.
func (ui *explorer) showAutomationCenter() {
	appendAuditLog("automacao", "Central de automações aberta")

	host := strings.TrimSpace(ui.connCreds.Host)
	if host == "" {
		host = "(host não informado)"
	}

	hint := widget.NewLabel(
		"Crie regras com gatilho e ação para reduzir tarefas manuais. Esta é a base inicial do módulo, pronta para evoluir com persistência e execução real.",
	)
	hint.Wrapping = fyne.TextWrapWord

	presets := []automationPreset{
		{
			Title:       "Auto-restart de contêiner crítico",
			Description: "Reinicia automaticamente serviço crítico quando o estado sai de execução.",
			Trigger:     "Container parado por mais de 20s",
			Action:      "docker restart + alerta por e-mail",
			Enabled:     true,
		},
		{
			Title:       "Proteção de disco",
			Description: "Executa limpeza de logs temporários quando uso de disco passa do limite.",
			Trigger:     "Disco /var acima de 85%",
			Action:      "Script de limpeza + notificação",
			Enabled:     false,
		},
		{
			Title:       "Diagnóstico pós-erro",
			Description: "Coleta evidências quando um serviço principal apresenta falha.",
			Trigger:     "Falha repetida do serviço (3x em 10 min)",
			Action:      "Gerar pacote de diagnóstico + abrir incidente",
			Enabled:     true,
		},
	}

	totalLbl := widget.NewLabel("")
	totalLbl.Wrapping = fyne.TextWrapOff
	search := widget.NewEntry()
	search.SetPlaceHolder("Pesquisar automações (nome, gatilho, ação)…")
	btnHelp := widget.NewButtonWithIcon("", theme.HelpIcon(), func() {
		dialog.ShowInformation(
			"Sobre a Central de automações",
			"Esta tela serve para criar regras operacionais com gatilho + ação.\n\n"+
				"Já disponível:\n"+
				"- Visualização e pesquisa de automações\n"+
				"- Estrutura de interface para criar/organizar regras\n\n"+
				"Pendente (motor):\n"+
				"- Persistência das regras no host\n"+
				"- Executor em segundo plano para monitorar gatilhos\n"+
				"- Disparo de ações reais (SSH, Docker, alertas)\n\n"+
				"Próximo passo recomendado: implementar o motor com loop de execução e histórico de eventos.",
			ui.win,
		)
	})
	btnHelp.Importance = widget.LowImportance

	filtered := append([]automationPreset(nil), presets...)
	list := widget.NewList(
		func() int { return len(filtered) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("automação")
			title.TextStyle = fyne.TextStyle{Bold: true}
			title.Wrapping = fyne.TextWrapWord
			desc := widget.NewLabel("descrição")
			desc.Wrapping = fyne.TextWrapWord
			meta := widget.NewLabel("gatilho/ação")
			meta.Wrapping = fyne.TextWrapWord
			body := container.NewVBox(
				title,
				widget.NewSeparator(),
				desc,
				meta,
			)
			return panelCard(container.NewPadded(body))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || int(id) >= len(filtered) {
				return
			}
			row := filtered[id]
			card := obj.(*fyne.Container)
			padded := card.Objects[1].(*fyne.Container)
			box := padded.Objects[0].(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			desc := box.Objects[2].(*widget.Label)
			meta := box.Objects[3].(*widget.Label)

			status := "Desativada"
			if row.Enabled {
				status = "Ativa"
			}
			title.SetText(fmt.Sprintf("%s  [%s]", row.Title, status))
			desc.SetText(row.Description)
			meta.SetText(fmt.Sprintf("Gatilho: %s\nAção: %s", row.Trigger, row.Action))
		},
	)
	for i := 0; i < len(filtered); i++ {
		list.SetItemHeight(widget.ListItemID(i), 156)
	}
	list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || int(id) >= len(filtered) {
			return
		}
		row := filtered[id]
		dialog.ShowInformation(
			"Detalhes da automação",
			fmt.Sprintf(
				"%s\n\nDescrição: %s\n\nGatilho: %s\nAção: %s\n\nPróximo passo: conectar esta automação ao motor de execução.",
				row.Title,
				row.Description,
				row.Trigger,
				row.Action,
			),
			ui.win,
		)
	}

	applyFilter := func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		filtered = filtered[:0]
		for _, p := range presets {
			blob := strings.ToLower(strings.Join([]string{p.Title, p.Description, p.Trigger, p.Action}, " "))
			if q == "" || strings.Contains(blob, q) {
				filtered = append(filtered, p)
			}
		}
		totalLbl.SetText(fmt.Sprintf("Mostrando %d automação(ões) no host %s.", len(filtered), host))
		list.Refresh()
		for i := 0; i < len(filtered); i++ {
			list.SetItemHeight(widget.ListItemID(i), 156)
		}
		list.UnselectAll()
	}
	search.OnChanged = applyFilter
	applyFilter("")

	btnNew := widget.NewButtonWithIcon("Nova automação", theme.ContentAddIcon(), func() {
		dialog.ShowInformation(
			"Nova automação",
			"Em seguida, podemos implementar o assistente para criar regras com gatilho + ação e salvar no host.",
			ui.win,
		)
	})
	btnNew.Importance = widget.HighImportance

	btnRunbook := widget.NewButtonWithIcon("Executar runbook", theme.MediaPlayIcon(), func() {
		dialog.ShowInformation(
			"Runbook",
			"Futuro fluxo: executar playbook operacional com validações e confirmação antes de aplicar mudanças.",
			ui.win,
		)
	})
	btnRunbook.Importance = widget.MediumImportance

	btnPolicies := widget.NewButtonWithIcon("Políticas", theme.WarningIcon(), func() {
		dialog.ShowInformation(
			"Políticas e compliance",
			"Futuro fluxo: monitorar violações (sem limite de recursos, uso de root, imagem não aprovada) e sugerir correções.",
			ui.win,
		)
	})
	btnPolicies.Importance = widget.MediumImportance

	top := container.NewVBox(
		hint,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, btnHelp, search),
		totalLbl,
		widget.NewSeparator(),
	)
	actions := container.NewHBox(
		btnNew,
		btnRunbook,
		btnPolicies,
		layout.NewSpacer(),
	)
	body := container.NewBorder(
		container.NewPadded(top),
		container.NewPadded(container.NewVBox(widget.NewSeparator(), actions)),
		nil,
		nil,
		container.NewPadded(container.NewVScroll(list)),
	)

	ui.openSettingsFullscreenWithBack("Central de automações", body, nil)
}
