package appui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
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

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
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

type transientSecret struct {
	Password string
	KeyPass  string
}

var (
	loginSecretMu       sync.Mutex
	loginSessionSecrets = map[string]transientSecret{}
)

const (
	themePreferenceKey = "ui.theme.mode"
	themeModeSystem    = "system"
	themeModeLight     = "light"
	themeModeDark      = "dark"
)

// Run inicia a aplicação Fyne.
func Run() {
	a := app.NewWithID("io.containerway.app")
	applyThemeMode(a, loadThemeMode(a))
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
	w.Resize(fyne.NewSize(460, 440))
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
	keyPath.SetPlaceHolder("caminho da chave .pem / id_rsa")
	keyPass := widget.NewPasswordEntry()
	keyPass.SetPlaceHolder("senha da chave (se houver)")
	knownHosts := widget.NewEntry()
	knownHosts.SetPlaceHolder("known_hosts: caminho1|caminho2")
	themeSelect := widget.NewSelect([]string{"Padrão do sistema", "Claro", "Escuro"}, nil)
	themeSelect.SetSelected(themeLabelForMode(loadThemeMode(fyne.CurrentApp())))
	themeSelect.OnChanged = func(selected string) {
		mode := themeModeFromLabel(selected)
		app := fyne.CurrentApp()
		app.Preferences().SetString(themePreferenceKey, mode)
		applyThemeMode(app, mode)
	}
	insecureHost := widget.NewCheck("Ignorar chave de host SSH (inseguro)", nil)
	insecureHost.SetChecked(true)
	parallelJobsEntry := widget.NewEntry()
	parallelJobsEntry.SetText("3")
	parallelJobsEntry.SetPlaceHolder("transferências em paralelo (1–16)")
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	saveSecrets := widget.NewCheck("Salvar senha/chave nesta conexão (uso local)", nil)
	rememberSession := widget.NewCheck("Lembrar senha/chave só nesta sessão", nil)
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
		rememberSession.SetChecked(false)
		if sec, ok := getTransientSecret(profileSecretKey(c.Name, c.Host, c.User)); ok {
			pass.SetText(sec.Password)
			keyPass.SetText(sec.KeyPass)
			rememberSession.SetChecked(true)
		}
	}

	clearProfileInputs := func() {
		connName.SetText("")
		host.SetText("")
		user.SetText("")
		pass.SetText("")
		keyPath.SetText("")
		keyPass.SetText("")
		knownHosts.SetText("")
		insecureHost.SetChecked(true)
		parallelJobsEntry.SetText("3")
		saveSecrets.SetChecked(false)
		rememberSession.SetChecked(false)
		status.SetText("")
	}

	profileSelect.OnChanged = func(sel string) {
		if sel == "Nova conexão…" {
			clearProfileInputs()
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

	validationHint := widget.NewLabel("")
	validationHint.Wrapping = fyne.TextWrapWord

	var connect *widget.Button
	var testConn *widget.Button
	updateLoginValidation := func() {
		hostVal := strings.TrimSpace(host.Text)
		userVal := strings.TrimSpace(user.Text)
		keyVal := strings.TrimSpace(keyPath.Text)
		parVal := strings.TrimSpace(parallelJobsEntry.Text)
		msg := ""
		canConnect := true
		switch {
		case hostVal == "":
			msg = "Informe o host para conectar."
			canConnect = false
		case userVal == "":
			msg = "Informe o usuário SSH."
			canConnect = false
		case strings.TrimSpace(pass.Text) == "" && keyVal == "":
			msg = "Informe senha ou chave PEM/PPK."
			canConnect = false
		default:
			v, err := strconv.Atoi(parVal)
			if err != nil || v < 1 || v > 16 {
				msg = "Paralelismo deve ser entre 1 e 16."
				canConnect = false
			} else if keyVal != "" {
				if _, err := os.Stat(keyVal); err != nil {
					msg = "Arquivo de chave não encontrado no caminho informado."
					canConnect = false
				}
			}
		}
		validationHint.SetText(msg)
		if connect != nil {
			if canConnect {
				connect.Enable()
			} else {
				connect.Disable()
			}
		}
		if testConn != nil {
			if canConnect {
				testConn.Enable()
			} else {
				testConn.Disable()
			}
		}
	}

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

	saveBtnWrap := fynecontainer.NewGridWrap(
		fyne.NewSize(82, saveProfile.MinSize().Height),
		saveProfile,
	)
	deleteBtnWrap := fynecontainer.NewGridWrap(
		fyne.NewSize(90, deleteProfile.MinSize().Height),
		deleteProfile,
	)
	savedConnRow := fynecontainer.NewBorder(
		nil, nil, nil,
		fynecontainer.NewHBox(saveBtnWrap, deleteBtnWrap),
		profileSelect,
	)

	tabs := fynecontainer.NewAppTabs(
		fynecontainer.NewTabItem("Conexões SSH/SFTP", fynecontainer.NewVBox(
			savedConnRow,
			connName,
			saveSecrets,
			widget.NewSeparator(),
			formConn,
		)),
		fynecontainer.NewTabItem("Chave e segurança", formAdv),
	)
	tabs.SetTabLocation(fynecontainer.TabLocationTop)
	themeRow := fynecontainer.NewBorder(nil, nil, widget.NewLabel("Tema"), nil, themeSelect)

	connect = widget.NewButtonWithIcon("Conectar", theme.LoginIcon(), func() {
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
				w.SetContent(buildExplorer(w, sess, pJobs, creds))
				setExplorerWindow(w)
			})
			if rememberSession.Checked {
				setTransientSecret(profileSecretKey(connName.Text, host.Text, user.Text), transientSecret{
					Password: pass.Text,
					KeyPass:  keyPass.Text,
				})
			} else {
				deleteTransientSecret(profileSecretKey(connName.Text, host.Text, user.Text))
			}
		}()
	})
	connect.Importance = widget.HighImportance
	testConn = widget.NewButtonWithIcon("Testar conexão", theme.ConfirmIcon(), func() {
		status.SetText("Testando conexão…")
		creds := session.Credentials{
			Host:            host.Text,
			User:            user.Text,
			Password:        pass.Text,
			KeyPath:         strings.TrimSpace(keyPath.Text),
			KeyPass:         keyPass.Text,
			KnownHostsFiles: splitKnownHostsFiles(knownHosts.Text),
			InsecureHostKey: insecureHost.Checked,
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			sess, err := session.Connect(ctx, creds)
			if err != nil {
				msg := formatConnectionTestStatus(err)
				fyne.Do(func() {
					status.SetText(msg)
					dialog.ShowError(err, w)
				})
				return
			}
			sess.Close()
			fyne.Do(func() {
				status.SetText("Teste de conexão:\nSSH: OK\nSFTP: OK\nDocker: OK")
			})
		}()
	})

	cardInner := fynecontainer.NewVBox(
		themeRow,
		tabs,
		rememberSession,
		validationHint,
		status,
		widget.NewSeparator(),
		fynecontainer.NewHBox(testConn, connect),
	)

	host.OnChanged = func(string) { updateLoginValidation() }
	user.OnChanged = func(string) { updateLoginValidation() }
	pass.OnChanged = func(string) { updateLoginValidation() }
	keyPath.OnChanged = func(string) { updateLoginValidation() }
	parallelJobsEntry.OnChanged = func(string) { updateLoginValidation() }
	updateLoginValidation()
	card := widget.NewCard(
		"ContainerWay",
		"SSH · SFTP · Docker remoto sem expor a API em TCP",
		cardInner,
	)

	return fynecontainer.NewCenter(card)
}

func loadThemeMode(a fyne.App) string {
	mode := strings.TrimSpace(a.Preferences().StringWithFallback(themePreferenceKey, themeModeSystem))
	switch mode {
	case themeModeSystem, themeModeLight, themeModeDark:
		return mode
	default:
		return themeModeSystem
	}
}

func applyThemeMode(a fyne.App, mode string) {
	switch mode {
	case themeModeLight:
		a.Settings().SetTheme(theme.LightTheme())
	case themeModeDark:
		a.Settings().SetTheme(newModernTheme())
	default:
		a.Settings().SetTheme(theme.DefaultTheme())
	}
}

func themeLabelForMode(mode string) string {
	switch mode {
	case themeModeLight:
		return "Claro"
	case themeModeDark:
		return "Escuro"
	default:
		return "Padrão do sistema"
	}
}

func themeModeFromLabel(label string) string {
	switch strings.TrimSpace(label) {
	case "Claro":
		return themeModeLight
	case "Escuro":
		return themeModeDark
	default:
		return themeModeSystem
	}
}

type explorer struct {
	win fyne.Window
	s   *session.Session
	hfs *hostfs.FS
	cfs *containerfs.FS
	connCreds session.Credentials

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
	leftFooterInfo  *widget.Label
	rightFooterInfo *widget.Label
	leftSearch  *widget.Entry
	rightSearch *widget.Entry
	leftTypeFilter  *widget.Select
	rightTypeFilter *widget.Select
	leftBack    []string
	rightBack   []string

	tm            *transfer.Manager
	parallelJobs int
	activePane    string
	btnOpenLocal  *widget.Button
	btnOpenRemote *widget.Button
	btnUp         *widget.Button
	btnDown       *widget.Button
	btnLeftSend   *widget.Button
	btnRightRecv  *widget.Button
	lblSudoState *widget.Label
	btnDisableSudo *widget.Button

	// Evita aplicar listagens antigas se o usuário mudar de pasta/contexto a meio.
	rightRefreshSeq atomic.Uint64

	remoteEditMu       sync.Mutex
	remoteEditSessions map[string]*remoteEditSession
	rootPromptOpen     atomic.Bool
	sudoEnabled        bool
	sudoUser           string
	sudoPass           string
	sudoValidatedAt    time.Time
	sudoTTL            time.Duration
	dialogShortcutActive atomic.Bool
	dialogConfirmAction  func()
	dialogCancelAction   func()
	dragActive           bool
	dragFromLeft         bool
	dragItemID           widget.ListItemID
	dragAccumX           float32
	copiedEntry          *copiedItem
	batchMu              sync.Mutex
	batchRunning         bool
	batchLabel           string
	batchTotal           int
	batchDone            int
	batchFailures        []string
}

type copiedItem struct {
	entry       fsutil.DirEntry
	fromLeft    bool
	hostMode    bool
	containerID string
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

func formatConnectionTestStatus(err error) string {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.HasPrefix(msg, "conexão tcp:"),
		strings.HasPrefix(msg, "ssh:"),
		strings.Contains(msg, "known_hosts"),
		strings.Contains(msg, "host key"):
		return "Teste de conexão:\nSSH: falhou\nSFTP: não testado\nDocker: não testado"
	case strings.HasPrefix(msg, "sftp:"):
		return "Teste de conexão:\nSSH: OK\nSFTP: falhou\nDocker: não testado"
	case strings.HasPrefix(msg, "docker:"),
		strings.HasPrefix(msg, "docker ("),
		strings.Contains(msg, "/var/run/docker.sock"),
		strings.Contains(msg, "permission denied"):
		return "Teste de conexão:\nSSH: OK\nSFTP: OK\nDocker: sem permissão/indisponível"
	default:
		return "Teste de conexão:\nSSH/SFTP/Docker: falha não classificada"
	}
}

func profileSecretKey(name, host, user string) string {
	n := strings.TrimSpace(name)
	if n != "" {
		return "name:" + strings.ToLower(n)
	}
	return "hostuser:" + strings.ToLower(strings.TrimSpace(host)) + "|" + strings.ToLower(strings.TrimSpace(user))
}

func getTransientSecret(key string) (transientSecret, bool) {
	loginSecretMu.Lock()
	defer loginSecretMu.Unlock()
	sec, ok := loginSessionSecrets[key]
	return sec, ok
}

func setTransientSecret(key string, sec transientSecret) {
	loginSecretMu.Lock()
	defer loginSecretMu.Unlock()
	loginSessionSecrets[key] = sec
}

func deleteTransientSecret(key string) {
	loginSecretMu.Lock()
	defer loginSecretMu.Unlock()
	delete(loginSessionSecrets, key)
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

func buildExplorer(w fyne.Window, s *session.Session, parallelJobs int, creds session.Credentials) fyne.CanvasObject {
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
		connCreds:     creds,
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
		sudoTTL:        10 * time.Minute,
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
	ui.leftSearch.SetPlaceHolder("Pesquisar no computador local (nome, ext:log, tipo:pasta)")
	ui.leftTypeFilter = widget.NewSelect([]string{"Tudo", "Pastas", "Arquivos"}, func(_ string) {
		ui.applyLeftFilter()
	})
	ui.leftTypeFilter.SetSelected("Tudo")
	ui.rightSearch = widget.NewEntry()
	ui.rightSearch.SetPlaceHolder("Pesquisar no lado do servidor (nome, ext:log, tipo:pasta)")
	ui.rightTypeFilter = widget.NewSelect([]string{"Tudo", "Pastas", "Arquivos"}, func(_ string) {
		ui.applyRightFilter()
	})
	ui.rightTypeFilter.SetSelected("Tudo")
	ui.leftFooterInfo = widget.NewLabel("Local: selecione um item.")
	ui.leftFooterInfo.Wrapping = fyne.TextWrapWord
	ui.rightFooterInfo = widget.NewLabel("Servidor: selecione um item.")
	ui.rightFooterInfo.Wrapping = fyne.TextWrapWord

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
		ui.resetRightSearch()
		ui.rightSel = -1
		ui.rightList.UnselectAll()
		ui.refreshRight()
		ui.updateBreadcrumb()
	})
	ui.ctxSelect.SetSelectedIndex(0)

	ui.btnOpenLocal = widget.NewButtonWithIcon("Abrir", theme.FolderOpenIcon(), func() { ui.onLeftActivate() })
	ui.btnOpenRemote = widget.NewButtonWithIcon("Abrir", theme.FolderOpenIcon(), func() { ui.onRightActivate() })
	ui.btnLeftSend = widget.NewButtonWithIcon("Enviar", theme.UploadIcon(), func() { ui.upload() })
	ui.btnRightRecv = widget.NewButtonWithIcon("Receber", theme.DownloadIcon(), func() { ui.download() })
	btnLeftSendBatch := widget.NewButtonWithIcon("Enviar visíveis", theme.ContentAddIcon(), func() { ui.uploadVisibleBatch() })
	btnRightRecvBatch := widget.NewButtonWithIcon("Receber visíveis", theme.ContentAddIcon(), func() { ui.downloadVisibleBatch() })
	ui.btnOpenLocal.Importance = widget.MediumImportance
	ui.btnOpenRemote.Importance = widget.MediumImportance
	ui.btnLeftSend.Importance = widget.HighImportance
	ui.btnRightRecv.Importance = widget.HighImportance
	btnLeftSendBatch.Importance = widget.MediumImportance
	btnRightRecvBatch.Importance = widget.MediumImportance
	btnBackLocal := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goLeftBack() })
	btnUpLocal := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goLeftUp() })
	btnHomeLocal := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goLeftHome() })
	btnReloadLocal := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshLeft() })
	btnBackRemote := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goRightBack() })
	btnUpRemote := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goRightUp() })
	btnHomeRemote := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goRightHome() })
	btnReloadRemote := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshRight() })

	ui.btnUp = widget.NewButtonWithIcon("Enviar", theme.UploadIcon(), func() { ui.upload() })
	ui.btnUp.Importance = widget.HighImportance
	ui.btnDown = widget.NewButtonWithIcon("Receber", theme.DownloadIcon(), func() { ui.download() })
	ui.btnDown.Importance = widget.HighImportance
	btnDisconnect := widget.NewButtonWithIcon("Sair", theme.LogoutIcon(), func() {
		s.Close()
		goToLogin(w)
	})
	btnDisconnect.Importance = widget.DangerImportance
	ui.lblSudoState = widget.NewLabel("Sudo: inativo")
	ui.btnDisableSudo = widget.NewButtonWithIcon("Desativar sudo", theme.CancelIcon(), func() { ui.disableSudoMode() })

	toolbar := fynecontainer.NewHBox(
		ui.btnUp,
		ui.btnDown,
		layout.NewSpacer(),
		ui.lblSudoState,
		ui.btnDisableSudo,
		layout.NewSpacer(),
		btnDisconnect,
	)
	top := fynecontainer.NewVBox(
		fynecontainer.NewPadded(toolbar),
		widget.NewSeparator(),
	)

	leftFavs := ui.defaultLocalShortcuts()
	ui.leftQuick = widget.NewSelect(leftFavs, func(sel string) {
		p, ok := ui.resolveLocalShortcut(sel)
		if !ok || p == "" || p == ui.leftPath {
			return
		}
		ui.pushLeftHistory(p)
		ui.leftPath = p
		ui.resetLeftSearch()
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
		ui.resetRightSearch()
		ui.refreshRight()
	})
	ui.rightQuick.SetSelected("/")

	const (
		quickSelectWidth = float32(170)
		ctxSelectWidth   = float32(185)
		typeSelectWidth  = float32(130)
		navBtnWidth      = float32(36)
	)
	leftQuickWrap := fynecontainer.NewGridWrap(fyne.NewSize(quickSelectWidth, ui.leftQuick.MinSize().Height), ui.leftQuick)
	rightQuickWrap := fynecontainer.NewGridWrap(fyne.NewSize(quickSelectWidth, ui.rightQuick.MinSize().Height), ui.rightQuick)
	ctxSelectWrap := fynecontainer.NewGridWrap(fyne.NewSize(ctxSelectWidth, ui.ctxSelect.MinSize().Height), ui.ctxSelect)
	leftTypeFilterWrap := fynecontainer.NewGridWrap(fyne.NewSize(typeSelectWidth, ui.leftTypeFilter.MinSize().Height), ui.leftTypeFilter)
	rightTypeFilterWrap := fynecontainer.NewGridWrap(fyne.NewSize(typeSelectWidth, ui.rightTypeFilter.MinSize().Height), ui.rightTypeFilter)
	leftBackWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnBackLocal.MinSize().Height), btnBackLocal)
	leftUpWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnUpLocal.MinSize().Height), btnUpLocal)
	leftHomeWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnHomeLocal.MinSize().Height), btnHomeLocal)
	leftReloadWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnReloadLocal.MinSize().Height), btnReloadLocal)
	rightBackWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnBackRemote.MinSize().Height), btnBackRemote)
	rightUpWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnUpRemote.MinSize().Height), btnUpRemote)
	rightHomeWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnHomeRemote.MinSize().Height), btnHomeRemote)
	rightReloadWrap := fynecontainer.NewGridWrap(fyne.NewSize(navBtnWidth, btnReloadRemote.MinSize().Height), btnReloadRemote)

	leftHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			widget.NewIcon(theme.HomeIcon()),
			widget.NewLabelWithStyle("Computador local", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
		),
		fynecontainer.NewHBox(
			leftBackWrap,
			leftUpWrap,
			leftHomeWrap,
			leftReloadWrap,
			layout.NewSpacer(),
			leftQuickWrap,
		),
		fynecontainer.NewBorder(nil, nil, nil, leftTypeFilterWrap, ui.leftSearch),
		fynecontainer.NewHBox(
			ui.btnOpenLocal,
			ui.btnLeftSend,
			btnLeftSendBatch,
		),
	)
	leftPaneBase := fynecontainer.NewBorder(
		fynecontainer.NewPadded(leftHead),
		nil, nil, nil,
		fynecontainer.NewPadded(fynecontainer.NewScroll(ui.leftList)),
	)
	leftPane := panelCard(leftPaneBase)

	rightHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			widget.NewIcon(theme.StorageIcon()),
			widget.NewLabelWithStyle("Lado do servidor", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
		),
		fynecontainer.NewHBox(
			rightBackWrap,
			rightUpWrap,
			rightHomeWrap,
			rightReloadWrap,
			layout.NewSpacer(),
			rightQuickWrap,
			ctxSelectWrap,
		),
		fynecontainer.NewBorder(nil, nil, nil, rightTypeFilterWrap, ui.rightSearch),
		fynecontainer.NewHBox(
			ui.btnOpenRemote,
			ui.btnRightRecv,
			btnRightRecvBatch,
		),
	)
	rightPaneBase := fynecontainer.NewBorder(
		fynecontainer.NewPadded(rightHead),
		nil, nil, nil,
		fynecontainer.NewPadded(fynecontainer.NewScroll(ui.rightList)),
	)
	rightPane := panelCard(rightPaneBase)

	split := fynecontainer.NewHSplit(leftPane, rightPane)
	split.SetOffset(0.48)

	bottomInfoSplit := fynecontainer.NewHSplit(
		panelCard(fynecontainer.NewPadded(ui.leftFooterInfo)),
		panelCard(fynecontainer.NewPadded(ui.rightFooterInfo)),
	)
	bottomInfoSplit.SetOffset(0.48)
	bottom := fynecontainer.NewVBox(
		bottomInfoSplit,
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
	ui.updateSudoUIState()

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

func panelCard(content fyne.CanvasObject) fyne.CanvasObject {
	border := canvas.NewRectangle(theme.DisabledColor())
	border.FillColor = color.Transparent
	border.StrokeColor = theme.DisabledColor()
	border.StrokeWidth = 1.5
	return fynecontainer.NewMax(border, content)
}

func (ui *explorer) startRowDrag(left bool, id widget.ListItemID) {
	if left {
		if id < 0 || int(id) >= len(ui.leftRows) {
			return
		}
		ui.leftList.Select(id)
		ui.leftSel = int(id)
		ui.activePane = "left"
	} else {
		if id < 0 || int(id) >= len(ui.rightRows) {
			return
		}
		ui.rightList.Select(id)
		ui.rightSel = int(id)
		ui.activePane = "right"
	}
	ui.dragActive = true
	ui.dragFromLeft = left
	ui.dragItemID = id
	ui.dragAccumX = 0
	ui.updateActionState()
}

func (ui *explorer) updateRowDrag(deltaX float32) {
	if !ui.dragActive {
		return
	}
	ui.dragAccumX += deltaX
}

func (ui *explorer) finishRowDrag() {
	if !ui.dragActive {
		return
	}
	const minCrossDrag = float32(120)
	fromLeft := ui.dragFromLeft
	acc := ui.dragAccumX
	ui.dragActive = false
	ui.dragAccumX = 0

	if fromLeft && acc > minCrossDrag {
		ui.status.SetText("Arrastar detectado: enviando para o servidor…")
		ui.upload()
		return
	}
	if !fromLeft && acc < -minCrossDrag {
		ui.status.SetText("Arrastar detectado: recebendo para o computador local…")
		ui.download()
		return
	}
}

func (ui *explorer) copySelectedEntry(left bool, id widget.ListItemID) {
	if left {
		if id < 0 || int(id) >= len(ui.leftRows) {
			return
		}
		e := ui.leftRows[id]
		if e.Name == ".." {
			return
		}
		ui.copiedEntry = &copiedItem{entry: e, fromLeft: true}
		ui.status.SetText("Copiado (local): " + e.Name)
		return
	}
	if id < 0 || int(id) >= len(ui.rightRows) {
		return
	}
	e := ui.rightRows[id]
	if e.Name == ".." {
		return
	}
	containerID := ""
	if !ui.hostMode && ui.cfs != nil {
		containerID = ui.cfs.ID
	}
	ui.copiedEntry = &copiedItem{
		entry:       e,
		fromLeft:    false,
		hostMode:    ui.hostMode,
		containerID: containerID,
	}
	ui.status.SetText("Copiado (servidor): " + e.Name)
}

func (ui *explorer) pasteCopiedTo(leftTarget bool, targetDir string) {
	if ui.copiedEntry == nil {
		dialog.ShowInformation("Colar", "Copie um arquivo ou pasta antes de colar.", ui.win)
		return
	}
	src := *ui.copiedEntry
	name := filepath.Base(src.entry.Path)
	if !src.fromLeft {
		name = path.Base(src.entry.Path)
	}

	// Local -> Local
	if src.fromLeft && leftTarget {
		dst := filepath.Join(targetDir, name)
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Copiar local:%s → local:%s", src.entry.Path, dst),
			Run: func(ctx context.Context, on transfer.Progress) error {
				if src.entry.IsDir {
					return copyLocalDir(ctx, src.entry.Path, dst)
				}
				return copyLocalFile(ctx, src.entry.Path, dst)
			},
		})
		ui.startDrain()
		return
	}
	// Local -> Remoto
	if src.fromLeft && !leftTarget {
		ui.enqueueLocalToRemote(src.entry, targetDir)
		ui.startDrain()
		return
	}
	// Remoto -> Local
	if !src.fromLeft && leftTarget {
		ui.enqueueRemoteToLocal(src, targetDir)
		ui.startDrain()
		return
	}
	// Remoto -> Remoto (mesmo contexto)
	if src.hostMode != ui.hostMode || (!src.hostMode && src.containerID != "" && ui.cfs != nil && src.containerID != ui.cfs.ID) {
		dialog.ShowInformation("Colar", "Para colar no remoto, mantenha o mesmo contexto (host ou contêiner).", ui.win)
		return
	}
	ui.enqueueRemoteToRemote(src, targetDir)
	ui.startDrain()
}

func copyLocalFile(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func copyLocalDir(ctx context.Context, srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(full string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(srcDir, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dstDir, 0o755)
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		return copyLocalFile(ctx, full, dst)
	})
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
	if ui.leftList == nil {
		return
	}
	selectedPath := ""
	if ui.leftSel >= 0 && ui.leftSel < len(ui.leftRows) {
		selectedPath = ui.leftRows[ui.leftSel].Path
	}
	criteria := parseRightFilterCriteria(ui.leftSearch.Text)
	typeChoice := "Tudo"
	if ui.leftTypeFilter != nil {
		typeChoice = strings.TrimSpace(ui.leftTypeFilter.Selected)
	}
	if criteria.term == "" && criteria.ext == "" && typeChoice == "Tudo" {
		ui.leftRows = append([]fsutil.DirEntry(nil), ui.leftAll...)
	} else {
		filtered := make([]fsutil.DirEntry, 0, len(ui.leftAll))
		for _, e := range ui.leftAll {
			if e.Name == ".." || rightEntryMatches(e, criteria, typeChoice) {
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
	if criteria.term != "" || criteria.ext != "" || typeChoice != "Tudo" {
		matches := 0
		for _, e := range ui.leftRows {
			if e.Name != ".." {
				matches++
			}
		}
		if matches == 0 {
			ui.status.SetText(fmt.Sprintf("Nenhum resultado para o filtro atual em %s.", ui.leftPath))
		}
	}
	ui.updateBreadcrumb()
	ui.updateActionState()
	ui.updateSummaryInfo()
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
				if ui.sudoEnabled {
					rows, err = ui.listHostWithSudo(ctx, p)
				} else {
					rows, err = hfs.List(ctx, p)
				}
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
				ui.maybePromptRootAccess(err)
				ui.rightAll = nil
				ui.rightRows = nil
				ui.rightSel = -1
				ui.rightList.UnselectAll()
				ui.rightList.Refresh()
				ui.updateSummaryInfo()
				return
			}
			ui.rightAll = rows
			ui.applyRightFilter()
			if strings.HasPrefix(ui.status.Text, "Carregando pastas") || strings.HasPrefix(ui.status.Text, "Sudo ativo") {
				ui.status.SetText("")
			}
		})
	}(seq)
}

func (ui *explorer) maybePromptRootAccess(listErr error) {
	if listErr == nil || !ui.hostMode {
		return
	}
	msg := strings.ToLower(listErr.Error())
	if !strings.Contains(msg, "permission denied") {
		return
	}
	if ui.rootPromptOpen.Load() {
		return
	}
	ui.rootPromptOpen.Store(true)

	userEntry := widget.NewEntry()
	userEntry.SetText(ui.connCreds.User)
	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("senha do usuário da sessão com sudo")
	userEntry.Resize(fyne.NewSize(260, userEntry.MinSize().Height))
	passEntry.Resize(fyne.NewSize(260, passEntry.MinSize().Height))

	ui.openFormDialogWithShortcuts(
		"Acesso negado",
		"Aplicar sudo",
		"Cancelar",
		fyne.NewSize(460, 240),
		[]*widget.FormItem{
			widget.NewFormItem("Usuário", userEntry),
			widget.NewFormItem("Senha", passEntry),
		},
		func() {
			defer ui.rootPromptOpen.Store(false)
			if strings.TrimSpace(passEntry.Text) == "" {
				dialog.ShowInformation("Credenciais obrigatórias", "Informe a senha para tentar acesso elevado via sudo.", ui.win)
				return
			}
			user := strings.TrimSpace(userEntry.Text)
			if user == "" {
				dialog.ShowInformation("Credenciais obrigatórias", "Informe o usuário para sudo.", ui.win)
				return
			}
			ui.enableSudoMode(user, passEntry.Text)
		},
		func() {
			defer ui.rootPromptOpen.Store(false)
			ui.status.SetText("Acesso elevado cancelado.")
		},
	)
}

func (ui *explorer) enableSudoMode(user, password string) {
	ui.status.SetText("Validando sudo no servidor…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		resolvedUser, err := ui.testSudoAccess(ctx, user, password)
		if err != nil {
			fyne.Do(func() {
				ui.status.SetText("Falha ao validar sudo.")
				dialog.ShowError(
					fmt.Errorf(
						"não foi possível elevar com sudo: %s\n\nLog de diagnóstico: %s",
						formatSudoErrorMessage(err),
						filepath.Join(os.TempDir(), "containerway-sudo-debug.log"),
					),
					ui.win,
				)
			})
			return
		}
		fyne.Do(func() {
			ui.sudoEnabled = true
			ui.sudoUser = resolvedUser
			ui.sudoPass = password
			ui.sudoValidatedAt = time.Now()
			ui.updateSudoUIState()
			ui.status.SetText(fmt.Sprintf("Sudo ativo (%s). Recarregando pasta…", resolvedUser))
			ui.refreshRight()
		})
	}()
}

func (ui *explorer) disableSudoMode() {
	ui.sudoEnabled = false
	ui.sudoUser = ""
	ui.sudoPass = ""
	ui.sudoValidatedAt = time.Time{}
	ui.updateSudoUIState()
	ui.status.SetText("Sudo desativado.")
}

func (ui *explorer) updateSudoUIState() {
	if ui.lblSudoState == nil || ui.btnDisableSudo == nil {
		return
	}
	if ui.sudoEnabled && strings.TrimSpace(ui.sudoUser) != "" {
		ui.lblSudoState.SetText("Sudo: ativo (" + ui.sudoUser + ")")
		ui.btnDisableSudo.Enable()
		return
	}
	ui.lblSudoState.SetText("Sudo: inativo")
	ui.btnDisableSudo.Disable()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func appendSudoDebugLog(line string) {
	logPath := filepath.Join(os.TempDir(), "containerway-sudo-debug.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " + line + "\n")
}

func formatSudoErrorMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "a password is required"), strings.Contains(low, "incorrect password"), strings.Contains(low, "try again"):
		return "Senha sudo incorreta."
	case strings.Contains(low, "is not in the sudoers"), strings.Contains(low, "not allowed to run sudo"):
		return "Este usuário não possui permissão no sudoers."
	case strings.Contains(low, "a terminal is required"), strings.Contains(low, "must be run from a terminal"), strings.Contains(low, "requiretty"):
		return "O servidor exige TTY para sudo (ajuste sudoers/SSH para uso não interativo)."
	default:
		return msg
	}
}

func (ui *explorer) runSSHCommandWithInput(ctx context.Context, cmd, input string) (string, string, error) {
	if ui.s == nil || ui.s.SSH == nil {
		return "", "", fmt.Errorf("sessão SSH indisponível")
	}
	ch := make(chan struct {
		stdout string
		stderr string
		err    error
	}, 1)
	go func() {
		sess, err := ui.s.SSH.NewSession()
		if err != nil {
			ch <- struct {
				stdout string
				stderr string
				err    error
			}{"", "", err}
			return
		}
		defer sess.Close()
		var outBuf, errBuf strings.Builder
		sess.Stdout = &outBuf
		sess.Stderr = &errBuf
		stdin, err := sess.StdinPipe()
		if err != nil {
			ch <- struct {
				stdout string
				stderr string
				err    error
			}{"", "", err}
			return
		}
		if err := sess.Start(cmd); err != nil {
			ch <- struct {
				stdout string
				stderr string
				err    error
			}{"", "", err}
			return
		}
		if input != "" {
			_, _ = io.WriteString(stdin, input+"\n")
		}
		_ = stdin.Close()
		err = sess.Wait()
		ch <- struct {
			stdout string
			stderr string
			err    error
		}{outBuf.String(), errBuf.String(), err}
	}()

	select {
	case <-ctx.Done():
		appendSudoDebugLog(fmt.Sprintf("runSSH timeout cmd=%q err=%v", cmd, ctx.Err()))
		return "", "", ctx.Err()
	case res := <-ch:
		appendSudoDebugLog(fmt.Sprintf("runSSH cmd=%q err=%v stderr=%q stdout=%q", cmd, res.err, strings.TrimSpace(res.stderr), strings.TrimSpace(res.stdout)))
		return res.stdout, res.stderr, res.err
	}
}

func (ui *explorer) ensureSudoSession(ctx context.Context) error {
	if !ui.sudoEnabled || strings.TrimSpace(ui.sudoUser) == "" || strings.TrimSpace(ui.sudoPass) == "" {
		return fmt.Errorf("sudo não configurado")
	}
	if !ui.sudoValidatedAt.IsZero() && time.Since(ui.sudoValidatedAt) < ui.sudoTTL {
		return nil
	}
	cmd := fmt.Sprintf("sudo -S -p '' -u %s -v", shellQuote(ui.sudoUser))
	_, stderr, err := ui.runSSHCommandWithInput(ctx, cmd, ui.sudoPass)
	if err != nil {
		fyne.Do(func() {
			ui.disableSudoMode()
		})
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("%s", strings.TrimSpace(stderr))
		}
		return err
	}
	ui.sudoValidatedAt = time.Now()
	return nil
}

func (ui *explorer) copyHostFileWithSudoToLocal(ctx context.Context, remotePath, localPath string) error {
	if err := ui.ensureSudoSession(ctx); err != nil {
		return err
	}
	if ui.s == nil || ui.s.SSH == nil {
		return fmt.Errorf("sessão SSH indisponível")
	}
	sess, err := ui.s.SSH.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	outFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	var errBuf strings.Builder
	sess.Stdout = outFile
	sess.Stderr = &errBuf
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(ui.sudoUser),
		shellQuote("cat -- "+shellQuote(path.Clean(remotePath))),
	)
	if err := sess.Start(cmd); err != nil {
		return err
	}
	_, _ = io.WriteString(stdin, ui.sudoPass+"\n")
	_ = stdin.Close()

	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf(msg)
		}
	}
	return nil
}

func (ui *explorer) copyLocalFileToHostWithSudo(ctx context.Context, localPath, remotePath string) error {
	if err := ui.ensureSudoSession(ctx); err != nil {
		return err
	}
	if ui.s == nil || ui.s.SSH == nil {
		return fmt.Errorf("sessão SSH indisponível")
	}
	in, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer in.Close()

	sess, err := ui.s.SSH.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	var errBuf strings.Builder
	sess.Stderr = &errBuf
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}

	target := path.Clean(remotePath)
	cmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(ui.sudoUser),
		shellQuote("cat > "+shellQuote(target)),
	)
	if err := sess.Start(cmd); err != nil {
		return err
	}
	if _, err := io.WriteString(stdin, ui.sudoPass+"\n"); err != nil {
		_ = stdin.Close()
		return err
	}
	if _, err := io.Copy(stdin, in); err != nil {
		_ = stdin.Close()
		return err
	}
	_ = stdin.Close()

	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf(msg)
		}
	}
	return nil
}

func localDirTotalBytes(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (ui *explorer) copyLocalDirToHostWithSudo(ctx context.Context, localDir, remoteDestDir string) (int64, error) {
	if err := ui.ensureSudoSession(ctx); err != nil {
		return 0, err
	}
	if ui.s == nil || ui.s.SSH == nil {
		return 0, fmt.Errorf("sessão SSH indisponível")
	}
	sess, err := ui.s.SSH.NewSession()
	if err != nil {
		return 0, err
	}
	defer sess.Close()

	var errBuf strings.Builder
	sess.Stderr = &errBuf
	stdin, err := sess.StdinPipe()
	if err != nil {
		return 0, err
	}
	remoteDest := path.Clean(remoteDestDir)
	cmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(ui.sudoUser),
		shellQuote("mkdir -p "+shellQuote(remoteDest)+" && tar -C "+shellQuote(remoteDest)+" -xf -"),
	)
	if err := sess.Start(cmd); err != nil {
		return 0, err
	}
	if _, err := io.WriteString(stdin, ui.sudoPass+"\n"); err != nil {
		_ = stdin.Close()
		return 0, err
	}
	if err := tarxfer.WriteLocalDirToTar(ctx, localDir, stdin); err != nil {
		_ = stdin.Close()
		return 0, err
	}
	_ = stdin.Close()
	if err := sess.Wait(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return 0, fmt.Errorf(msg)
	}
	return localDirTotalBytes(localDir), nil
}

func (ui *explorer) copyHostDirWithSudoToLocal(ctx context.Context, remoteDir, destLocalDir string) (int64, error) {
	if err := ui.ensureSudoSession(ctx); err != nil {
		return 0, err
	}
	if ui.s == nil || ui.s.SSH == nil {
		return 0, fmt.Errorf("sessão SSH indisponível")
	}
	if err := os.MkdirAll(destLocalDir, 0o755); err != nil {
		return 0, err
	}

	sess, err := ui.s.SSH.NewSession()
	if err != nil {
		return 0, err
	}
	defer sess.Close()

	var errBuf strings.Builder
	sess.Stderr = &errBuf
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return 0, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		return 0, err
	}

	cleanRemote := path.Clean(remoteDir)
	cmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(ui.sudoUser),
		shellQuote("tar -C "+shellQuote(cleanRemote)+" -cf - ."),
	)
	if err := sess.Start(cmd); err != nil {
		return 0, err
	}
	if _, err := io.WriteString(stdin, ui.sudoPass+"\n"); err != nil {
		_ = stdin.Close()
		return 0, err
	}
	_ = stdin.Close()

	extractDone := make(chan struct {
		written int64
		err     error
	}, 1)
	go func() {
		n, err := tarxfer.ExtractTarToLocalDir(stdout, destLocalDir)
		extractDone <- struct {
			written int64
			err     error
		}{written: n, err: err}
	}()

	waitDone := make(chan error, 1)
	go func() { waitDone <- sess.Wait() }()

	var written int64
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case ex := <-extractDone:
		if ex.err != nil {
			return 0, ex.err
		}
		written = ex.written
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case err := <-waitDone:
		if err != nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg == "" {
				msg = err.Error()
			}
			return 0, fmt.Errorf(msg)
		}
	}

	return written, nil
}

func (ui *explorer) testSudoAccess(ctx context.Context, user, password string) (string, error) {
	target := strings.TrimSpace(user)
	if target == "" {
		target = "root"
	}
	// Primeiro tenta com o usuário informado.
	uid, err := ui.sudoUID(ctx, target, password)
	if err == nil && uid == "0" {
		return target, nil
	}
	// Se não elevou e não era root, tenta automaticamente root.
	if !strings.EqualFold(target, "root") {
		uidRoot, errRoot := ui.sudoUID(ctx, "root", password)
		if errRoot == nil && uidRoot == "0" {
			return "root", nil
		}
		if errRoot != nil {
			return "", errRoot
		}
		return "", fmt.Errorf("sudo não elevou privilégios (uid=%s) nem com root", uidRoot)
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("sudo não elevou privilégios (uid=%s)", uid)
}

func (ui *explorer) sudoUID(ctx context.Context, user, password string) (string, error) {
	cmd := fmt.Sprintf("sudo -k -S -p '' -u %s sh -lc 'id -u'", shellQuote(user))
	stdout, stderr, err := ui.runSSHCommandWithInput(ctx, cmd, password)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return "", fmt.Errorf("%s", strings.TrimSpace(stderr))
		}
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

func (ui *explorer) listHostWithSudo(ctx context.Context, p string) ([]fsutil.DirEntry, error) {
	if !ui.sudoEnabled || strings.TrimSpace(ui.sudoUser) == "" || strings.TrimSpace(ui.sudoPass) == "" {
		return nil, fmt.Errorf("sudo não configurado")
	}
	if err := ui.ensureSudoSession(ctx); err != nil {
		return nil, err
	}
	clean := path.Clean(strings.TrimSpace(p))
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	listCmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(ui.sudoUser),
		shellQuote("id -u; id -un; ls -1Ap -- "+shellQuote(clean)),
	)
	stdout, stderr, err := ui.runSSHCommandWithInput(ctx, listCmd, ui.sudoPass)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf(msg)
	}
	lines := strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n")
	if len(lines) < 3 {
		return nil, fmt.Errorf("resposta inesperada do sudo ao listar pasta")
	}
	uidLine := strings.TrimSpace(lines[0])
	userLine := strings.TrimSpace(lines[1])
	if uidLine != "0" {
		return nil, fmt.Errorf("sudo não elevou privilégios (uid=%s, user=%s)", uidLine, userLine)
	}

	out := make([]fsutil.DirEntry, 0, 64)
	if clean != "/" {
		parent := path.Dir(clean)
		if parent == "" || parent == "." {
			parent = "/"
		}
		out = append(out, fsutil.DirEntry{Name: "..", Path: parent, IsDir: true})
	}
	for _, line := range lines[2:] {
		item := strings.TrimSpace(line)
		if item == "" || item == "." || item == ".." {
			continue
		}
		isDir := strings.HasSuffix(item, "/")
		name := strings.TrimSuffix(item, "/")
		if name == "" || name == "." || name == ".." {
			continue
		}
		out = append(out, fsutil.DirEntry{
			Name:    name,
			Path:    path.Join(clean, name),
			IsDir:   isDir,
			Size:    0,
			ModTime: time.Now(),
		})
	}
	fsutil.SortLikeWinSCP(out)
	return out, nil
}

func (ui *explorer) applyRightFilter() {
	if ui.rightList == nil {
		return
	}
	selectedPath := ""
	if ui.rightSel >= 0 && ui.rightSel < len(ui.rightRows) {
		selectedPath = ui.rightRows[ui.rightSel].Path
	}
	criteria := parseRightFilterCriteria(ui.rightSearch.Text)
	typeChoice := "Tudo"
	if ui.rightTypeFilter != nil {
		typeChoice = strings.TrimSpace(ui.rightTypeFilter.Selected)
	}
	if criteria.term == "" && criteria.ext == "" && typeChoice == "Tudo" {
		ui.rightRows = append([]fsutil.DirEntry(nil), ui.rightAll...)
	} else {
		filtered := make([]fsutil.DirEntry, 0, len(ui.rightAll))
		for _, e := range ui.rightAll {
			if e.Name == ".." || rightEntryMatches(e, criteria, typeChoice) {
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
	if criteria.term != "" || criteria.ext != "" || typeChoice != "Tudo" {
		matches := 0
		for _, e := range ui.rightRows {
			if e.Name != ".." {
				matches++
			}
		}
		if matches == 0 {
			ui.status.SetText(fmt.Sprintf("Nenhum resultado para o filtro atual em %s.", ui.rightPath))
		}
	}
	ui.updateBreadcrumb()
	ui.updateActionState()
	ui.updateSummaryInfo()
}

type rightFilterCriteria struct {
	term string
	ext  string
	kind string
}

func parseRightFilterCriteria(raw string) rightFilterCriteria {
	out := rightFilterCriteria{}
	parts := strings.Fields(strings.TrimSpace(raw))
	terms := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(strings.ToLower(p))
		switch {
		case strings.HasPrefix(t, "ext:"):
			out.ext = strings.TrimPrefix(t, "ext:")
			out.ext = strings.TrimPrefix(out.ext, ".")
		case strings.HasPrefix(t, "tipo:"):
			v := strings.TrimPrefix(t, "tipo:")
			if v == "pasta" || v == "arquivo" {
				out.kind = v
			}
		case strings.HasPrefix(t, "type:"):
			v := strings.TrimPrefix(t, "type:")
			if v == "folder" {
				out.kind = "pasta"
			}
			if v == "file" {
				out.kind = "arquivo"
			}
		default:
			terms = append(terms, t)
		}
	}
	out.term = strings.Join(terms, " ")
	return out
}

func rightEntryMatches(e fsutil.DirEntry, c rightFilterCriteria, typeChoice string) bool {
	name := strings.ToLower(e.Name)
	if c.term != "" && !strings.Contains(name, c.term) {
		return false
	}
	requiredKind := strings.ToLower(strings.TrimSpace(typeChoice))
	switch requiredKind {
	case "pastas":
		if !e.IsDir {
			return false
		}
	case "arquivos":
		if e.IsDir {
			return false
		}
	}
	if c.kind == "pasta" && !e.IsDir {
		return false
	}
	if c.kind == "arquivo" && e.IsDir {
		return false
	}
	if c.ext != "" {
		if e.IsDir {
			return false
		}
		return strings.HasSuffix(name, "."+c.ext)
	}
	return true
}

func (ui *explorer) goLeftBack() {
	n := len(ui.leftBack)
	if n == 0 {
		return
	}
	prev := ui.leftBack[n-1]
	ui.leftBack = ui.leftBack[:n-1]
	ui.leftPath = prev
	ui.resetLeftSearch()
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
	ui.resetRightSearch()
	ui.refreshRight()
}

func (ui *explorer) goLeftUp() {
	parent := filepath.Dir(ui.leftPath)
	if parent == ui.leftPath {
		return
	}
	ui.leftBack = append(ui.leftBack, ui.leftPath)
	ui.leftPath = parent
	ui.resetLeftSearch()
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
	ui.resetRightSearch()
	ui.refreshRight()
}

func (ui *explorer) goLeftHome() {
	if ui.leftPath == homeOrRoot() {
		return
	}
	ui.leftBack = append(ui.leftBack, ui.leftPath)
	ui.leftPath = homeOrRoot()
	ui.resetLeftSearch()
	ui.refreshLeft()
}

func (ui *explorer) goRightHome() {
	if ui.rightPath == "/" {
		return
	}
	ui.rightBack = append(ui.rightBack, ui.rightPath)
	ui.rightPath = "/"
	ui.resetRightSearch()
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
		ui.resetLeftSearch()
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
		ui.resetRightSearch()
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
					if ui.sudoEnabled {
						n, err := ui.copyLocalDirToHostWithSudo(ctx, src.Path, remoteBase)
						if on != nil {
							on(n, max(n, int64(1)))
						}
						return err
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
				if ui.sudoEnabled {
					st, err := os.Stat(src.Path)
					if err != nil {
						return err
					}
					if on != nil {
						on(0, st.Size())
					}
					if err := ui.copyLocalFileToHostWithSudo(ctx, src.Path, dst); err != nil {
						return err
					}
					if on != nil {
						on(st.Size(), st.Size())
					}
					return nil
				}
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
					if ui.sudoEnabled {
						n, err := ui.copyHostDirWithSudoToLocal(ctx, src.Path, dstPath)
						if on != nil {
							on(n, max(n, int64(1)))
						}
						return err
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
				if ui.sudoEnabled {
					if on != nil {
						on(0, -1)
					}
					return ui.copyHostFileWithSudoToLocal(ctx, src.Path, dstPath)
				}
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

func transferableEntries(rows []fsutil.DirEntry) []fsutil.DirEntry {
	out := make([]fsutil.DirEntry, 0, len(rows))
	for _, e := range rows {
		if e.Name == ".." {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (ui *explorer) uploadVisibleBatch() {
	items := transferableEntries(ui.leftRows)
	if len(items) == 0 {
		dialog.ShowInformation("ContainerWay", "Não há itens visíveis no painel local para enviar.", ui.win)
		return
	}
	msg := fmt.Sprintf("Enviar %d item(ns) visível(is) para %s?", len(items), ui.rightPath)
	dialog.ShowConfirm("Enviar em lote", msg, func(ok bool) {
		if !ok {
			return
		}
		ui.beginBatch("Envio em lote", len(items))
		for _, entry := range items {
			ui.enqueueLocalToRemote(entry, ui.rightPath)
		}
		ui.status.SetText(fmt.Sprintf("Fila iniciada: %d item(ns) para envio", len(items)))
		ui.startDrain()
	}, ui.win)
}

func (ui *explorer) downloadVisibleBatch() {
	items := transferableEntries(ui.rightRows)
	if len(items) == 0 {
		dialog.ShowInformation("ContainerWay", "Não há itens visíveis no painel do servidor para receber.", ui.win)
		return
	}
	msg := fmt.Sprintf("Receber %d item(ns) visível(is) em %s?", len(items), ui.leftPath)
	dialog.ShowConfirm("Receber em lote", msg, func(ok bool) {
		if !ok {
			return
		}
		ui.beginBatch("Recebimento em lote", len(items))
		containerID := ""
		if !ui.hostMode && ui.cfs != nil {
			containerID = ui.cfs.ID
		}
		for _, entry := range items {
			ui.enqueueRemoteToLocal(copiedItem{
				entry:       entry,
				hostMode:    ui.hostMode,
				containerID: containerID,
			}, ui.leftPath)
		}
		ui.status.SetText(fmt.Sprintf("Fila iniciada: %d item(ns) para recebimento", len(items)))
		ui.startDrain()
	}, ui.win)
}

func (ui *explorer) beginBatch(label string, total int) {
	ui.batchMu.Lock()
	defer ui.batchMu.Unlock()
	ui.batchRunning = total > 0
	ui.batchLabel = label
	ui.batchTotal = total
	ui.batchDone = 0
	ui.batchFailures = nil
}

func (ui *explorer) batchSnapshot() (running bool, done int, total int) {
	ui.batchMu.Lock()
	defer ui.batchMu.Unlock()
	return ui.batchRunning, ui.batchDone, ui.batchTotal
}

func (ui *explorer) consumeBatchResult(job transfer.Job, err error) (active bool, finished bool, done int, total int, progress string, summary string, summaryErr error) {
	ui.batchMu.Lock()
	defer ui.batchMu.Unlock()
	if !ui.batchRunning {
		return false, false, 0, 0, "", "", nil
	}
	ui.batchDone++
	if err != nil {
		ui.batchFailures = append(ui.batchFailures, fmt.Sprintf("%s: %v", job.Name, err))
	}
	if ui.batchDone < ui.batchTotal {
		return true, false, ui.batchDone, ui.batchTotal, fmt.Sprintf("Lote em andamento: %d/%d", ui.batchDone, ui.batchTotal), "", nil
	}
	total = ui.batchTotal
	fails := len(ui.batchFailures)
	ok := total - fails
	label := strings.TrimSpace(ui.batchLabel)
	if label == "" {
		label = "Transferência em lote"
	}
	ui.batchRunning = false
	ui.batchLabel = ""
	ui.batchTotal = 0
	ui.batchDone = 0
	if fails == 0 {
		return true, true, total, total, "", fmt.Sprintf("%s concluída: %d sucesso(s), 0 falha(s).", label, ok), nil
	}
	preview := ui.batchFailures
	if len(preview) > 4 {
		preview = preview[:4]
	}
	msg := fmt.Sprintf("%s concluída: %d sucesso(s), %d falha(s).\n\nFalhas:\n- %s", label, ok, fails, strings.Join(preview, "\n- "))
	if len(ui.batchFailures) > len(preview) {
		msg += fmt.Sprintf("\n- ... e mais %d falha(s)", len(ui.batchFailures)-len(preview))
	}
	return true, true, total, total, "", "", errors.New(msg)
}

func (ui *explorer) enqueueLocalToRemote(src fsutil.DirEntry, dstDir string) {
	dstName := filepath.Base(src.Path)
	if src.IsDir {
		if ui.hostMode {
			remoteBase := path.Join(dstDir, dstName)
			ui.tm.Enqueue(transfer.Job{
				Name: fmt.Sprintf("Copiar local:%s → servidor:%s", src.Path, remoteBase),
				Run: func(ctx context.Context, on transfer.Progress) error {
					if ui.sudoEnabled {
						_, err := ui.copyLocalDirToHostWithSudo(ctx, src.Path, remoteBase)
						return err
					}
					_, err := tarxfer.SFTPUploadLocalTree(ctx, src.Path, remoteBase, ui.hfs.Client)
					return err
				},
			})
			return
		}
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Copiar local:%s → contêiner:%s", src.Path, dstDir),
			Run: func(ctx context.Context, on transfer.Progress) error {
				return tarxfer.UploadLocalDirToContainer(ctx, ui.s.Docker, ui.cfs.ID, src.Path, dstDir)
			},
		})
		return
	}
	if ui.hostMode {
		dst := path.Join(dstDir, dstName)
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Copiar local:%s → servidor:%s", src.Path, dst),
			Run: func(ctx context.Context, on transfer.Progress) error {
				if ui.sudoEnabled {
					return ui.copyLocalFileToHostWithSudo(ctx, src.Path, dst)
				}
				f, err := os.Open(src.Path)
				if err != nil {
					return err
				}
				defer f.Close()
				wf, err := ui.hfs.CreateWriter(dst)
				if err != nil {
					return err
				}
				defer wf.Close()
				_, err = io.Copy(wf, f)
				return err
			},
		})
		return
	}
	ui.tm.Enqueue(transfer.Job{
		Name: fmt.Sprintf("Copiar local:%s → contêiner:%s", src.Path, dstDir),
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
			return ui.cfs.UploadFile(ctx, dstDir, dstName, f, st.Size())
		},
	})
}

func (ui *explorer) enqueueRemoteToLocal(src copiedItem, dstDir string) {
	dstPath := filepath.Join(dstDir, path.Base(src.entry.Path))
	if src.entry.IsDir {
		if src.hostMode {
			ui.tm.Enqueue(transfer.Job{
				Name: fmt.Sprintf("Copiar servidor:%s → local:%s", src.entry.Path, dstPath),
				Run: func(ctx context.Context, on transfer.Progress) error {
					if ui.sudoEnabled {
						_, err := ui.copyHostDirWithSudoToLocal(ctx, src.entry.Path, dstPath)
						return err
					}
					_, err := tarxfer.SFTPDownloadTree(ctx, ui.hfs.Client, src.entry.Path, dstPath)
					return err
				},
			})
			return
		}
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Copiar contêiner:%s → local:%s", src.entry.Path, dstPath),
			Run: func(ctx context.Context, on transfer.Progress) error {
				cid := src.containerID
				if cid == "" && ui.cfs != nil {
					cid = ui.cfs.ID
				}
				_, err := tarxfer.ExtractContainerDirToLocal(ctx, ui.s.Docker, cid, src.entry.Path, dstPath)
				return err
			},
		})
		return
	}
	if src.hostMode {
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Copiar servidor:%s → local:%s", src.entry.Path, dstPath),
			Run: func(ctx context.Context, on transfer.Progress) error {
				if ui.sudoEnabled {
					return ui.copyHostFileWithSudoToLocal(ctx, src.entry.Path, dstPath)
				}
				rf, err := ui.hfs.OpenReader(src.entry.Path)
				if err != nil {
					return err
				}
				defer rf.Close()
				out, err := os.Create(dstPath)
				if err != nil {
					return err
				}
				defer out.Close()
				_, err = io.Copy(out, rf)
				return err
			},
		})
		return
	}
	ui.tm.Enqueue(transfer.Job{
		Name: fmt.Sprintf("Copiar contêiner:%s → local:%s", src.entry.Path, dstPath),
		Run: func(ctx context.Context, on transfer.Progress) error {
			cid := src.containerID
			if cid == "" && ui.cfs != nil {
				cid = ui.cfs.ID
			}
			cfs := &containerfs.FS{Docker: ui.s.Docker, ID: cid}
			rc, _, err := cfs.OpenFileReader(ctx, src.entry.Path)
			if err != nil {
				return err
			}
			defer rc.Close()
			out, err := os.Create(dstPath)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			return err
		},
	})
}

func (ui *explorer) enqueueRemoteToRemote(src copiedItem, dstDir string) {
	name := path.Base(src.entry.Path)
	if src.entry.IsDir {
		if src.hostMode {
			ui.tm.Enqueue(transfer.Job{
				Name: fmt.Sprintf("Copiar servidor:%s → servidor:%s", src.entry.Path, dstDir),
				Run: func(ctx context.Context, on transfer.Progress) error {
					if ui.sudoEnabled {
						tmpDir, err := os.MkdirTemp("", "containerway-copy-*")
						if err != nil {
							return err
						}
						defer os.RemoveAll(tmpDir)
						localTmp := filepath.Join(tmpDir, name)
						if _, err := ui.copyHostDirWithSudoToLocal(ctx, src.entry.Path, localTmp); err != nil {
							return err
						}
						_, err = ui.copyLocalDirToHostWithSudo(ctx, localTmp, path.Join(dstDir, name))
						return err
					}
					return fmt.Errorf("copiar pasta servidor->servidor sem sudo ainda não suportado")
				},
			})
			return
		}
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Copiar contêiner:%s → contêiner:%s", src.entry.Path, dstDir),
			Run: func(ctx context.Context, on transfer.Progress) error {
				rc, _, err := ui.s.Docker.CopyFromContainer(ctx, src.containerID, src.entry.Path)
				if err != nil {
					return err
				}
				defer rc.Close()
				opts := dcontainer.CopyToContainerOptions{AllowOverwriteDirWithFile: true}
				return ui.s.Docker.CopyToContainer(ctx, src.containerID, dstDir, rc, opts)
			},
		})
		return
	}
	if src.hostMode {
		dst := path.Join(dstDir, name)
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Copiar servidor:%s → servidor:%s", src.entry.Path, dst),
			Run: func(ctx context.Context, on transfer.Progress) error {
				if ui.sudoEnabled {
					tmpFile, err := os.CreateTemp("", "containerway-copy-*")
					if err != nil {
						return err
					}
					tmpPath := tmpFile.Name()
					_ = tmpFile.Close()
					defer os.Remove(tmpPath)
					if err := ui.copyHostFileWithSudoToLocal(ctx, src.entry.Path, tmpPath); err != nil {
						return err
					}
					return ui.copyLocalFileToHostWithSudo(ctx, tmpPath, dst)
				}
				return fmt.Errorf("copiar arquivo servidor->servidor sem sudo ainda não suportado")
			},
		})
		return
	}
	ui.tm.Enqueue(transfer.Job{
		Name: fmt.Sprintf("Copiar contêiner:%s → contêiner:%s", src.entry.Path, dstDir),
		Run: func(ctx context.Context, on transfer.Progress) error {
			cfs := &containerfs.FS{Docker: ui.s.Docker, ID: src.containerID}
			rc, size, err := cfs.OpenFileReader(ctx, src.entry.Path)
			if err != nil {
				return err
			}
			defer rc.Close()
			return cfs.UploadFile(ctx, dstDir, name, rc, size)
		},
	})
}

func (ui *explorer) startDrain() {
	ctx := context.Background()
	ui.tm.DrainAsync(ctx, ui.parallelJobs,
		func(j transfer.Job) {
			fyne.Do(func() {
				ui.progress.Show()
				batchRunning, batchDone, batchTotal := ui.batchSnapshot()
				if batchRunning && batchTotal > 0 {
					ui.progress.SetValue(float64(batchDone) / float64(batchTotal))
				} else {
					ui.progress.SetValue(0)
				}
				ui.lastJobText.SetText(j.Name)
				ui.status.SetText("Transferindo…")
			})
		},
		func(j transfer.Job, err error) {
			fyne.Do(func() {
				batchActive, batchFinished, batchDone, batchTotal, batchProgress, batchSummary, batchErr := ui.consumeBatchResult(j, err)
				if batchActive {
					ui.progress.Show()
					if batchTotal > 0 {
						ui.progress.SetValue(float64(batchDone) / float64(batchTotal))
					}
					if batchFinished {
						ui.progress.Hide()
						if batchErr != nil {
							ui.status.SetText(batchErr.Error())
							dialog.ShowError(batchErr, ui.win)
						} else {
							ui.status.SetText(batchSummary)
							dialog.ShowInformation("Transferência em lote concluída", batchSummary, ui.win)
						}
					} else {
						ui.status.SetText(batchProgress)
					}
					ui.refreshLeft()
					ui.refreshRightQuiet()
					return
				}
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
				if batchRunning, _, _ := ui.batchSnapshot(); batchRunning {
					return
				}
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
	if ui.btnLeftSend != nil {
		if hasLeft && left.Name != ".." {
			ui.btnLeftSend.Enable()
		} else {
			ui.btnLeftSend.Disable()
		}
	}
	if ui.btnRightRecv != nil {
		if hasRight && right.Name != ".." {
			ui.btnRightRecv.Enable()
		} else {
			ui.btnRightRecv.Disable()
		}
	}
	_ = left
	_ = right
	_ = hasLeft
	_ = hasRight
	ui.updateFooterPanels()
}

func entryTypeLabel(e fsutil.DirEntry) string {
	if e.IsDir {
		return "pasta"
	}
	return "arquivo"
}

func summarizeEntries(rows []fsutil.DirEntry) (dirs int, files int) {
	for _, e := range rows {
		if e.Name == ".." {
			continue
		}
		if e.IsDir {
			dirs++
			continue
		}
		files++
	}
	return dirs, files
}

func (ui *explorer) updateSummaryInfo() {
	ui.updateFooterPanels()
}

func (ui *explorer) updateFooterPanels() {
	if ui.leftFooterInfo == nil || ui.rightFooterInfo == nil {
		return
	}
	leftDirs, leftFiles := summarizeEntries(ui.leftRows)
	rightDirs, rightFiles := summarizeEntries(ui.rightRows)

	leftSelText := "nenhum item selecionado"
	if left, ok := ui.selectedLeftEntry(); ok {
		leftActionHint := "ações: abrir/enviar disponíveis"
		if left.Name == ".." {
			leftActionHint = "ações: usar voltar/subir; enviar indisponível"
		}
		leftSelText = fmt.Sprintf("selecionado: %s (%s) | %s", left.Name, entryTypeLabel(left), leftActionHint)
	} else {
		leftSelText = "nenhum item selecionado | ações: selecione um item para abrir/enviar"
	}
	rightSelText := "nenhum item selecionado"
	if right, ok := ui.selectedRightEntry(); ok {
		rightActionHint := "ações: abrir/receber disponíveis"
		if right.Name == ".." {
			rightActionHint = "ações: usar voltar/subir; receber indisponível"
		}
		rightSelText = fmt.Sprintf("selecionado: %s (%s) | %s", right.Name, entryTypeLabel(right), rightActionHint)
	} else {
		rightSelText = "nenhum item selecionado | ações: selecione um item para abrir/receber"
	}

	rightPathLabel := fmt.Sprintf("Pasta servidor: %s", ui.rightPath)
	if !ui.hostMode && ui.cfs != nil {
		short := strings.TrimPrefix(ui.cfs.ID, "sha256:")
		if len(short) > 12 {
			short = short[:12]
		}
		rightPathLabel = fmt.Sprintf("Pasta contêiner (%s): %s", short, ui.rightPath)
	}

	ui.leftFooterInfo.SetText(fmt.Sprintf(
		"Pasta local: %s | Itens: %d pastas, %d arquivos | %s",
		ui.leftPath, leftDirs, leftFiles, leftSelText,
	))
	ui.rightFooterInfo.SetText(fmt.Sprintf(
		"%s | Itens: %d pastas, %d arquivos | %s",
		rightPathLabel, rightDirs, rightFiles, rightSelText,
	))
}

func (ui *explorer) registerExplorerShortcuts() {
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEscape}, func(fyne.Shortcut) {
		ui.triggerDialogCancel()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn}, func(fyne.Shortcut) {
		ui.triggerDialogConfirm()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEnter}, func(fyne.Shortcut) {
		ui.triggerDialogConfirm()
	})
	ui.win.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if ui.dialogShortcutActive.Load() {
			return
		}
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
		case fyne.KeyTab:
			if ui.activePane == "left" {
				ui.activePane = "right"
				if len(ui.rightRows) > 0 && ui.rightSel < 0 {
					ui.rightSel = 0
					ui.rightList.Select(0)
				}
			} else {
				ui.activePane = "left"
				if len(ui.leftRows) > 0 && ui.leftSel < 0 {
					ui.leftSel = 0
					ui.leftList.Select(0)
				}
			}
			ui.updateActionState()
		case fyne.KeyF3:
			ui.focusActiveSearch()
		case fyne.KeyF6:
			if ui.activePane == "left" {
				ui.upload()
			} else {
				ui.download()
			}
		case fyne.KeyF2:
			ui.renameActive()
		case fyne.KeyDelete:
			ui.deleteActive()
		case fyne.KeyF5:
			ui.refreshLeft()
			ui.refreshRight()
		}
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		ui.focusActiveSearch()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(fyne.Shortcut) {
		ui.createFolderActive()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF6, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(fyne.Shortcut) {
		if ui.activePane == "left" {
			ui.uploadVisibleBatch()
		} else {
			ui.downloadVisibleBatch()
		}
	})
}

func (ui *explorer) triggerDialogConfirm() {
	if !ui.dialogShortcutActive.Load() {
		return
	}
	if ui.dialogConfirmAction != nil {
		ui.dialogConfirmAction()
	}
}

func (ui *explorer) triggerDialogCancel() {
	if !ui.dialogShortcutActive.Load() {
		return
	}
	if ui.dialogCancelAction != nil {
		ui.dialogCancelAction()
	}
}

func (ui *explorer) openFormDialogWithShortcuts(
	title string,
	confirmText string,
	cancelText string,
	size fyne.Size,
	items []*widget.FormItem,
	onConfirm func(),
	onCancel func(),
) {
	done := atomic.Bool{}
	var formDlg dialog.Dialog

	confirmOnce := func() {
		if done.Swap(true) {
			return
		}
		ui.dialogShortcutActive.Store(false)
		ui.dialogConfirmAction = nil
		ui.dialogCancelAction = nil
		if formDlg != nil {
			formDlg.Hide()
		}
		if onConfirm != nil {
			onConfirm()
		}
	}
	cancelOnce := func() {
		if done.Swap(true) {
			return
		}
		ui.dialogShortcutActive.Store(false)
		ui.dialogConfirmAction = nil
		ui.dialogCancelAction = nil
		if formDlg != nil {
			formDlg.Hide()
		}
		if onCancel != nil {
			onCancel()
		}
	}

	formDlg = dialog.NewForm(
		title,
		confirmText,
		cancelText,
		items,
		func(ok bool) {
			if ok {
				confirmOnce()
				return
			}
			cancelOnce()
		},
		ui.win,
	)
	ui.dialogConfirmAction = confirmOnce
	ui.dialogCancelAction = cancelOnce
	ui.dialogShortcutActive.Store(true)

	for _, it := range items {
		if e, ok := it.Widget.(*widget.Entry); ok {
			e.OnSubmitted = func(_ string) {
				confirmOnce()
			}
		}
	}
	if size.Width > 0 && size.Height > 0 {
		formDlg.Resize(size)
	}
	formDlg.Show()
}

func (ui *explorer) focusActiveSearch() {
	if ui.activePane == "left" {
		ui.win.Canvas().Focus(ui.leftSearch)
		return
	}
	ui.win.Canvas().Focus(ui.rightSearch)
}

func (ui *explorer) resetLeftSearch() {
	if ui.leftSearch != nil && strings.TrimSpace(ui.leftSearch.Text) != "" {
		ui.leftSearch.SetText("")
	}
	if ui.leftTypeFilter != nil && strings.TrimSpace(ui.leftTypeFilter.Selected) != "Tudo" {
		ui.leftTypeFilter.SetSelected("Tudo")
	}
}

func (ui *explorer) resetRightSearch() {
	if ui.rightSearch != nil && strings.TrimSpace(ui.rightSearch.Text) != "" {
		ui.rightSearch.SetText("")
	}
	if ui.rightTypeFilter != nil && strings.TrimSpace(ui.rightTypeFilter.Selected) != "Tudo" {
		ui.rightTypeFilter.SetSelected("Tudo")
	}
}

func (ui *explorer) renameActive() {
	if ui.activePane == "left" {
		e, ok := ui.selectedLeftEntry()
		if !ok || e.Name == ".." {
			dialog.ShowInformation("Renomear", "Selecione um item válido no painel local.", ui.win)
			return
		}
		name := widget.NewEntry()
		name.SetText(e.Name)
		ui.openFormDialogWithShortcuts(
			"Renomear (local)",
			"Salvar",
			"Cancelar",
			fyne.NewSize(460, 220),
			[]*widget.FormItem{
				widget.NewFormItem("Novo nome", name),
			},
			func() {
				newName := strings.TrimSpace(name.Text)
				if newName == "" || newName == e.Name {
					return
				}
				target := filepath.Join(filepath.Dir(e.Path), newName)
				if err := localfs.Rename(e.Path, target); err != nil {
					dialog.ShowError(fmt.Errorf("não foi possível renomear item local: %w", err), ui.win)
					return
				}
				ui.status.SetText("Item local renomeado com sucesso.")
				ui.refreshLeft()
			},
			nil,
		)
		return
	}
	e, ok := ui.selectedRightEntry()
	if !ok || e.Name == ".." {
		dialog.ShowInformation("Renomear", "Selecione um item válido no painel do servidor.", ui.win)
		return
	}
	name := widget.NewEntry()
	name.SetText(e.Name)
	ui.openFormDialogWithShortcuts(
		"Renomear (servidor)",
		"Salvar",
		"Cancelar",
		fyne.NewSize(460, 220),
		[]*widget.FormItem{
			widget.NewFormItem("Novo nome", name),
		},
		func() {
			newName := strings.TrimSpace(name.Text)
			if newName == "" || newName == e.Name {
				return
			}
			target := path.Join(path.Dir(e.Path), newName)
			var err error
			if ui.hostMode {
				err = ui.hfs.Rename(e.Path, target)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				err = ui.cfs.Rename(ctx, e.Path, target)
			}
			if err != nil {
				dialog.ShowError(fmt.Errorf("não foi possível renomear item remoto: %w", err), ui.win)
				return
			}
			ui.status.SetText("Item remoto renomeado com sucesso.")
			ui.refreshRight()
		},
		nil,
	)
}

func (ui *explorer) deleteActive() {
	if ui.activePane == "left" {
		e, ok := ui.selectedLeftEntry()
		if !ok || e.Name == ".." {
			dialog.ShowInformation("Excluir", "Selecione um item válido no painel local.", ui.win)
			return
		}
		msg := fmt.Sprintf("Deseja excluir \"%s\" do computador local?", e.Name)
		dialog.ShowConfirm("Confirmar exclusão", msg, func(confirm bool) {
			if !confirm {
				ui.status.SetText("Exclusão cancelada.")
				return
			}
			if err := localfs.Remove(e.Path, e.IsDir); err != nil {
				dialog.ShowError(fmt.Errorf("não foi possível excluir item local: %w", err), ui.win)
				return
			}
			ui.status.SetText("Item local excluído com sucesso.")
			ui.refreshLeft()
		}, ui.win)
		return
	}
	e, ok := ui.selectedRightEntry()
	if !ok || e.Name == ".." {
		dialog.ShowInformation("Excluir", "Selecione um item válido no painel do servidor.", ui.win)
		return
	}
	msg := fmt.Sprintf("Deseja excluir \"%s\" do servidor?", e.Name)
	dialog.ShowConfirm("Confirmar exclusão", msg, func(confirm bool) {
		if !confirm {
			ui.status.SetText("Exclusão cancelada.")
			return
		}
		var err error
		if ui.hostMode {
			err = ui.hfs.Remove(e.Path, e.IsDir)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			err = ui.cfs.Remove(ctx, e.Path, e.IsDir)
		}
		if err != nil {
			dialog.ShowError(fmt.Errorf("não foi possível excluir item remoto: %w", err), ui.win)
			return
		}
		ui.status.SetText("Item remoto excluído com sucesso.")
		ui.refreshRight()
	}, ui.win)
}

func (ui *explorer) createFolderActive() {
	name := widget.NewEntry()
	title := "Nova pasta (local)"
	if ui.activePane == "right" {
		title = "Nova pasta (servidor)"
	}
	ui.openFormDialogWithShortcuts(
		title,
		"Criar",
		"Cancelar",
		fyne.Size{},
		[]*widget.FormItem{
			widget.NewFormItem("Nome da pasta", name),
		},
		func() {
			folderName := strings.TrimSpace(name.Text)
			if folderName == "" {
				return
			}
			if ui.activePane == "left" {
				target := filepath.Join(ui.leftPath, folderName)
				if err := localfs.Mkdir(target); err != nil {
					dialog.ShowError(fmt.Errorf("não foi possível criar pasta local: %w", err), ui.win)
					return
				}
				ui.status.SetText("Pasta local criada com sucesso.")
				ui.refreshLeft()
				return
			}
			target := path.Join(ui.rightPath, folderName)
			var err error
			if ui.hostMode {
				err = ui.hfs.Mkdir(target)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				err = ui.cfs.Mkdir(ctx, target)
			}
			if err != nil {
				dialog.ShowError(fmt.Errorf("não foi possível criar pasta remota: %w", err), ui.win)
				return
			}
			ui.status.SetText("Pasta remota criada com sucesso.")
			ui.refreshRight()
		},
		nil,
	)
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
				ui.resetLeftSearch()
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
		ui.resetRightSearch()
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
			ui.resetRightSearch()
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
		targetDir := ui.leftPath
		if e.IsDir && e.Name != ".." {
			targetDir = e.Path
		}
		items := []*fyne.MenuItem{
			fyne.NewMenuItem("Abrir pasta", func() { ui.onLeftActivate() }),
			fyne.NewMenuItem("Enviar para o servidor", func() { ui.upload() }),
			fyne.NewMenuItem("Enviar itens visíveis", func() { ui.uploadVisibleBatch() }),
			fyne.NewMenuItem("Copiar", func() { ui.copySelectedEntry(true, id) }),
			fyne.NewMenuItem("Colar aqui", func() { ui.pasteCopiedTo(true, targetDir) }),
			fyne.NewMenuItem("Renomear", func() { ui.renameActive() }),
			fyne.NewMenuItem("Excluir", func() { ui.deleteActive() }),
			fyne.NewMenuItem("Nova pasta aqui", func() { ui.createFolderActive() }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Atualizar lista local", func() { ui.refreshLeft() }),
		}
		if !e.IsDir {
			items[0].Disabled = true
		}
		if e.Name == ".." {
			items[1].Disabled = true
		}
		if ui.copiedEntry == nil {
			items[3].Disabled = true
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
	targetDir := ui.rightPath
	if e.IsDir && e.Name != ".." {
		targetDir = e.Path
	}
	items := []*fyne.MenuItem{
		fyne.NewMenuItem("Abrir pasta", func() { ui.onRightActivate() }),
		fyne.NewMenuItem("Receber no computador local", func() { ui.download() }),
		fyne.NewMenuItem("Receber itens visíveis", func() { ui.downloadVisibleBatch() }),
		fyne.NewMenuItem("Copiar", func() { ui.copySelectedEntry(false, id) }),
		fyne.NewMenuItem("Colar aqui", func() { ui.pasteCopiedTo(false, targetDir) }),
		fyne.NewMenuItem("Renomear", func() { ui.renameActive() }),
		fyne.NewMenuItem("Excluir", func() { ui.deleteActive() }),
		fyne.NewMenuItem("Nova pasta aqui", func() { ui.createFolderActive() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Atualizar lista remota", func() { ui.refreshRight() }),
	}
	if !e.IsDir {
		items[0].Disabled = true
	}
	if e.Name == ".." {
		items[1].Disabled = true
	}
	if ui.copiedEntry == nil {
		items[3].Disabled = true
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
			if ui.sudoEnabled {
				if err := ui.copyHostFileWithSudoToLocal(ctx, e.Path, tmpPath); err != nil {
					fyne.Do(func() {
						dialog.ShowError(fmt.Errorf("não foi possível ler arquivo remoto com sudo: %w", err), ui.win)
					})
					return
				}
			} else {
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
			}
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
		if ui.sudoEnabled {
			return ui.copyLocalFileToHostWithSudo(ctx, s.tempPath, s.remotePath)
		}
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
