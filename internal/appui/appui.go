package appui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
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
	ctxSelect   *widget.Select
	status      *widget.Label
	progress    *widget.ProgressBar
	lastJobText *widget.Label
	leftSearch  *widget.Entry
	rightSearch *widget.Entry
	leftBack    []string
	rightBack   []string

	tm            *transfer.Manager
	parallelJobs int

	// Evita aplicar listagens antigas se o usuário mudar de pasta/contexto a meio.
	rightRefreshSeq atomic.Uint64
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
	ui.leftList.OnSelected = func(id widget.ListItemID) { ui.leftSel = int(id) }

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
	ui.rightList.OnSelected = func(id widget.ListItemID) { ui.rightSel = int(id) }

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

	btnOpenLocal := widget.NewButtonWithIcon("Abrir pasta", theme.FolderOpenIcon(), func() { ui.onLeftActivate() })
	btnOpenRemote := widget.NewButtonWithIcon("Abrir pasta", theme.FolderOpenIcon(), func() { ui.onRightActivate() })
	btnBackLocal := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goLeftBack() })
	btnUpLocal := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goLeftUp() })
	btnHomeLocal := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goLeftHome() })
	btnReloadLocal := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshLeft() })
	btnBackRemote := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goRightBack() })
	btnUpRemote := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goRightUp() })
	btnHomeRemote := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goRightHome() })
	btnReloadRemote := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshRight() })

	btnUp := widget.NewButtonWithIcon("Enviar", theme.UploadIcon(), func() { ui.upload() })
	btnUp.Importance = widget.HighImportance
	btnDown := widget.NewButtonWithIcon("Receber", theme.DownloadIcon(), func() { ui.download() })
	btnDown.Importance = widget.HighImportance
	btnRefresh := widget.NewButtonWithIcon("Atualizar", theme.ViewRefreshIcon(), func() {
		ui.refreshLeft()
		ui.refreshRight()
	})
	btnDisconnect := widget.NewButtonWithIcon("Sair", theme.LogoutIcon(), func() {
		s.Close()
		goToLogin(w)
	})
	btnDisconnect.Importance = widget.DangerImportance

	toolbar := fynecontainer.NewHBox(
		btnUp,
		btnDown,
		btnRefresh,
		layout.NewSpacer(),
		btnDisconnect,
	)
	top := fynecontainer.NewVBox(
		fynecontainer.NewPadded(toolbar),
		widget.NewSeparator(),
	)

	leftHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			widget.NewIcon(theme.HomeIcon()),
			widget.NewLabelWithStyle("Computador local", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
			btnBackLocal,
			btnUpLocal,
			btnHomeLocal,
			btnReloadLocal,
			btnOpenLocal,
		),
		ui.leftPathLbl,
		ui.leftSearch,
	)
	leftPane := fynecontainer.NewBorder(
		fynecontainer.NewPadded(leftHead),
		nil, nil, nil,
		fynecontainer.NewPadded(fynecontainer.NewScroll(ui.leftList)),
	)

	ctxHelp := widget.NewLabel("Escolha pastas do Linux no servidor ou um contêiner ligado. Na lista só entram contêineres em execução. Duplo clique numa pasta para entrar (ou use «Abrir pasta»).")
	ctxHelp.Wrapping = fyne.TextWrapWord
	ctxHelp.TextStyle = fyne.TextStyle{Italic: true}

	rightHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			widget.NewIcon(theme.StorageIcon()),
			widget.NewLabelWithStyle("Lado do servidor", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
			btnBackRemote,
			btnUpRemote,
			btnHomeRemote,
			btnReloadRemote,
			btnOpenRemote,
		),
		fynecontainer.NewPadded(fynecontainer.NewVBox(ctxHelp, ui.ctxSelect)),
		fynecontainer.NewPadded(ui.breadcrumb),
		ui.rightSearch,
	)
	rightPane := fynecontainer.NewBorder(
		fynecontainer.NewPadded(rightHead),
		nil, nil, nil,
		fynecontainer.NewPadded(fynecontainer.NewScroll(ui.rightList)),
	)

	split := fynecontainer.NewHSplit(leftPane, rightPane)
	split.SetOffset(0.48)

	bottom := fynecontainer.NewVBox(
		fynecontainer.NewPadded(ui.status),
		fynecontainer.NewPadded(ui.progress),
		fynecontainer.NewPadded(ui.lastJobText),
	)

	ui.refreshLeft()
	ui.refreshRight()
	ui.leftSearch.OnChanged = func(_ string) { ui.applyLeftFilter() }
	ui.rightSearch.OnChanged = func(_ string) { ui.applyRightFilter() }
	ui.updateBreadcrumb()

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
	if ui.hostMode {
		ui.breadcrumb.SetText(fmt.Sprintf("Pasta no servidor: %s", ui.rightPath))
		return
	}
	short := strings.TrimPrefix(ui.cfs.ID, "sha256:")
	if len(short) > 12 {
		short = short[:12]
	}
	ui.breadcrumb.SetText(fmt.Sprintf("Dentro do contêiner (ID %s): %s", short, ui.rightPath))
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
	ui.leftList.Refresh()
	ui.leftList.ScrollToTop()
	ui.updateBreadcrumb()
}

func (ui *explorer) refreshRight() {
	ui.refreshRightImpl(true)
}

// refreshRightQuiet atualiza a lista direita sem alterar a barra de estado (ex.: após transferência, para não apagar "Concluído:").
func (ui *explorer) refreshRightQuiet() {
	ui.refreshRightImpl(false)
}

func (ui *explorer) refreshRightImpl(showLoading bool) {
	seq := ui.rightRefreshSeq.Add(1)
	hostMode := ui.hostMode
	p := ui.rightPath
	hfs := ui.hfs
	var cfs *containerfs.FS
	if !hostMode {
		cfs = ui.cfs
	}

	if showLoading {
		ui.status.SetText("Carregando pastas…")
	}

	go func(seq uint64) {
		if !hostMode && cfs == nil {
			if showLoading {
				fyne.Do(func() {
					if seq != ui.rightRefreshSeq.Load() {
						return
					}
					ui.status.SetText("")
				})
			}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var rows []fsutil.DirEntry
		var err error
		if hostMode {
			rows, err = hfs.List(ctx, p)
		} else {
			rows, err = cfs.List(ctx, p)
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
			if showLoading {
				ui.status.SetText("")
			}
		})
	}(seq)
}

func (ui *explorer) applyRightFilter() {
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
	ui.rightList.Refresh()
	ui.rightList.ScrollToTop()
	ui.updateBreadcrumb()
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
		return
	}
	e := ui.leftRows[ui.leftSel]
	if e.IsDir {
		ui.pushLeftHistory(e.Path)
		ui.leftPath = e.Path
		ui.refreshLeft()
	}
}

func (ui *explorer) onRightActivate() {
	if ui.rightSel < 0 || ui.rightSel >= len(ui.rightRows) {
		return
	}
	e := ui.rightRows[ui.rightSel]
	if e.IsDir {
		ui.pushRightHistory(e.Path)
		ui.rightPath = e.Path
		ui.updateBreadcrumb()
		ui.refreshRight()
	}
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
