package appui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	dcontainer "github.com/docker/docker/api/types/container"

	"containerway/internal/containerfs"
	"containerway/internal/fsutil"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
	"containerway/internal/session"
	"containerway/internal/tarxfer"
	"containerway/internal/transfer"
)

// Run inicia a aplicação Fyne.
func Run() {
	a := app.NewWithID("io.containerway.app")
	a.Settings().SetTheme(newModernTheme())
	if ico := appWindowIcon(); ico != nil {
		a.SetIcon(ico)
	}
	w := a.NewWindow("ContainerWay")
	if ico := appWindowIcon(); ico != nil {
		w.SetIcon(ico)
	}
	w.SetContent(buildLogin(w))
	setLoginWindow(w)
	w.ShowAndRun()
}

func setLoginWindow(w fyne.Window) {
	w.Resize(fyne.NewSize(500, 600))
}

func setExplorerWindow(w fyne.Window) {
	w.Resize(fyne.NewSize(1100, 720))
}

func goToLogin(w fyne.Window) {
	setLoginWindow(w)
	w.SetContent(buildLogin(w))
}

func buildLogin(w fyne.Window) fyne.CanvasObject {
	host := widget.NewEntry()
	host.SetPlaceHolder("ex.: 192.168.1.10 ou servidor:22")
	user := widget.NewEntry()
	user.SetPlaceHolder("usuário no servidor (SSH)")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("senha (opcional se usar chave)")
	keyPath := widget.NewEntry()
	keyPath.SetPlaceHolder("caminho para .pem / id_rsa (OpenSSH)")
	keyPass := widget.NewPasswordEntry()
	keyPass.SetPlaceHolder("senha da chave privada (se houver)")
	knownHosts := widget.NewEntry()
	knownHosts.SetPlaceHolder("known_hosts (opcional); vários: caminho1|caminho2")
	insecureHost := widget.NewCheck("Ignorar chave de host SSH (inseguro)", nil)
	insecureHost.SetChecked(true)
	parallelJobsEntry := widget.NewEntry()
	parallelJobsEntry.SetText("3")
	parallelJobsEntry.SetPlaceHolder("transferências em paralelo (1–16)")
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	saveSecrets := widget.NewCheck("Salvar senha/chave nesta conexão (uso local)", nil)
	connName := widget.NewEntry()
	connName.SetPlaceHolder("Nome da conexão (ex.: Produção)")
	profileSelect := widget.NewSelect([]string{"Nova conexão…"}, nil)
	profileSelect.SetSelected("Nova conexão…")

	profiles, loadErr := loadSavedConnections()
	if loadErr != nil {
		status.SetText(loadErr.Error())
	}

	rebuildProfileOptions := func(selected string) {
		opts := []string{"Nova conexão…"}
		for _, p := range profiles {
			opts = append(opts, p.Name)
		}
		profileSelect.Options = opts
		profileSelect.Refresh()
		if selected != "" {
			profileSelect.SetSelected(selected)
		} else {
			profileSelect.SetSelected("Nova conexão…")
		}
	}

	applyProfile := func(c savedConnection) {
		connName.SetText(c.Name)
		host.SetText(c.Host)
		user.SetText(c.User)
		pass.SetText(c.Password)
		keyPath.SetText(c.KeyPath)
		keyPass.SetText(c.KeyPass)
		knownHosts.SetText(c.KnownHosts)
		insecureHost.SetChecked(c.InsecureHostKey)
		if strings.TrimSpace(c.ParallelJobs) == "" {
			parallelJobsEntry.SetText("3")
		} else {
			parallelJobsEntry.SetText(c.ParallelJobs)
		}
		saveSecrets.SetChecked(c.Password != "" || c.KeyPass != "")
	}

	profileSelect.OnChanged = func(sel string) {
		if sel == "Nova conexão…" {
			return
		}
		c, ok := findConnectionByName(profiles, sel)
		if !ok {
			status.SetText("Conexão selecionada não encontrada.")
			return
		}
		applyProfile(c)
		status.SetText("Conexão carregada: " + c.Name)
	}

	saveProfile := widget.NewButtonWithIcon("Salvar", theme.DocumentSaveIcon(), func() {
		name := strings.TrimSpace(connName.Text)
		if name == "" {
			dialog.ShowInformation("ContainerWay", "Informe um nome para salvar a conexão.", w)
			return
		}
		saved := savedConnection{
			Name:            name,
			Host:            strings.TrimSpace(host.Text),
			User:            strings.TrimSpace(user.Text),
			KeyPath:         strings.TrimSpace(keyPath.Text),
			KnownHosts:      strings.TrimSpace(knownHosts.Text),
			InsecureHostKey: insecureHost.Checked,
			ParallelJobs:    strings.TrimSpace(parallelJobsEntry.Text),
		}
		if saveSecrets.Checked {
			saved.Password = pass.Text
			saved.KeyPass = keyPass.Text
		}
		profiles = upsertConnection(profiles, saved)
		if err := saveConnections(profiles); err != nil {
			dialog.ShowError(err, w)
			return
		}
		rebuildProfileOptions(name)
		status.SetText("Conexão salva: " + name)
	})

	deleteProfile := widget.NewButtonWithIcon("Excluir", theme.DeleteIcon(), func() {
		target := strings.TrimSpace(profileSelect.Selected)
		if target == "" || target == "Nova conexão…" {
			dialog.ShowInformation("ContainerWay", "Selecione uma conexão salva para excluir.", w)
			return
		}
		dialog.ShowConfirm("Excluir conexão", fmt.Sprintf("Deseja excluir a conexão \"%s\"?", target), func(ok bool) {
			if !ok {
				return
			}
			profiles = removeConnectionByName(profiles, target)
			if err := saveConnections(profiles); err != nil {
				dialog.ShowError(err, w)
				return
			}
			rebuildProfileOptions("")
			status.SetText("Conexão excluída: " + target)
		}, w)
	})
	deleteProfile.Importance = widget.DangerImportance
	rebuildProfileOptions("")

	formConn := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Host", Widget: host},
			{Text: "Usuário", Widget: user},
			{Text: "Senha", Widget: pass},
		},
	}
	formAdv := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Chave PEM / PPK", Widget: keyPath},
			{Text: "Senha da chave", Widget: keyPass},
			{Text: "known_hosts", Widget: knownHosts},
			{Text: "", Widget: insecureHost},
			{Text: "Paralelismo", Widget: parallelJobsEntry},
		},
	}

	body := fynecontainer.NewVBox(
		widget.NewLabelWithStyle("Conexões salvas", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		fynecontainer.NewHBox(profileSelect, saveProfile, deleteProfile),
		connName,
		saveSecrets,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Conexão", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		formConn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Chave e segurança", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		formAdv,
	)

	connect := widget.NewButtonWithIcon("Conectar", theme.LoginIcon(), func() {
		status.SetText("Conectando…")
		creds := session.Credentials{
			Host:              host.Text,
			User:              user.Text,
			Password:          pass.Text,
			KeyPath:           strings.TrimSpace(keyPath.Text),
			KeyPass:           keyPass.Text,
			KnownHostsFiles:   splitKnownHostsFiles(knownHosts.Text),
			InsecureHostKey:   insecureHost.Checked,
		}
		pJobs := parseParallelWorkers(parallelJobsEntry.Text)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			sess, err := session.Connect(ctx, creds)
			if err != nil {
				fyne.Do(func() {
					status.SetText(err.Error())
					dialog.ShowError(err, w)
				})
				return
			}
			fyne.Do(func() {
				w.SetContent(buildExplorer(w, sess, pJobs))
				setExplorerWindow(w)
			})
		}()
	})
	connect.Importance = widget.HighImportance

	cardInner := fynecontainer.NewVBox(
		body,
		widget.NewSeparator(),
		connect,
		status,
	)
	card := widget.NewCard(
		"ContainerWay",
		"SSH · SFTP · Docker remoto sem expor a API em TCP",
		cardInner,
	)

	return fynecontainer.NewCenter(fynecontainer.NewPadded(card))
}

type explorer struct {
	win fyne.Window
	s   *session.Session
	hfs *hostfs.FS
	cfs *containerfs.FS

	leftPath  string
	rightPath string
	hostMode  bool

	containerOpts []string
	containerIDs  []string

	leftRows  []fsutil.DirEntry
	rightRows []fsutil.DirEntry
	leftAll   []fsutil.DirEntry
	rightAll  []fsutil.DirEntry
	leftSel   int
	rightSel  int

	leftList    *widget.List
	rightList   *widget.List
	leftPathLbl *widget.Label
	breadcrumb  *widget.Label
	leftCrumbs  *fyne.Container
	rightCrumbs *fyne.Container
	ctxSelect   *widget.Select
	leftQuick   *widget.Select
	rightQuick  *widget.Select
	status      *widget.Label
	progress    *widget.ProgressBar
	lastJobText *widget.Label
	selectionInfo *widget.Label
	leftSearch  *widget.Entry
	rightSearch *widget.Entry
	leftBack    []string
	rightBack   []string

	tm            *transfer.Manager
	parallelJobs int
	activePane    string
	btnOpenLocal  *widget.Button
	btnOpenRemote *widget.Button
	btnUp         *widget.Button
	btnDown       *widget.Button

	// Evita aplicar listagens antigas se o usuário mudar de pasta/contexto a meio.
	rightRefreshSeq atomic.Uint64

	remoteEditMu       sync.Mutex
	remoteEditSessions map[string]*remoteEditSession
}

type remoteEditSession struct {
	tempPath    string
	remotePath  string
	hostMode    bool
	containerID string
	lastMod     time.Time
	lastSize    int64
	stopped     atomic.Bool
}

func splitKnownHostsFiles(s string) []string {
	var out []string
	for _, part := range strings.Split(s, "|") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseParallelWorkers(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v < 1 {
		return 3
	}
	if v > 16 {
		return 16
	}
	return v
}

// truncateRunes encurta texto para caber em menus (UTF-8 seguro).
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}

// containerDisplayName devolve um nome curto para o menu (Swarm/Compose ou prefixo do nome).
func containerDisplayName(c dcontainer.Summary) string {
	if c.Labels != nil {
		if v := strings.TrimSpace(c.Labels["com.docker.swarm.service.name"]); v != "" {
			return v
		}
		if v := strings.TrimSpace(c.Labels["com.docker.compose.service"]); v != "" {
			return v
		}
	}
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimSpace(strings.TrimPrefix(c.Names[0], "/"))
	}
	if name == "" {
		return ""
	}
	// Nome longo estilo Swarm (stack_serviço.hash…): usa só o primeiro segmento.
	if len(name) > 48 {
		if i := strings.IndexByte(name, '.'); i > 0 {
			return name[:i]
		}
	}
	return name
}

func buildExplorer(w fyne.Window, s *session.Session, parallelJobs int) fyne.CanvasObject {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := s.Docker.ContainerList(ctx, dcontainer.ListOptions{All: false})
	if err != nil {
		errLabel := widget.NewLabel(fmt.Sprintf("Não foi possível usar o Docker neste servidor: %v", err))
		errLabel.Wrapping = fyne.TextWrapWord
		closeBtn := widget.NewButtonWithIcon("Encerrar sessão", theme.LogoutIcon(), func() {
			s.Close()
			goToLogin(w)
		})
		closeBtn.Importance = widget.DangerImportance
		inner := fynecontainer.NewVBox(errLabel, widget.NewSeparator(), closeBtn)
		return fynecontainer.NewPadded(widget.NewCard("Docker", "Verifique as permissões em /var/run/docker.sock", inner))
	}

	ui := &explorer{
		win:           w,
		s:             s,
		hfs:           &hostfs.FS{Client: s.SFTP},
		leftPath:      homeOrRoot(),
		rightPath:     "/",
		hostMode:      true,
		containerOpts: []string{"Pastas do servidor (fora dos contêineres)"},
		containerIDs:  []string{""},
		leftSel:        -1,
		rightSel:       -1,
		tm:             &transfer.Manager{},
		parallelJobs:   parallelJobs,
		activePane:     "left",
		remoteEditSessions: map[string]*remoteEditSession{},
	}
	for _, c := range list {
		// Reforço no cliente: só o que está “vivo” (como no docker ps sem -a).
		if c.State != dcontainer.StateRunning && c.State != dcontainer.StateRestarting {
			continue
		}
		disp := containerDisplayName(c)
		id := strings.TrimPrefix(c.ID, "sha256:")
		short := id
		if len(short) > 12 {
			short = short[:12]
		}
		var label string
		if disp == "" {
			label = fmt.Sprintf("Contêiner sem nome (ID %s)", short)
		} else {
			label = fmt.Sprintf("%s (ID %s)", truncateRunes(disp, 52), short)
		}
		ui.containerOpts = append(ui.containerOpts, label)
		ui.containerIDs = append(ui.containerIDs, c.ID)
	}

	ui.breadcrumb = widget.NewLabel("")
	ui.breadcrumb.Wrapping = fyne.TextWrapWord
	ui.leftCrumbs = fynecontainer.NewHBox()
	ui.rightCrumbs = fynecontainer.NewHBox()
	ui.leftPathLbl = widget.NewLabel("")
	ui.leftPathLbl.Wrapping = fyne.TextWrapWord
	ui.status = widget.NewLabel("")
	ui.status.Wrapping = fyne.TextWrapWord
	ui.progress = widget.NewProgressBar()
	ui.progress.Hide()
	ui.lastJobText = widget.NewLabel("")
	ui.leftSearch = widget.NewEntry()
	ui.leftSearch.SetPlaceHolder("Pesquisar no computador local")
	ui.rightSearch = widget.NewEntry()
	ui.rightSearch.SetPlaceHolder("Pesquisar no lado do servidor")
	ui.selectionInfo = widget.NewLabel("Selecione um arquivo ou pasta para ver ações disponíveis.")
	ui.selectionInfo.Wrapping = fyne.TextWrapWord

	ui.leftList = widget.NewList(
		func() int { return len(ui.leftRows) },
		func() fyne.CanvasObject {
			return newDirListRow(ui, true)
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*dirListRow)
			row.itemID = id
			box := row.box
			if id < 0 || id >= len(ui.leftRows) {
				return
			}
			e := ui.leftRows[id]
			ic := box.Objects[0].(*widget.Icon)
			l1 := box.Objects[1].(*widget.Label)
			l2 := box.Objects[3].(*widget.Label)
			switch {
			case e.Name == "..":
				ic.SetResource(theme.NavigateBackIcon())
			case e.IsDir:
				ic.SetResource(theme.FolderIcon())
			default:
				ic.SetResource(theme.DocumentIcon())
			}
			l1.SetText(e.Name)
			l2.SetText(sizeLabel(e))
		},
	)
	ui.leftList.OnSelected = func(id widget.ListItemID) {
		ui.leftSel = int(id)
		ui.activePane = "left"
		ui.updateActionState()
	}

	ui.rightList = widget.NewList(
		func() int { return len(ui.rightRows) },
		func() fyne.CanvasObject {
			return newDirListRow(ui, false)
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*dirListRow)
			row.itemID = id
			box := row.box
			if id < 0 || id >= len(ui.rightRows) {
				return
			}
			e := ui.rightRows[id]
			ic := box.Objects[0].(*widget.Icon)
			l1 := box.Objects[1].(*widget.Label)
			l2 := box.Objects[3].(*widget.Label)
			switch {
			case e.Name == "..":
				ic.SetResource(theme.NavigateBackIcon())
			case e.IsDir:
				ic.SetResource(theme.FolderIcon())
			default:
				ic.SetResource(theme.DocumentIcon())
			}
			l1.SetText(e.Name)
			l2.SetText(sizeLabel(e))
		},
	)
	ui.rightList.OnSelected = func(id widget.ListItemID) {
		ui.rightSel = int(id)
		ui.activePane = "right"
		ui.updateActionState()
	}

	// ctxSelect depois das listas: SetSelectedIndex(0) dispara o callback e usa rightList.UnselectAll().
	ui.ctxSelect = widget.NewSelect(ui.containerOpts, func(sel string) {
		idx := -1
		for i, o := range ui.containerOpts {
			if o == sel {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		id := ui.containerIDs[idx]
		ui.hostMode = (id == "")
		if ui.hostMode {
			ui.cfs = nil
		} else {
			ui.cfs = &containerfs.FS{Docker: s.Docker, ID: id}
		}
		ui.rightPath = "/"
		ui.rightSel = -1
		ui.rightList.UnselectAll()
		ui.refreshRight()
		ui.updateBreadcrumb()
	})
	ui.ctxSelect.SetSelectedIndex(0)

	ui.btnOpenLocal = widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() { ui.onLeftActivate() })
	ui.btnOpenRemote = widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() { ui.onRightActivate() })
	btnBackLocal := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goLeftBack() })
	btnUpLocal := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goLeftUp() })
	btnHomeLocal := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goLeftHome() })
	btnReloadLocal := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshLeft() })
	btnBackRemote := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goRightBack() })
	btnUpRemote := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goRightUp() })
	btnHomeRemote := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goRightHome() })
	btnReloadRemote := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshRight() })

	ui.btnUp = widget.NewButtonWithIcon("", theme.UploadIcon(), func() { ui.upload() })
	ui.btnUp.Importance = widget.HighImportance
	ui.btnDown = widget.NewButtonWithIcon("", theme.DownloadIcon(), func() { ui.download() })
	ui.btnDown.Importance = widget.HighImportance
	btnRefresh := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		ui.refreshLeft()
		ui.refreshRight()
	})
	btnDisconnect := widget.NewButtonWithIcon("", theme.LogoutIcon(), func() {
		s.Close()
		goToLogin(w)
	})
	btnDisconnect.Importance = widget.DangerImportance

	toolbar := fynecontainer.NewHBox(
		ui.btnUp,
		ui.btnDown,
		btnRefresh,
		layout.NewSpacer(),
		btnDisconnect,
	)
	top := toolbar

	leftFavs := ui.defaultLocalShortcuts()
	ui.leftQuick = widget.NewSelect(leftFavs, func(sel string) {
		p, ok := ui.resolveLocalShortcut(sel)
		if !ok || p == "" || p == ui.leftPath {
			return
		}
		ui.pushLeftHistory(p)
		ui.leftPath = p
		ui.refreshLeft()
	})
	ui.leftQuick.SetSelected("Diretório inicial")

	rightFavs := []string{"/", "/home", "/opt", "/var", "/tmp"}
	ui.rightQuick = widget.NewSelect(rightFavs, func(sel string) {
		p := strings.TrimSpace(sel)
		if p == "" || p == ui.rightPath {
			return
		}
		ui.pushRightHistory(p)
		ui.rightPath = p
		ui.refreshRight()
	})
	ui.rightQuick.SetSelected("/")

	leftHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			btnBackLocal,
			btnUpLocal,
			btnHomeLocal,
			btnReloadLocal,
			ui.btnOpenLocal,
			layout.NewSpacer(),
			ui.leftQuick,
		),
		ui.leftSearch,
	)
	leftPane := fynecontainer.NewBorder(
		fynecontainer.NewPadded(leftHead),
		nil, nil, nil,
		fynecontainer.NewPadded(fynecontainer.NewScroll(ui.leftList)),
	)

	rightHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			btnBackRemote,
			btnUpRemote,
			btnHomeRemote,
			btnReloadRemote,
			ui.btnOpenRemote,
			layout.NewSpacer(),
			ui.ctxSelect,
		),
		fynecontainer.NewHBox(ui.rightQuick, ui.rightSearch),
	)
	rightPane := fynecontainer.NewBorder(
		fynecontainer.NewPadded(rightHead),
		nil, nil, nil,
		fynecontainer.NewPadded(fynecontainer.NewScroll(ui.rightList)),
	)

	split := fynecontainer.NewHSplit(leftPane, rightPane)
	split.SetOffset(0.48)

	bottom := fynecontainer.NewVBox(
		ui.status,
		ui.progress,
	)

	ui.refreshLeft()
	ui.refreshRight()
	ui.leftSearch.OnChanged = func(_ string) { ui.applyLeftFilter() }
	ui.rightSearch.OnChanged = func(_ string) { ui.applyRightFilter() }
	ui.registerExplorerShortcuts()
	ui.updateBreadcrumb()
	ui.updateActionState()

	return fynecontainer.NewBorder(top, bottom, nil, nil, split)
}

func homeOrRoot() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "."
	}
	return h
}

func sizeLabel(e fsutil.DirEntry) string {
	if e.Name == ".." {
		return ""
	}
	if e.IsDir {
		return ""
	}
	return transfer.FormatBytes(e.Size)
}

func (ui *explorer) updateBreadcrumb() {
	ui.leftPathLbl.SetText(fmt.Sprintf("Pasta local: %s", ui.leftPath))
	ui.leftCrumbs.Objects = ui.makePathButtons(ui.leftPath, true)
	ui.leftCrumbs.Refresh()
	if ui.hostMode {
		ui.breadcrumb.SetText(fmt.Sprintf("Pasta no servidor: %s", ui.rightPath))
		ui.rightCrumbs.Objects = ui.makePathButtons(ui.rightPath, false)
		ui.rightCrumbs.Refresh()
		return
	}
	short := strings.TrimPrefix(ui.cfs.ID, "sha256:")
	if len(short) > 12 {
		short = short[:12]
	}
	ui.breadcrumb.SetText(fmt.Sprintf("Dentro do contêiner (ID %s): %s", short, ui.rightPath))
	ui.rightCrumbs.Objects = ui.makePathButtons(ui.rightPath, false)
	ui.rightCrumbs.Refresh()
}

func (ui *explorer) refreshLeft() {
	rows, err := localfs.List(ui.leftPath)
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	ui.leftAll = rows
	ui.applyLeftFilter()
}

func (ui *explorer) applyLeftFilter() {
	selectedPath := ""
	if ui.leftSel >= 0 && ui.leftSel < len(ui.leftRows) {
		selectedPath = ui.leftRows[ui.leftSel].Path
	}
	term := strings.ToLower(strings.TrimSpace(ui.leftSearch.Text))
	if term == "" {
		ui.leftRows = append([]fsutil.DirEntry(nil), ui.leftAll...)
	} else {
		filtered := make([]fsutil.DirEntry, 0, len(ui.leftAll))
		for _, e := range ui.leftAll {
			if e.Name == ".." || strings.Contains(strings.ToLower(e.Name), term) {
				filtered = append(filtered, e)
			}
		}
		ui.leftRows = filtered
	}
	ui.leftSel = -1
	ui.leftList.UnselectAll()
	if selectedPath != "" {
		for i, e := range ui.leftRows {
			if e.Path == selectedPath {
				ui.leftSel = i
				ui.leftList.Select(i)
				break
			}
		}
	}
	ui.leftList.Refresh()
	ui.leftList.ScrollToTop()
	ui.updateBreadcrumb()
	ui.updateActionState()
}

func (ui *explorer) refreshRight() {
	ui.refreshRightImpl(true)
}

// refreshRightQuiet atualiza a lista direita sem alterar a barra de estado (ex.: após transferência, para não apagar "Concluído:").
func (ui *explorer) refreshRightQuiet() {
	ui.refreshRightImpl(false)
}

func (ui *explorer) refreshRightImpl(showLoading bool) {
	_ = showLoading
	seq := ui.rightRefreshSeq.Add(1)
	hostMode := ui.hostMode
	p := ui.rightPath
	hfs := ui.hfs
	var cfs *containerfs.FS
	if !hostMode {
		cfs = ui.cfs
	}

	go func(seq uint64) {
		if !hostMode && cfs == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		type listResult struct {
			rows []fsutil.DirEntry
			err  error
		}
		resCh := make(chan listResult, 1)
		go func() {
			var rows []fsutil.DirEntry
			var err error
			if hostMode {
				rows, err = hfs.List(ctx, p)
			} else {
				rows, err = cfs.List(ctx, p)
			}
			resCh <- listResult{rows: rows, err: err}
		}()
		var rows []fsutil.DirEntry
		var err error
		select {
		case res := <-resCh:
			rows, err = res.rows, res.err
		case <-time.After(65 * time.Second):
			err = fmt.Errorf("tempo limite ao listar \"%s\"; tente outra pasta ou Atualizar", p)
		}
		fyne.Do(func() {
			if seq != ui.rightRefreshSeq.Load() {
				return
			}
			if err != nil {
				ui.status.SetText(fmt.Sprintf("Erro ao listar: %v", err))
				ui.rightAll = nil
				ui.rightRows = nil
				ui.rightSel = -1
				ui.rightList.UnselectAll()
				ui.rightList.Refresh()
				return
			}
			ui.rightAll = rows
			ui.applyRightFilter()
			if strings.HasPrefix(ui.status.Text, "Carregando pastas") {
				ui.status.SetText("")
			}
		})
	}(seq)
}

func (ui *explorer) applyRightFilter() {
	selectedPath := ""
	if ui.rightSel >= 0 && ui.rightSel < len(ui.rightRows) {
		selectedPath = ui.rightRows[ui.rightSel].Path
	}
	term := strings.ToLower(strings.TrimSpace(ui.rightSearch.Text))
	if term == "" {
		ui.rightRows = append([]fsutil.DirEntry(nil), ui.rightAll...)
	} else {
		filtered := make([]fsutil.DirEntry, 0, len(ui.rightAll))
		for _, e := range ui.rightAll {
			if e.Name == ".." || strings.Contains(strings.ToLower(e.Name), term) {
				filtered = append(filtered, e)
			}
		}
		ui.rightRows = filtered
	}
	ui.rightSel = -1
	ui.rightList.UnselectAll()
	if selectedPath != "" {
		for i, e := range ui.rightRows {
			if e.Path == selectedPath {
				ui.rightSel = i
				ui.rightList.Select(i)
				break
			}
		}
	}
	ui.rightList.Refresh()
	ui.rightList.ScrollToTop()
	ui.updateBreadcrumb()
	ui.updateActionState()
}

func (ui *explorer) goLeftBack() {
	n := len(ui.leftBack)
	if n == 0 {
		return
	}
	prev := ui.leftBack[n-1]
	ui.leftBack = ui.leftBack[:n-1]
	ui.leftPath = prev
	ui.refreshLeft()
}

func (ui *explorer) goRightBack() {
	n := len(ui.rightBack)
	if n == 0 {
		return
	}
	prev := ui.rightBack[n-1]
	ui.rightBack = ui.rightBack[:n-1]
	ui.rightPath = prev
	ui.refreshRight()
}

func (ui *explorer) goLeftUp() {
	parent := filepath.Dir(ui.leftPath)
	if parent == ui.leftPath {
		return
	}
	ui.leftBack = append(ui.leftBack, ui.leftPath)
	ui.leftPath = parent
	ui.refreshLeft()
}

func (ui *explorer) goRightUp() {
	parent := path.Dir(ui.rightPath)
	if parent == "" || parent == "." {
		parent = "/"
	}
	if parent == ui.rightPath {
		return
	}
	ui.rightBack = append(ui.rightBack, ui.rightPath)
	ui.rightPath = parent
	ui.refreshRight()
}

func (ui *explorer) goLeftHome() {
	if ui.leftPath == homeOrRoot() {
		return
	}
	ui.leftBack = append(ui.leftBack, ui.leftPath)
	ui.leftPath = homeOrRoot()
	ui.refreshLeft()
}

func (ui *explorer) goRightHome() {
	if ui.rightPath == "/" {
		return
	}
	ui.rightBack = append(ui.rightBack, ui.rightPath)
	ui.rightPath = "/"
	ui.refreshRight()
}

func (ui *explorer) pushLeftHistory(next string) {
	if next != "" && next != ui.leftPath {
		ui.leftBack = append(ui.leftBack, ui.leftPath)
	}
}

func (ui *explorer) pushRightHistory(next string) {
	if next != "" && next != ui.rightPath {
		ui.rightBack = append(ui.rightBack, ui.rightPath)
	}
}

func (ui *explorer) onLeftActivate() {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		dialog.ShowInformation("ContainerWay", "Selecione uma pasta no painel local para abrir.", ui.win)
		return
	}
	e := ui.leftRows[ui.leftSel]
	if e.IsDir {
		ui.pushLeftHistory(e.Path)
		ui.leftPath = e.Path
		ui.refreshLeft()
		ui.status.SetText("Pasta local aberta: " + e.Path)
	}
}

func (ui *explorer) onLeftDoubleAction() {
	e, ok := ui.selectedLeftEntry()
	if !ok {
		return
	}
	if e.IsDir {
		ui.onLeftActivate()
		return
	}
	if e.Name == ".." {
		ui.goLeftUp()
		return
	}
	if err := openWithDefaultApp(e.Path); err != nil {
		dialog.ShowError(fmt.Errorf("não foi possível abrir o arquivo: %w", err), ui.win)
		return
	}
	ui.status.SetText("Arquivo local aberto: " + e.Name)
}

func (ui *explorer) onRightActivate() {
	if ui.rightSel < 0 || ui.rightSel >= len(ui.rightRows) {
		dialog.ShowInformation("ContainerWay", "Selecione uma pasta no painel do servidor para abrir.", ui.win)
		return
	}
	e := ui.rightRows[ui.rightSel]
	if e.IsDir {
		ui.pushRightHistory(e.Path)
		ui.rightPath = e.Path
		ui.updateBreadcrumb()
		ui.refreshRight()
		ui.status.SetText("Pasta remota aberta: " + e.Path)
	}
}

func (ui *explorer) onRightDoubleAction() {
	e, ok := ui.selectedRightEntry()
	if !ok {
		return
	}
	if e.IsDir {
		ui.onRightActivate()
		return
	}
	if e.Name == ".." {
		ui.goRightUp()
		return
	}
	dialog.ShowConfirm(
		"Editar arquivo remoto",
		fmt.Sprintf("Deseja abrir \"%s\" para edição remota? O arquivo será sincronizado de volta quando você salvar.", e.Name),
		func(open bool) {
			if !open {
				return
			}
			ui.openRemoteForEdit(e)
		},
		ui.win,
	)
}

func (ui *explorer) upload() {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		dialog.ShowInformation("ContainerWay", "Selecione um arquivo ou pasta na lista à esquerda (seu computador).", ui.win)
		return
	}
	src := ui.leftRows[ui.leftSel]
	if src.Name == ".." {
		dialog.ShowInformation("ContainerWay", "Selecione um arquivo ou pasta válidos.", ui.win)
		return
	}
	dstName := filepath.Base(src.Path)

	if src.IsDir {
		if ui.hostMode {
			remoteBase := path.Join(ui.rightPath, dstName)
			ui.tm.Enqueue(transfer.Job{
				Name: fmt.Sprintf("Enviar pasta %s → servidor:%s", src.Path, remoteBase),
				Run: func(ctx context.Context, on transfer.Progress) error {
					if on != nil {
						on(0, -1)
					}
					n, err := tarxfer.SFTPUploadLocalTree(ctx, src.Path, remoteBase, ui.hfs.Client)
					if on != nil {
						on(n, max(n, int64(1)))
					}
					return err
				},
			})
		} else {
			ui.tm.Enqueue(transfer.Job{
				Name: fmt.Sprintf("Enviar pasta %s → contêiner:%s", src.Path, ui.rightPath),
				Run: func(ctx context.Context, on transfer.Progress) error {
					if on != nil {
						on(0, -1)
					}
					err := tarxfer.UploadLocalDirToContainer(ctx, ui.s.Docker, ui.cfs.ID, src.Path, ui.rightPath)
					if on != nil {
						on(1, 1)
					}
					return err
				},
			})
		}
		ui.startDrain()
		return
	}

	if ui.hostMode {
		dst := path.Join(ui.rightPath, dstName)
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Enviar %s → servidor:%s", src.Path, dst),
			Run: func(ctx context.Context, on transfer.Progress) error {
				f, err := os.Open(src.Path)
				if err != nil {
					return err
				}
				defer f.Close()
				st, err := f.Stat()
				if err != nil {
					return err
				}
				total := st.Size()
				var done atomic.Int64
				wf, err := ui.hfs.CreateWriter(dst)
				if err != nil {
					return err
				}
				defer wf.Close()
				pr := &transfer.CountingReader{R: f, N: &done, Total: total}
				if on != nil {
					t := time.NewTicker(120 * time.Millisecond)
					defer t.Stop()
					go func() {
						for {
							select {
							case <-ctx.Done():
								return
							case <-t.C:
								on(done.Load(), total)
							}
						}
					}()
				}
				if _, err := io.Copy(wf, pr); err != nil {
					return err
				}
				if on != nil {
					on(total, total)
				}
				return nil
			},
		})
	} else {
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Enviar %s → contêiner:%s", src.Path, ui.rightPath),
			Run: func(ctx context.Context, on transfer.Progress) error {
				f, err := os.Open(src.Path)
				if err != nil {
					return err
				}
				defer f.Close()
				st, err := f.Stat()
				if err != nil {
					return err
				}
				total := st.Size()
				var done atomic.Int64
				pr := &transfer.CountingReader{R: f, N: &done, Total: total}
				if on != nil {
					t := time.NewTicker(120 * time.Millisecond)
					defer t.Stop()
					go func() {
						for {
							select {
							case <-ctx.Done():
								return
							case <-t.C:
								on(done.Load(), total)
							}
						}
					}()
				}
				if err := ui.cfs.UploadFile(ctx, ui.rightPath, dstName, pr, total); err != nil {
					return err
				}
				if on != nil {
					on(total, total)
				}
				return nil
			},
		})
	}
	ui.startDrain()
}

func (ui *explorer) download() {
	if ui.rightSel < 0 || ui.rightSel >= len(ui.rightRows) {
		dialog.ShowInformation("ContainerWay", "Selecione um arquivo ou pasta na lista à direita (servidor ou contêiner).", ui.win)
		return
	}
	src := ui.rightRows[ui.rightSel]
	if src.Name == ".." {
		dialog.ShowInformation("ContainerWay", "Selecione um arquivo ou pasta válidos.", ui.win)
		return
	}
	dstPath := filepath.Join(ui.leftPath, src.Name)

	if src.IsDir {
		if ui.hostMode {
			ui.tm.Enqueue(transfer.Job{
				Name: fmt.Sprintf("Receber pasta servidor:%s → %s", src.Path, dstPath),
				Run: func(ctx context.Context, on transfer.Progress) error {
					if on != nil {
						on(0, -1)
					}
					n, err := tarxfer.SFTPDownloadTree(ctx, ui.hfs.Client, src.Path, dstPath)
					if on != nil {
						on(n, max(n, int64(1)))
					}
					return err
				},
			})
		} else {
			ui.tm.Enqueue(transfer.Job{
				Name: fmt.Sprintf("Receber pasta contêiner:%s → %s", src.Path, dstPath),
				Run: func(ctx context.Context, on transfer.Progress) error {
					if on != nil {
						on(0, -1)
					}
					n, err := tarxfer.ExtractContainerDirToLocal(ctx, ui.s.Docker, ui.cfs.ID, src.Path, dstPath)
					if on != nil {
						on(n, max(n, int64(1)))
					}
					return err
				},
			})
		}
		ui.startDrain()
		return
	}

	if ui.hostMode {
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Receber servidor:%s → %s", src.Path, dstPath),
			Run: func(ctx context.Context, on transfer.Progress) error {
				rf, err := ui.hfs.OpenReader(src.Path)
				if err != nil {
					return err
				}
				defer rf.Close()
				st, err := rf.Stat()
				if err != nil {
					return err
				}
				total := st.Size()
				out, err := os.Create(dstPath)
				if err != nil {
					return err
				}
				defer out.Close()
				var done atomic.Int64
				pw := &transfer.CountingWriter{W: out, N: &done, Total: total}
				if on != nil {
					t := time.NewTicker(120 * time.Millisecond)
					defer t.Stop()
					go func() {
						for {
							select {
							case <-ctx.Done():
								return
							case <-t.C:
								on(done.Load(), total)
							}
						}
					}()
				}
				if _, err := io.Copy(pw, rf); err != nil {
					return err
				}
				if on != nil {
					on(total, total)
				}
				return nil
			},
		})
	} else {
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Receber contêiner:%s → %s", src.Path, dstPath),
			Run: func(ctx context.Context, on transfer.Progress) error {
				rc, total, err := ui.cfs.OpenFileReader(ctx, src.Path)
				if err != nil {
					return err
				}
				defer rc.Close()
				out, err := os.Create(dstPath)
				if err != nil {
					return err
				}
				defer out.Close()
				var done atomic.Int64
				pr := &transfer.CountingReader{R: rc, N: &done, Total: total}
				if on != nil {
					t := time.NewTicker(120 * time.Millisecond)
					defer t.Stop()
					go func() {
						for {
							select {
							case <-ctx.Done():
								return
							case <-t.C:
								on(done.Load(), total)
							}
						}
					}()
				}
				if _, err := io.Copy(out, pr); err != nil {
					return err
				}
				if on != nil {
					on(total, total)
				}
				return nil
			},
		})
	}
	ui.startDrain()
}

func (ui *explorer) startDrain() {
	ctx := context.Background()
	ui.tm.DrainAsync(ctx, ui.parallelJobs,
		func(j transfer.Job) {
			fyne.Do(func() {
				ui.progress.Show()
				ui.progress.SetValue(0)
				ui.lastJobText.SetText(j.Name)
				ui.status.SetText("Transferindo…")
			})
		},
		func(j transfer.Job, err error) {
			fyne.Do(func() {
				ui.progress.SetValue(1)
				ui.progress.Hide()
				if err != nil {
					ui.status.SetText(fmt.Sprintf("Erro: %v", err))
					dialog.ShowError(err, ui.win)
				} else {
					ui.status.SetText("Concluído: " + j.Name)
					dialog.ShowInformation("Transferência concluída", j.Name, ui.win)
				}
				ui.refreshLeft()
				ui.refreshRightQuiet()
			})
		},
		func(done, total int64) {
			fyne.Do(func() {
				if total > 0 {
					ui.progress.SetValue(float64(done) / float64(total))
				} else if total < 0 {
					ui.progress.SetValue(0.1)
				}
			})
		},
	)
}

func (ui *explorer) selectedLeftEntry() (fsutil.DirEntry, bool) {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		return fsutil.DirEntry{}, false
	}
	return ui.leftRows[ui.leftSel], true
}

func (ui *explorer) selectedRightEntry() (fsutil.DirEntry, bool) {
	if ui.rightSel < 0 || ui.rightSel >= len(ui.rightRows) {
		return fsutil.DirEntry{}, false
	}
	return ui.rightRows[ui.rightSel], true
}

func (ui *explorer) updateActionState() {
	left, hasLeft := ui.selectedLeftEntry()
	right, hasRight := ui.selectedRightEntry()
	if ui.btnUp != nil {
		if hasLeft && left.Name != ".." {
			ui.btnUp.Enable()
		} else {
			ui.btnUp.Disable()
		}
	}
	if ui.btnDown != nil {
		if hasRight && right.Name != ".." {
			ui.btnDown.Enable()
		} else {
			ui.btnDown.Disable()
		}
	}
	if ui.btnOpenLocal != nil {
		if hasLeft && left.IsDir {
			ui.btnOpenLocal.Enable()
		} else {
			ui.btnOpenLocal.Disable()
		}
	}
	if ui.btnOpenRemote != nil {
		if hasRight && right.IsDir {
			ui.btnOpenRemote.Enable()
		} else {
			ui.btnOpenRemote.Disable()
		}
	}
	switch ui.activePane {
	case "left":
		if hasLeft {
			ui.selectionInfo.SetText(fmt.Sprintf("Selecionado (local): %s | Tipo: %s | Sugestão: %s", left.Name, entryTypeLabel(left), leftActionSuggestion(left)))
		} else {
			ui.selectionInfo.SetText("Lado ativo: computador local. Selecione um item para habilitar ações.")
		}
	default:
		if hasRight {
			ui.selectionInfo.SetText(fmt.Sprintf("Selecionado (servidor): %s | Tipo: %s | Sugestão: %s", right.Name, entryTypeLabel(right), rightActionSuggestion(right)))
		} else {
			ui.selectionInfo.SetText("Lado ativo: servidor. Selecione um item para habilitar ações.")
		}
	}
}

func entryTypeLabel(e fsutil.DirEntry) string {
	if e.IsDir {
		return "pasta"
	}
	return "arquivo"
}

func leftActionSuggestion(e fsutil.DirEntry) string {
	if e.Name == ".." {
		return "subir nível"
	}
	if e.IsDir {
		return "abrir pasta ou enviar"
	}
	return "enviar"
}

func rightActionSuggestion(e fsutil.DirEntry) string {
	if e.Name == ".." {
		return "subir nível"
	}
	if e.IsDir {
		return "abrir pasta ou receber"
	}
	return "receber"
}

func (ui *explorer) registerExplorerShortcuts() {
	ui.win.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if _, ok := ui.win.Canvas().Focused().(*widget.Entry); ok {
			return
		}
		switch k.Name {
		case fyne.KeyEnter, fyne.KeyReturn:
			if ui.activePane == "left" {
				ui.onLeftActivate()
			} else {
				ui.onRightActivate()
			}
		case fyne.KeyBackspace:
			if ui.activePane == "left" {
				ui.goLeftUp()
			} else {
				ui.goRightUp()
			}
		case fyne.KeyF5:
			ui.refreshLeft()
			ui.refreshRight()
		}
	})
}

func (ui *explorer) makePathButtons(p string, left bool) []fyne.CanvasObject {
	if left {
		clean := filepath.Clean(p)
		if clean == "" {
			return nil
		}
		sep := string(filepath.Separator)
		parts := strings.Split(clean, sep)
		var out []fyne.CanvasObject
		current := ""
		for i, part := range parts {
			if part == "" && i > 0 {
				continue
			}
			if i == 0 && strings.HasSuffix(part, ":") {
				current = part + sep
			} else if current == "" {
				current = part
			} else {
				current = filepath.Join(current, part)
			}
			target := current
			lbl := part
			if lbl == "" {
				lbl = sep
			}
			btn := widget.NewButton(lbl, func() {
				ui.pushLeftHistory(target)
				ui.leftPath = target
				ui.refreshLeft()
			})
			btn.Importance = widget.LowImportance
			out = append(out, btn)
			if i < len(parts)-1 {
				out = append(out, widget.NewLabel(" / "))
			}
		}
		return out
	}
	clean := path.Clean(p)
	if clean == "." {
		clean = "/"
	}
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	out := []fyne.CanvasObject{}
	rootBtn := widget.NewButton("/", func() {
		ui.pushRightHistory("/")
		ui.rightPath = "/"
		ui.refreshRight()
	})
	rootBtn.Importance = widget.LowImportance
	out = append(out, rootBtn)
	current := "/"
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		current = path.Join(current, part)
		target := current
		out = append(out, widget.NewLabel(" / "))
		btn := widget.NewButton(part, func() {
			ui.pushRightHistory(target)
			ui.rightPath = target
			ui.refreshRight()
		})
		btn.Importance = widget.LowImportance
		out = append(out, btn)
	}
	return out
}

func (ui *explorer) defaultLocalShortcuts() []string {
	return []string{"Diretório inicial", "Desktop", "Documentos", "Downloads"}
}

func (ui *explorer) resolveLocalShortcut(sel string) (string, bool) {
	home := homeOrRoot()
	switch sel {
	case "Diretório inicial":
		return home, true
	case "Desktop":
		return filepath.Join(home, "Desktop"), true
	case "Documentos":
		return filepath.Join(home, "Documents"), true
	case "Downloads":
		return filepath.Join(home, "Downloads"), true
	default:
		return "", false
	}
}

func (ui *explorer) showRowContextMenu(left bool, id widget.ListItemID, pos fyne.Position) {
	if left {
		if id < 0 || int(id) >= len(ui.leftRows) {
			return
		}
		ui.leftList.Select(id)
		ui.leftSel = int(id)
		ui.activePane = "left"
		ui.updateActionState()
		e := ui.leftRows[id]
		items := []*fyne.MenuItem{
			fyne.NewMenuItem("Abrir pasta", func() { ui.onLeftActivate() }),
			fyne.NewMenuItem("Enviar para o servidor", func() { ui.upload() }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Atualizar lista local", func() { ui.refreshLeft() }),
		}
		if !e.IsDir {
			items[0].Disabled = true
		}
		if e.Name == ".." {
			items[1].Disabled = true
		}
		widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), ui.win.Canvas(), pos)
		return
	}
	if id < 0 || int(id) >= len(ui.rightRows) {
		return
	}
	ui.rightList.Select(id)
	ui.rightSel = int(id)
	ui.activePane = "right"
	ui.updateActionState()
	e := ui.rightRows[id]
	items := []*fyne.MenuItem{
		fyne.NewMenuItem("Abrir pasta", func() { ui.onRightActivate() }),
		fyne.NewMenuItem("Receber no computador local", func() { ui.download() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Atualizar lista remota", func() { ui.refreshRight() }),
	}
	if !e.IsDir {
		items[0].Disabled = true
	}
	if e.Name == ".." {
		items[1].Disabled = true
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), ui.win.Canvas(), pos)
}

func (ui *explorer) openRemoteForEdit(e fsutil.DirEntry) {
	go func() {
		fyne.Do(func() {
			ui.status.SetText("Abrindo arquivo remoto para edição…")
		})
		ext := filepath.Ext(e.Name)
		tmp, err := os.CreateTemp("", "containerway-open-*"+ext)
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("não foi possível criar arquivo temporário: %w", err), ui.win)
			})
			return
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		editOnHost := ui.hostMode
		editContainerID := ""
		if !editOnHost && ui.cfs != nil {
			editContainerID = ui.cfs.ID
		}
		if editOnHost {
			rf, err := ui.hfs.OpenReader(e.Path)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("não foi possível ler arquivo remoto: %w", err), ui.win)
				})
				return
			}
			defer rf.Close()
			out, err := os.Create(tmpPath)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("não foi possível gravar arquivo temporário: %w", err), ui.win)
				})
				return
			}
			if _, err := io.Copy(out, rf); err != nil {
				_ = out.Close()
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("falha ao copiar arquivo remoto: %w", err), ui.win)
				})
				return
			}
			_ = out.Close()
		} else {
			cfs := &containerfs.FS{Docker: ui.s.Docker, ID: editContainerID}
			rc, _, err := cfs.OpenFileReader(ctx, e.Path)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("não foi possível ler arquivo no contêiner: %w", err), ui.win)
				})
				return
			}
			defer rc.Close()
			out, err := os.Create(tmpPath)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("não foi possível gravar arquivo temporário: %w", err), ui.win)
				})
				return
			}
			if _, err := io.Copy(out, rc); err != nil {
				_ = out.Close()
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("falha ao copiar arquivo do contêiner: %w", err), ui.win)
				})
				return
			}
			_ = out.Close()
		}
		if err := openWithDefaultApp(tmpPath); err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("arquivo baixado, mas não foi possível abrir: %w", err), ui.win)
			})
			return
		}
		st, err := os.Stat(tmpPath)
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("arquivo aberto, mas não foi possível iniciar monitoramento: %w", err), ui.win)
			})
			return
		}
		session := &remoteEditSession{
			tempPath:    tmpPath,
			remotePath:  e.Path,
			hostMode:    editOnHost,
			containerID: editContainerID,
			lastMod:     st.ModTime(),
			lastSize:    st.Size(),
		}
		ui.trackRemoteEditSession(session)
		ui.startRemoteEditWatcher(session)
		fyne.Do(func() {
			ui.status.SetText("Arquivo remoto aberto para edição: " + e.Name)
		})
	}()
}

func (ui *explorer) trackRemoteEditSession(s *remoteEditSession) {
	ui.remoteEditMu.Lock()
	defer ui.remoteEditMu.Unlock()
	if old, ok := ui.remoteEditSessions[s.tempPath]; ok {
		old.stopped.Store(true)
	}
	ui.remoteEditSessions[s.tempPath] = s
}

func (ui *explorer) startRemoteEditWatcher(s *remoteEditSession) {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		idleChecks := 0
		for range ticker.C {
			if s.stopped.Load() {
				return
			}
			st, err := os.Stat(s.tempPath)
			if err != nil {
				s.stopped.Store(true)
				return
			}
			changed := st.ModTime() != s.lastMod || st.Size() != s.lastSize
			if changed {
				if err := ui.syncEditedFileBack(s); err != nil {
					fyne.Do(func() {
						ui.status.SetText("Erro ao sincronizar edição remota")
						dialog.ShowError(fmt.Errorf("não foi possível sincronizar o arquivo remoto: %w", err), ui.win)
					})
					continue
				}
				s.lastMod = st.ModTime()
				s.lastSize = st.Size()
				idleChecks = 0
				fyne.Do(func() {
					ui.status.SetText("Alterações salvas no servidor automaticamente.")
				})
				continue
			}
			idleChecks++
			if idleChecks > 180 { // ~6 minutos sem mudanças; mantém baixo custo de monitoramento
				s.stopped.Store(true)
				ui.remoteEditMu.Lock()
				delete(ui.remoteEditSessions, s.tempPath)
				ui.remoteEditMu.Unlock()
				return
			}
		}
	}()
}

func (ui *explorer) syncEditedFileBack(s *remoteEditSession) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	in, err := os.Open(s.tempPath)
	if err != nil {
		return err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if s.hostMode {
		w, err := ui.hfs.CreateWriter(s.remotePath)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = io.Copy(w, in)
		return err
	}
	cfs := &containerfs.FS{Docker: ui.s.Docker, ID: s.containerID}
	return cfs.UploadFile(ctx, path.Dir(s.remotePath), path.Base(s.remotePath), in, st.Size())
}

func openWithDefaultApp(filePath string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", "", filePath).Start()
	case "darwin":
		return exec.Command("open", filePath).Start()
	default:
		return exec.Command("xdg-open", filePath).Start()
	}
}
