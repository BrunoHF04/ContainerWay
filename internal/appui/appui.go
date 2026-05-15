package appui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/hinshun/vt10x"
	"golang.org/x/crypto/ssh"

	"containerway/internal/containerfs"
	"containerway/internal/fsutil"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
	"containerway/internal/mailnotify"
	"containerway/internal/policy"
	"containerway/internal/session"
	"containerway/internal/tarxfer"
	"containerway/internal/transfer"
)

type transientSecret struct {
	Password string
	KeyPass  string
}

type accessUser struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

var (
	loginSecretMu         sync.Mutex
	loginSessionSecrets   = map[string]transientSecret{}
	auditLogMu            sync.Mutex
	auditActorMu          sync.Mutex
	auditActorName        = "desconhecido"
	accessUserMu          sync.Mutex
	currentAccessUserName = ""
	sessionAuditMu        sync.Mutex
	sessionAuditLines     []string
	// Evita reentrância: SetTheme dispara Settings.AddListener e trocar tema dentro do pop-up do Select no Windows pode encerrar o processo.
	themeApplyReentrant atomic.Bool
	// Último modo de tema aplicado (preferência); evita segundo SetTheme idêntico vindos do listener.
	lastAppliedThemePreference atomic.Value // string
)

const (
	themePreferenceKey                = "ui.theme.mode"
	leftFavoritesPreferenceKey        = "explorer.left.favorites"
	rightFavoritesPreferenceKey       = "explorer.right.favorites"
	operationHistoryPreferenceKey     = "explorer.operation.history"
	accessUsersPreferenceKey          = "access.users"
	notifyEnabledPreferenceKey        = "notify.email.enabled"
	notifyRecipientPreferenceKey      = "notify.email.to" // legado: um endereço; ainda sincronizado ao salvar
	notifyRecipientsJSONPreferenceKey = "notify.email.recipients"
	notifySMTPHostPreferenceKey       = "notify.smtp.host"
	notifySMTPPortPreferenceKey       = "notify.smtp.port"
	notifySMTPUserPreferenceKey       = "notify.smtp.user"
	notifySMTPPasswordPreferenceKey   = "notify.smtp.password"
	notifySMTPFromPreferenceKey       = "notify.smtp.from"
	terminalCommandFavoritesKey       = "terminal.commands.favorites"
	auditLogFileName                  = "containerway-activity.log"
	defaultAccessUser                 = "admin"
	defaultAccessPass                 = "!q1w2e3r4$"
	themeModeSystem                   = "system"
	themeModeLight                    = "light"
	themeModeDark                     = "dark"
	sessionEmailMaxLines              = 1500 // máx. de linhas do registro no e-mail de fim de sessão
	firstRunTipsPreferenceKey         = "access.firstRunTipsDismissed"
)

// Run inicia a aplicação Fyne.
func Run() {
	a := app.NewWithID("io.containerway.app")
	applyThemeMode(a, loadThemeMode(a))
	a.Settings().AddListener(func(_ fyne.Settings) {
		applyThemeMode(a, loadThemeMode(a))
	})
	if ico := appWindowIcon(); ico != nil {
		a.SetIcon(ico)
	}
	w := a.NewWindow("ContainerWay")
	if ico := appWindowIcon(); ico != nil {
		w.SetIcon(ico)
	}
	w.SetContent(buildAccessLogin(w))
	setAccessLoginWindow(w)
	w.ShowAndRun()
}

// setAccessLoginWindow executa parte da logica deste modulo.
func setAccessLoginWindow(w fyne.Window) {
	w.Resize(fyne.NewSize(480, 360))
	w.CenterOnScreen()
}

// setLoginWindow executa parte da logica deste modulo.
func setLoginWindow(w fyne.Window) {
	// Tamanho compatível com abas e formulário; evitar ficar menor que o MinSize do conteúdo.
	w.Resize(fyne.NewSize(520, 640))
	w.CenterOnScreen()
}

// setExplorerWindow executa parte da logica deste modulo.
func setExplorerWindow(w fyne.Window) {
	switch runtime.GOOS {
	case "windows", "darwin":
		// Evitar Resize antes da maximização: no Windows isso deixa a janela em estado
		// restaurado e, após SetContent, a maximização nativa costuma não permanecer.
		maximizeMainWindow(w)
	default:
		w.Resize(fyne.NewSize(1100, 720))
		w.CenterOnScreen()
	}
}

// setSessionHubWindow executa parte da logica deste modulo.
func setSessionHubWindow(w fyne.Window) {
	// Tela inicial da sessão: compacta e centralizada. Se a janela vinha maximizada (explorador),
	// é preciso restaurar antes do Resize — senão o Windows mantém estado maximizado e o layout fica inconsistente.
	const hubW, hubH float32 = 820, 460
	restoreNormalMainWindow(w)
	w.Resize(fyne.NewSize(hubW, hubH))
	w.CenterOnScreen()
	fyne.Do(func() {
		w.Resize(fyne.NewSize(hubW, hubH))
		w.CenterOnScreen()
	})
}

// goToLogin executa parte da logica deste modulo.
func goToLogin(w fyne.Window) {
	w.SetCloseIntercept(nil)
	// Volta do explorador/hub maximizado: restaurar antes de trocar conteúdo e redimensionar (igual tela Início).
	restoreNormalMainWindow(w)
	w.SetContent(buildAccessLogin(w))
	setAccessLoginWindow(w)
	fyne.Do(func() {
		w.Resize(fyne.NewSize(480, 360))
		w.CenterOnScreen()
	})
}

// buildAccessLogin executa parte da logica deste modulo.
func buildAccessLogin(w fyne.Window) fyne.CanvasObject {
	username := widget.NewEntry()
	username.SetPlaceHolder(tr("acc_ph_user"))
	password := widget.NewPasswordEntry()
	password.SetPlaceHolder(tr("acc_ph_pass"))
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	accounts := loadAccessAccounts()

	tryLogin := func() {
		u := normalizeAccessUsername(username.Text)
		p := strings.TrimSpace(password.Text)
		if u == "" || p == "" {
			status.SetText(tr("acc_need_both"))
			return
		}
		acc, ok := findAccessAccount(accounts, u)
		if ok && strings.TrimSpace(acc.Password) == p {
			resetSessionAuditBuffer()
			setAuditActor(acc.DisplayName)
			setCurrentAccessUser(acc.Username)
			appendAuditLog("acesso", "Login de acesso autorizado")
			if cfg := loadMailNotifySettings(); cfg.Valid() {
				go sendNotifyLoginEmailWithConfig(copyMailNotifySettingsForAsync(cfg), acc)
			}
			w.SetContent(buildLogin(w))
			setLoginWindow(w)
			maybeShowFirstRunTips(w)
			// Após SetContent o layout pode alterar o tamanho real da janela; recentrar no próximo ciclo.
			fyne.Do(func() { w.CenterOnScreen() })
			return
		}
		appendAuditLog("acesso", "Tentativa de login de acesso inválida")
		status.SetText(tr("acc_bad_creds"))
	}

	enterBtn := widget.NewButtonWithIcon(tr("acc_btn_enter"), theme.LoginIcon(), tryLogin)
	enterBtn.Importance = widget.HighImportance
	password.OnSubmitted = func(string) { tryLogin() }

	content := fynecontainer.NewVBox(
		widget.NewLabelWithStyle(tr("acc_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel(tr("acc_desc")),
		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem(tr("acc_form_user"), username),
			widget.NewFormItem(tr("acc_form_pass"), password),
		),
		status,
		enterBtn,
	)
	card := widget.NewCard(tr("app_name"), tr("acc_card_sub"), content)
	return fynecontainer.NewCenter(card)
}

// buildLogin executa parte da logica deste modulo.
func buildLogin(w fyne.Window) fyne.CanvasObject {
	host := widget.NewEntry()
	host.SetPlaceHolder(tr("conn_ph_host"))
	user := widget.NewEntry()
	user.SetPlaceHolder(tr("conn_ph_user"))
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder(tr("conn_ph_pass"))
	keyPath := widget.NewEntry()
	keyPath.SetPlaceHolder(tr("conn_ph_key"))
	keyPass := widget.NewPasswordEntry()
	keyPass.SetPlaceHolder(tr("conn_ph_keypass"))
	knownHosts := widget.NewEntry()
	knownHosts.SetPlaceHolder(tr("conn_ph_knownhosts"))
	themeSelect := widget.NewSelect([]string{tr("theme_system"), tr("theme_light"), tr("theme_dark")}, nil)
	themeSelect.SetSelected(themeLabelForMode(loadThemeMode(fyne.CurrentApp())))
	themeSelect.OnChanged = func(selected string) {
		mode := themeModeFromLabel(selected)
		app := fyne.CurrentApp()
		app.Preferences().SetString(themePreferenceKey, mode)
		fyne.Do(func() {
			applyThemeMode(app, mode)
		})
	}
	insecureHost := widget.NewCheck(tr("chk_insecure"), nil)
	insecureHost.SetChecked(true)
	if policy.ForbidInsecureHostKey() {
		insecureHost.SetChecked(false)
		insecureHost.Disable()
	}
	dockerSocketEntry := widget.NewEntry()
	dockerSocketEntry.SetPlaceHolder(tr("conn_ph_docker"))
	parallelJobsEntry := widget.NewEntry()
	parallelJobsEntry.SetText("3")
	parallelJobsEntry.SetPlaceHolder(tr("conn_ph_parallel"))
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	saveSecrets := widget.NewCheck(tr("save_secrets"), nil)
	rememberSession := widget.NewCheck(tr("remember_sess"), nil)
	connName := widget.NewEntry()
	connName.SetPlaceHolder(tr("conn_ph_name"))
	profileSelect := widget.NewSelect([]string{tr("profile_new")}, nil)
	profileSelect.SetSelected(tr("profile_new"))

	profiles, loadErr := loadSavedConnections()
	if loadErr != nil {
		status.SetText(loadErr.Error())
	}

	rebuildProfileOptions := func(selected string) {
		opts := []string{tr("profile_new")}
		for _, p := range profiles {
			opts = append(opts, p.Name)
		}
		profileSelect.Options = opts
		profileSelect.Refresh()
		if selected != "" {
			profileSelect.SetSelected(selected)
		} else {
			profileSelect.SetSelected(tr("profile_new"))
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
		dockerSocketEntry.SetText(c.DockerSocket)
		insecureHost.SetChecked(c.InsecureHostKey)
		if policy.ForbidInsecureHostKey() {
			insecureHost.SetChecked(false)
		}
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
		dockerSocketEntry.SetText("")
		insecureHost.SetChecked(true)
		if policy.ForbidInsecureHostKey() {
			insecureHost.SetChecked(false)
		}
		parallelJobsEntry.SetText("3")
		saveSecrets.SetChecked(false)
		rememberSession.SetChecked(false)
		status.SetText("")
	}

	profileSelect.OnChanged = func(sel string) {
		if sel == tr("profile_new") {
			clearProfileInputs()
			appendAuditLog("login", "Formulário de nova conexão selecionado")
			return
		}
		c, ok := findConnectionByName(profiles, sel)
		if !ok {
			status.SetText(tr("conn_not_found"))
			return
		}
		applyProfile(c)
		status.SetText(fmt.Sprintf(tr("conn_loaded_fmt"), c.Name))
		appendAuditLog("login", "Conexão carregada: "+c.Name)
	}

	saveProfile := widget.NewButtonWithIcon(tr("btn_save"), theme.DocumentSaveIcon(), func() {
		name := strings.TrimSpace(connName.Text)
		if name == "" {
			dialog.ShowInformation(tr("app_name"), tr("dlg_conn_need_name"), w)
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
			DockerSocket:    strings.TrimSpace(dockerSocketEntry.Text),
		}
		if policy.ForbidInsecureHostKey() {
			saved.InsecureHostKey = false
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
		status.SetText(fmt.Sprintf(tr("conn_saved_fmt"), name))
		appendAuditLog("login", "Conexão salva: "+name)
	})

	deleteProfile := widget.NewButtonWithIcon(tr("btn_delete"), theme.DeleteIcon(), func() {
		target := strings.TrimSpace(profileSelect.Selected)
		if target == "" || target == tr("profile_new") {
			dialog.ShowInformation(tr("app_name"), tr("dlg_conn_pick_delete"), w)
			return
		}
		dialog.ShowConfirm(tr("dlg_conn_delete_title"), fmt.Sprintf(tr("dlg_conn_delete_fmt"), target), func(ok bool) {
			if !ok {
				return
			}
			profiles = removeConnectionByName(profiles, target)
			if err := saveConnections(profiles); err != nil {
				dialog.ShowError(err, w)
				return
			}
			rebuildProfileOptions("")
			status.SetText(fmt.Sprintf(tr("conn_deleted_fmt"), target))
			appendAuditLog("login", "Conexão excluída: "+target)
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
			msg = tr("val_host")
			canConnect = false
		case userVal == "":
			msg = tr("val_user")
			canConnect = false
		case strings.TrimSpace(pass.Text) == "" && keyVal == "":
			msg = tr("val_auth")
			canConnect = false
		default:
			v, err := strconv.Atoi(parVal)
			if err != nil || v < 1 || v > 16 {
				msg = tr("val_parallel")
				canConnect = false
			} else if keyVal != "" {
				if _, err := os.Stat(keyVal); err != nil {
					msg = tr("val_key_missing")
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
			{Text: tr("form_host"), Widget: host},
			{Text: tr("form_user"), Widget: user},
			{Text: tr("form_pass"), Widget: pass},
		},
	}
	formAdv := &widget.Form{
		Items: []*widget.FormItem{
			{Text: tr("form_key"), Widget: keyPath},
			{Text: tr("form_keypass"), Widget: keyPass},
			{Text: tr("form_knownhosts"), Widget: knownHosts},
			{Text: "", Widget: insecureHost},
			{Text: tr("form_docker"), Widget: dockerSocketEntry},
			{Text: tr("form_parallel"), Widget: parallelJobsEntry},
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
		fynecontainer.NewTabItem(tr("tab_conn"), fynecontainer.NewVBox(
			savedConnRow,
			connName,
			saveSecrets,
			widget.NewSeparator(),
			formConn,
		)),
		fynecontainer.NewTabItem(tr("tab_keysec"), formAdv),
	)
	tabs.SetTabLocation(fynecontainer.TabLocationTop)
	themeRow := fynecontainer.NewBorder(nil, nil, widget.NewLabel(tr("conn_theme_label")), nil, themeSelect)

	connect = widget.NewButtonWithIcon(tr("btn_connect"), theme.LoginIcon(), func() {
		status.SetText(tr("st_connecting"))
		appendAuditLog("login", "Tentativa de conexão para "+strings.TrimSpace(host.Text))
		if policy.ForbidInsecureHostKey() && insecureHost.Checked {
			status.SetText(tr("st_policy_hostkey"))
			dialog.ShowInformation(tr("dlg_policy_title"), tr("dlg_policy_insecure_body"), w)
			return
		}
		creds := session.Credentials{
			Host:              host.Text,
			User:              user.Text,
			Password:          pass.Text,
			KeyPath:           strings.TrimSpace(keyPath.Text),
			KeyPass:           keyPass.Text,
			KnownHostsFiles:   splitKnownHostsFiles(knownHosts.Text),
			InsecureHostKey:   insecureHost.Checked && !policy.ForbidInsecureHostKey(),
			DockerUnixSocket:  strings.TrimSpace(dockerSocketEntry.Text),
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
				appendAuditLog("login", "Falha ao conectar: "+err.Error())
				return
			}
			fyne.Do(func() {
				w.SetContent(buildExplorer(w, sess, pJobs, creds))
				setSessionHubWindow(w)
			})
			appendAuditLog("login", "Conexão estabelecida com sucesso para "+strings.TrimSpace(host.Text))
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
	testConn = widget.NewButtonWithIcon(tr("btn_test"), theme.ConfirmIcon(), func() {
		status.SetText(tr("st_testing"))
		appendAuditLog("login", "Teste de conexão iniciado para "+strings.TrimSpace(host.Text))
		if policy.ForbidInsecureHostKey() && insecureHost.Checked {
			status.SetText(tr("st_policy_hostkey"))
			dialog.ShowInformation(tr("dlg_policy_title"), tr("dlg_policy_insecure_body"), w)
			return
		}
		creds := session.Credentials{
			Host:              host.Text,
			User:              user.Text,
			Password:          pass.Text,
			KeyPath:           strings.TrimSpace(keyPath.Text),
			KeyPass:           keyPass.Text,
			KnownHostsFiles:   splitKnownHostsFiles(knownHosts.Text),
			InsecureHostKey:   insecureHost.Checked && !policy.ForbidInsecureHostKey(),
			DockerUnixSocket:  strings.TrimSpace(dockerSocketEntry.Text),
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
				appendAuditLog("login", "Teste de conexão com falha: "+err.Error())
				return
			}
			sess.Close()
			fyne.Do(func() {
				status.SetText(tr("st_test_ok"))
			})
			appendAuditLog("login", "Teste de conexão concluído com sucesso para "+strings.TrimSpace(host.Text))
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
		tr("app_name"),
		tr("login_card_subtitle"),
		cardInner,
	)

	return fynecontainer.NewCenter(card)
}

// loadThemeMode executa parte da logica deste modulo.
func loadThemeMode(a fyne.App) string {
	mode := strings.TrimSpace(a.Preferences().StringWithFallback(themePreferenceKey, themeModeSystem))
	switch mode {
	case themeModeSystem, themeModeLight, themeModeDark:
		return mode
	default:
		return themeModeSystem
	}
}

// applyThemeMode executa parte da logica deste modulo.
func applyThemeMode(a fyne.App, mode string) {
	if !themeApplyReentrant.CompareAndSwap(false, true) {
		return
	}
	defer themeApplyReentrant.Store(false)

	// O listener de Settings volta a chamar isto após SetTheme; para Claro/Escuro isso repetia
	// um tema novo (LightTheme aloca sempre) e forçava reflow desnecessário.
	if prev, ok := lastAppliedThemePreference.Load().(string); ok && prev == mode &&
		(mode == themeModeLight || mode == themeModeDark) {
		return
	}

	switch mode {
	case themeModeLight:
		a.Settings().SetTheme(forcedLightTheme())
	case themeModeDark:
		a.Settings().SetTheme(newModernTheme())
	default:
		a.Settings().SetTheme(theme.DefaultTheme())
	}
	lastAppliedThemePreference.Store(mode)
}

// themeLabelForMode executa parte da logica deste modulo.
func themeLabelForMode(mode string) string {
	switch mode {
	case themeModeLight:
		return tr("theme_light")
	case themeModeDark:
		return tr("theme_dark")
	default:
		return tr("theme_system")
	}
}

// themeModeFromLabel executa parte da logica deste modulo.
func themeModeFromLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == tr("theme_light") {
		return themeModeLight
	}
	if label == tr("theme_dark") {
		return themeModeDark
	}
	if label == tr("theme_system") {
		return themeModeSystem
	}
	switch label {
	case "Claro", "Light":
		return themeModeLight
	case "Escuro", "Dark", "Oscuro":
		return themeModeDark
	default:
		return themeModeSystem
	}
}

type explorer struct {
	win       fyne.Window
	s         *session.Session
	hfs       *hostfs.FS
	cfs       *containerfs.FS
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

	leftList        *widget.List
	rightList       *widget.List
	leftPathLbl     *widget.Label
	breadcrumb      *widget.Label
	leftCrumbs      *fyne.Container
	rightCrumbs     *fyne.Container
	ctxSelect       *widget.Select
	leftQuick       *widget.Select
	rightQuick      *widget.Select
	status          *widget.Label
	progress        *widget.ProgressBar
	lastJobText     *widget.Label
	leftFooterInfo  *widget.Label
	rightFooterInfo *widget.Label
	leftSearch      *widget.Entry
	rightSearch     *widget.Entry
	leftTypeFilter  *widget.Select
	rightTypeFilter *widget.Select
	leftBack        []string
	rightBack       []string

	tm             *transfer.Manager
	parallelJobs   int
	activePane     string
	btnOpenLocal   *widget.Button
	btnOpenRemote  *widget.Button
	btnUp          *widget.Button
	btnDown        *widget.Button
	btnLeftSend    *widget.Button
	btnRightRecv   *widget.Button
	lblSudoState   *widget.Label
	btnDisableSudo *widget.Button

	btnBackToHub     *widget.Button
	btnHistory       *widget.Button
	btnCompare       *widget.Button
	btnDisconnect    *widget.Button
	btnLeftSendBatch *widget.Button
	btnRightRecvBatch *widget.Button
	lblPaneLocal     *widget.Label
	lblPaneRemote    *widget.Label
	hintAddLeft      *hintIconButton
	hintRemoveLeft   *hintIconButton
	hintAddRight     *hintIconButton
	hintRemoveRight  *hintIconButton

	// Evita aplicar listagens antigas se o usuário mudar de pasta/contexto a meio.
	rightRefreshSeq atomic.Uint64

	remoteEditMu         sync.Mutex
	remoteEditSessions   map[string]*remoteEditSession
	rootPromptOpen       atomic.Bool
	sudoEnabled          bool
	sudoUser             string
	sudoPass             string
	sudoValidatedAt      time.Time
	sudoTTL              time.Duration
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
	opHistory            []string
	failedJobs           []transfer.Job

	explorerMain  fyne.CanvasObject
	sessionHub    fyne.CanvasObject
	explorerOnTop atomic.Bool // true quando a janela mostra o gerenciador (atalhos do explorador ativos)

	automationMu            sync.Mutex
	automationRules         []automationRule
	automationHistory       []string
	automationEngineMu      sync.Mutex
	automationEngineStop    chan struct{}
	automationEngineRunning atomic.Bool

	// Destino do botão Voltar nas telas cheias de configuração (admin): explorador ou hub da sessão.
	settingsReturnToExplorer bool
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

type hintIconButton struct {
	widget.Button
	hint    string
	status  *widget.Label
	prevMsg string
	hover   bool
}

func (b *hintIconButton) setHintText(s string) {
	if b == nil {
		return
	}
	b.hint = strings.TrimSpace(s)
}

type terminalEntry struct {
	widget.Entry
	onTab   func()
	onCtrlC func()
	onF10   func()
}

func newTerminalEntry() *terminalEntry {
	t := &terminalEntry{}
	t.MultiLine = true
	t.Wrapping = fyne.TextWrapOff
	t.ExtendBaseWidget(t)
	return t
}

func (t *terminalEntry) TypedKey(k *fyne.KeyEvent) {
	if k != nil {
		switch k.Name {
		case fyne.KeyTab:
			if t.onTab != nil {
				t.onTab()
			}
			return
		case fyne.KeyF10:
			if t.onF10 != nil {
				t.onF10()
			}
			return
		}
	}
	t.Entry.TypedKey(k)
}

func (t *terminalEntry) TypedShortcut(s fyne.Shortcut) {
	switch s.(type) {
	case *fyne.ShortcutCopy:
		if t.onCtrlC != nil {
			t.onCtrlC()
			return
		}
	}
	t.Entry.TypedShortcut(s)
}

type terminalCellStyle struct {
	textStyle fyne.TextStyle
	fg        color.Color
	bg        color.Color
}

func (s terminalCellStyle) Style() fyne.TextStyle        { return s.textStyle }
func (s terminalCellStyle) TextColor() color.Color       { return s.fg }
func (s terminalCellStyle) BackgroundColor() color.Color { return s.bg }

func xtermColorToRGBA(c vt10x.Color, isFG bool) color.Color {
	if isFG && c == vt10x.DefaultFG {
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255}
	}
	if !isFG && c == vt10x.DefaultBG {
		return color.NRGBA{R: 11, G: 17, B: 32, A: 255}
	}
	n := int(c)
	base16 := []color.NRGBA{
		{0, 0, 0, 255}, {205, 49, 49, 255}, {13, 188, 121, 255}, {229, 229, 16, 255},
		{36, 114, 200, 255}, {188, 63, 188, 255}, {17, 168, 205, 255}, {229, 229, 229, 255},
		{102, 102, 102, 255}, {241, 76, 76, 255}, {35, 209, 139, 255}, {245, 245, 67, 255},
		{59, 142, 234, 255}, {214, 112, 214, 255}, {41, 184, 219, 255}, {255, 255, 255, 255},
	}
	if n >= 0 && n < len(base16) {
		return base16[n]
	}
	if n >= 16 && n <= 231 {
		idx := n - 16
		r := idx / 36
		g := (idx % 36) / 6
		b := idx % 6
		scale := []uint8{0, 95, 135, 175, 215, 255}
		return color.NRGBA{R: scale[r], G: scale[g], B: scale[b], A: 255}
	}
	if n >= 232 && n <= 255 {
		v := uint8(8 + (n-232)*10)
		return color.NRGBA{R: v, G: v, B: v, A: 255}
	}
	if isFG {
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255}
	}
	return color.NRGBA{R: 11, G: 17, B: 32, A: 255}
}

func renderVTToTextGrid(vt vt10x.Terminal, grid *widget.TextGrid) {
	grid.SetText(vt.String())
	grid.Refresh()
	grid.ScrollToBottom()
}

func renderVTToTextGridANSI(vt vt10x.Terminal, grid *widget.TextGrid, drawCursor bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("render vt: %v", r)
		}
	}()
	vt.Lock()
	defer vt.Unlock()
	cols, rows := vt.Size()
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("tamanho VT inválido: %dx%d", cols, rows)
	}
	renderRows := make([]widget.TextGridRow, rows)
	cursor := vt.Cursor()
	cursorVisible := drawCursor && vt.CursorVisible()
	for y := 0; y < rows; y++ {
		cells := make([]widget.TextGridCell, cols)
		for x := 0; x < cols; x++ {
			g := vt.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			fg := xtermColorToRGBA(g.FG, true)
			bg := xtermColorToRGBA(g.BG, false)
			if cursorVisible && x == cursor.X && y == cursor.Y {
				// Cursor piscante por inversão de cores para destacar posição de digitação.
				fg, bg = bg, fg
			}
			cells[x] = widget.TextGridCell{
				Rune: ch,
				Style: terminalCellStyle{
					textStyle: fyne.TextStyle{Monospace: true},
					fg:        fg,
					bg:        bg,
				},
			}
		}
		renderRows[y] = widget.TextGridRow{Cells: cells}
	}
	grid.Rows = renderRows
	grid.Refresh()
	return nil
}

var (
	terminalStarOutlineIcon = fyne.NewStaticResource("terminal-star-outline.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#c9cdd4" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20.2l1.1-6.2L3 9.6l6.2-.9L12 3z"/></svg>`))
	terminalStarFilledIcon  = fyne.NewStaticResource("terminal-star-filled.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#f2c94c"><path d="m12 2.6 2.9 5.8 6.4.9-4.6 4.5 1.1 6.4L12 17.2 6.2 20.2l1.1-6.4L2.7 9.3l6.4-.9L12 2.6z"/></svg>`))
)

type remoteTerminalHostStats struct {
	osName      string
	kernel      string
	user        string
	bootTime    string
	uptimeShort string
	memUsedB    uint64
	memTotalB   uint64
	diskUsedB   uint64
	diskTotalB  uint64
}

// formatBytesIEC executa parte da logica deste modulo.
func formatBytesIEC(v uint64) string {
	if v == 0 {
		return "0 B"
	}
	const unit = 1024.0
	f := float64(v)
	switch {
	case f >= unit*unit*unit*unit:
		return fmt.Sprintf("%.2f TiB", f/(unit*unit*unit*unit))
	case f >= unit*unit*unit:
		return fmt.Sprintf("%.2f GiB", f/(unit*unit*unit))
	case f >= unit*unit:
		return fmt.Sprintf("%.2f MiB", f/(unit*unit))
	case f >= unit:
		return fmt.Sprintf("%.2f KiB", f/unit)
	default:
		return fmt.Sprintf("%d B", v)
	}
}

// parseUint executa parte da logica deste modulo.
func parseUint(s string) uint64 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseMapLine executa parte da logica deste modulo.
func parseMapLine(s string) (string, string, bool) {
	idx := strings.IndexByte(s, '=')
	if idx <= 0 || idx >= len(s)-1 {
		return "", "", false
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:]), true
}

// formatUptime executa parte da logica deste modulo.
func formatUptime(seconds uint64) string {
	if seconds == 0 {
		return "n/d"
	}
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// buildTerminalHostInfoText executa parte da logica deste modulo.
func buildTerminalHostInfoText(stats remoteTerminalHostStats) string {
	ram := "n/d"
	if stats.memTotalB > 0 {
		ram = fmt.Sprintf("%s / %s", formatBytesIEC(stats.memUsedB), formatBytesIEC(stats.memTotalB))
	}
	disk := "n/d"
	if stats.diskTotalB > 0 {
		disk = fmt.Sprintf("%s / %s", formatBytesIEC(stats.diskUsedB), formatBytesIEC(stats.diskTotalB))
	}
	osName := strings.TrimSpace(stats.osName)
	if osName == "" {
		osName = "Linux"
	}
	kernel := strings.TrimSpace(stats.kernel)
	if kernel == "" {
		kernel = "kernel n/d"
	}
	user := strings.TrimSpace(stats.user)
	if user == "" {
		user = "n/d"
	}
	boot := strings.TrimSpace(stats.bootTime)
	if boot == "" {
		boot = "n/d"
	}
	uptime := strings.TrimSpace(stats.uptimeShort)
	if uptime == "" {
		uptime = "n/d"
	}
	return fmt.Sprintf(tr("ui_term_host_summary_fmt"),
		osName, kernel, ram, disk, uptime, boot, user)
}

// fetchRemoteTerminalHostStats executa parte da logica deste modulo.
func (ui *explorer) fetchRemoteTerminalHostStats(ctx context.Context) (remoteTerminalHostStats, error) {
	script := `
OS_NAME=""
if [ -r /etc/os-release ]; then
  . /etc/os-release
  OS_NAME="${PRETTY_NAME:-$NAME}"
fi
if [ -z "$OS_NAME" ]; then
  OS_NAME="$(uname -o 2>/dev/null || echo Linux)"
fi
KERNEL="$(uname -sr 2>/dev/null || uname -a 2>/dev/null || echo '')"
USER_NAME="$(id -un 2>/dev/null || whoami 2>/dev/null || echo '')"
MEM_TOTAL_KB="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo 2>/dev/null | head -n1)"
MEM_AVAIL_KB="$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo 2>/dev/null | head -n1)"
if [ -z "$MEM_AVAIL_KB" ]; then
  MEM_AVAIL_KB="$(awk '/^MemFree:/ {print $2}' /proc/meminfo 2>/dev/null | head -n1)"
fi
set -- $(df -B1 / 2>/dev/null | awk 'NR==2 {print $2, $3}')
DISK_TOTAL_B="${1:-0}"
DISK_USED_B="${2:-0}"
UPTIME_SEC="$(cut -d. -f1 /proc/uptime 2>/dev/null || echo 0)"
BOOT_TIME="$(uptime -s 2>/dev/null || who -b 2>/dev/null | awk '{print $3" "$4}')"
echo "OS_NAME=$OS_NAME"
echo "KERNEL=$KERNEL"
echo "USER_NAME=$USER_NAME"
echo "MEM_TOTAL_KB=$MEM_TOTAL_KB"
echo "MEM_AVAIL_KB=$MEM_AVAIL_KB"
echo "DISK_TOTAL_B=$DISK_TOTAL_B"
echo "DISK_USED_B=$DISK_USED_B"
echo "UPTIME_SEC=$UPTIME_SEC"
echo "BOOT_TIME=$BOOT_TIME"
`
	stdout, _, err := ui.runSSHCommandWithInput(ctx, "sh -lc "+shellQuote(script), "")
	if err != nil {
		return remoteTerminalHostStats{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := parseMapLine(line)
		if !ok {
			continue
		}
		values[k] = v
	}
	memTotalB := parseUint(values["MEM_TOTAL_KB"]) * 1024
	memAvailB := parseUint(values["MEM_AVAIL_KB"]) * 1024
	memUsedB := uint64(0)
	if memTotalB > memAvailB {
		memUsedB = memTotalB - memAvailB
	}
	stats := remoteTerminalHostStats{
		osName:      values["OS_NAME"],
		kernel:      values["KERNEL"],
		user:        values["USER_NAME"],
		bootTime:    values["BOOT_TIME"],
		uptimeShort: formatUptime(parseUint(values["UPTIME_SEC"])),
		memUsedB:    memUsedB,
		memTotalB:   memTotalB,
		diskUsedB:   parseUint(values["DISK_USED_B"]),
		diskTotalB:  parseUint(values["DISK_TOTAL_B"]),
	}
	return stats, nil
}

func (ui *explorer) showTerminalConsoleVT(currentDir, host string) error {
	const (
		termCols = 160
		termRows = 48
	)

	vt := vt10x.New(vt10x.WithSize(termCols, termRows))
	if vt == nil {
		return errors.New("vt indisponível")
	}
	grid := widget.NewTextGrid()
	grid.ShowLineNumbers = false
	grid.ShowWhitespace = false
	if err := renderVTToTextGridANSI(vt, grid, true); err != nil {
		return err
	}
	terminalViewport := fynecontainer.NewPadded(fynecontainer.NewMax(grid))

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	statusRow := fynecontainer.NewVBox(status)
	statusRow.Hide()
	hostOSValue := widget.NewLabel(tr("ui_host_collecting"))
	hostResValue := widget.NewLabel(tr("ui_host_collecting"))
	hostTimeValue := widget.NewLabel(tr("ui_host_collecting"))
	hostUserValue := widget.NewLabel(tr("ui_host_collecting"))
	hostCompact := widget.NewLabel(tr("ui_host_collecting_info"))
	hostCompact.Wrapping = fyne.TextWrapOff
	clearBtn := widget.NewButton(tr("ui_term_clear"), nil)
	clearBtn.Importance = widget.MediumImportance
	ctrlCBtn := widget.NewButton(tr("ex_back"), nil)
	ctrlCBtn.Importance = widget.MediumImportance
	btnCopyOutput := widget.NewButton(tr("ui_term_copy_output"), nil)
	btnCopyOutput.Importance = widget.MediumImportance
	var footer *fyne.Container
	var body *fyne.Container
	showStatus := func(msg string) {
		fyne.Do(func() {
			msg = strings.TrimSpace(msg)
			status.SetText(msg)
			if msg == "" {
				statusRow.Hide()
			} else {
				statusRow.Show()
			}
			if footer != nil {
				footer.Refresh()
			}
			if body != nil {
				body.Refresh()
			}
		})
	}
	clearStatusAfter := func(expected string, delay time.Duration) {
		go func() {
			time.Sleep(delay)
			fyne.Do(func() {
				if strings.TrimSpace(status.Text) == strings.TrimSpace(expected) {
					status.SetText("")
					statusRow.Hide()
					if footer != nil {
						footer.Refresh()
					}
					if body != nil {
						body.Refresh()
					}
				}
			})
		}()
	}

	sess, err := ui.s.SSH.NewSession()
	if err != nil {
		return err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		return err
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", termRows, termCols, modes); err != nil {
		_ = sess.Close()
		return err
	}
	startCmd := shellStartWithTerminalBanner(currentDir, host)
	if err := sess.Start(startCmd); err != nil {
		_ = sess.Close()
		return err
	}

	var (
		closed      atomic.Bool
		vtMu        sync.Mutex
		renderDirty atomic.Bool
		cursorBlink atomic.Bool
		stopRender  = make(chan struct{})
		stopOnce    sync.Once
		lastCanvas  fyne.Size
		logMu       sync.Mutex
		terminalLog []byte
	)
	cursorBlink.Store(true)
	closeTerminal := func() {
		if closed.Swap(true) {
			return
		}
		stopOnce.Do(func() { close(stopRender) })
		ui.win.Canvas().SetOnTypedRune(nil)
		ui.win.Canvas().SetOnTypedKey(nil)
		_, _ = io.WriteString(stdin, "exit\n")
		_ = stdin.Close()
		_ = sess.Close()
	}

	appendFromRemote := func(chunk []byte) {
		clean := normalizeTerminalChunk(string(bytes.ToValidUTF8(chunk, []byte{})))
		if clean != "" {
			logMu.Lock()
			const maxTerminalLogBytes = 2 * 1024 * 1024
			if overflow := len(terminalLog) + len(clean) - maxTerminalLogBytes; overflow > 0 {
				if overflow >= len(terminalLog) {
					terminalLog = terminalLog[:0]
				} else {
					terminalLog = append([]byte{}, terminalLog[overflow:]...)
				}
			}
			terminalLog = append(terminalLog, clean...)
			logMu.Unlock()
		}
		vtMu.Lock()
		_, _ = vt.Write(chunk)
		vtMu.Unlock()
		renderDirty.Store(true)
	}

	getTerminalLog := func() string {
		logMu.Lock()
		defer logMu.Unlock()
		return strings.TrimSpace(string(terminalLog))
	}

	go func() {
		ticker := time.NewTicker(33 * time.Millisecond)
		cursorTicker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		defer cursorTicker.Stop()
		for {
			select {
			case <-stopRender:
				return
			case <-cursorTicker.C:
				if closed.Load() {
					continue
				}
				cursorBlink.Store(!cursorBlink.Load())
				renderDirty.Store(true)
			case <-ticker.C:
				if closed.Load() || !renderDirty.Swap(false) {
					continue
				}
				fyne.Do(func() {
					vtMu.Lock()
					rerr := renderVTToTextGridANSI(vt, grid, cursorBlink.Load())
					vtMu.Unlock()
					if rerr != nil {
						showStatus(tr("ui_term_ansi_unstable"))
					}
				})
			}
		}
	}()
	calcTermSize := func(sz fyne.Size) (cols, rows int) {
		// Usa o tamanho real do viewport do terminal para evitar cortes do topo.
		cols = int(sz.Width / 8.6)
		rows = int(sz.Height / 17.0)
		if cols < 60 {
			cols = 60
		}
		if rows < 16 {
			rows = 16
		}
		return cols, rows
	}
	applyResize := func(sz fyne.Size) {
		cols, rows := calcTermSize(sz)
		vtMu.Lock()
		vt.Resize(cols, rows)
		vtMu.Unlock()
		_ = sess.WindowChange(rows, cols)
		renderDirty.Store(true)
	}
	go func() {
		ticker := time.NewTicker(350 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopRender:
				return
			case <-ticker.C:
				if closed.Load() {
					return
				}
				cur := terminalViewport.Size()
				if cur.Width <= 0 || cur.Height <= 0 {
					continue
				}
				if cur == lastCanvas {
					continue
				}
				lastCanvas = cur
				applyResize(cur)
			}
		}
	}()

	streamLoop := func(r io.Reader) {
		buf := make([]byte, 4096)
		for {
			n, rerr := r.Read(buf)
			if n > 0 {
				appendFromRemote(buf[:n])
			}
			if rerr != nil {
				return
			}
		}
	}
	go streamLoop(stdout)
	go streamLoop(stderr)
	updateHostInfo := func(unavailableMsg string) {
		if closed.Load() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		stats, ferr := ui.fetchRemoteTerminalHostStats(ctx)
		if ferr != nil {
			if unavailableMsg == "" {
				unavailableMsg = tr("ui_host_unavailable")
			}
			showStatus(unavailableMsg)
			return
		}
		fyne.Do(func() {
			hostOSValue.SetText(strings.TrimSpace(stats.osName) + " | " + strings.TrimSpace(stats.kernel))
			hostResValue.SetText(fmt.Sprintf(tr("ui_term_host_ram_disk_fmt"),
				formatBytesIEC(stats.memUsedB), formatBytesIEC(stats.memTotalB),
				formatBytesIEC(stats.diskUsedB), formatBytesIEC(stats.diskTotalB)))
			hostTimeValue.SetText(fmt.Sprintf(tr("ui_term_host_uptime_boot_fmt"),
				strings.TrimSpace(stats.uptimeShort), strings.TrimSpace(stats.bootTime)))
			hostUserValue.SetText(fmt.Sprintf(tr("ui_host_user_fmt"), strings.TrimSpace(stats.user)))
			hostCompact.SetText(fmt.Sprintf(tr("ui_term_host_compact_fmt"),
				strings.TrimSpace(stats.osName),
				formatBytesIEC(stats.memUsedB), formatBytesIEC(stats.memTotalB),
				formatBytesIEC(stats.diskUsedB), formatBytesIEC(stats.diskTotalB),
				strings.TrimSpace(stats.uptimeShort),
				strings.TrimSpace(stats.user),
			))
		})
	}
	go func() {
		updateHostInfo(tr("ui_host_load_fail"))
		ticker := time.NewTicker(12 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopRender:
				return
			case <-ticker.C:
				updateHostInfo("")
			}
		}
	}()
	go func() {
		waitErr := sess.Wait()
		fyne.Do(func() {
			if waitErr != nil && !closed.Load() {
				showStatus(fmt.Sprintf(tr("ui_term_session_closed_fmt"), strings.TrimSpace(waitErr.Error())))
			} else if !closed.Load() {
				showStatus(tr("ui_term_session_closed"))
			}
			shouldAutoBack := !closed.Load()
			if shouldAutoBack {
				closeTerminal()
				ui.closeSettingsFullscreen()
				return
			}
			ui.win.Canvas().SetOnTypedRune(nil)
			ui.win.Canvas().SetOnTypedKey(nil)
			ctrlCBtn.Disable()
			clearBtn.Disable()
		})
	}()

	sendKey := func(seq string) {
		if closed.Load() || seq == "" {
			return
		}
		if _, werr := io.WriteString(stdin, seq); werr != nil {
			showStatus(tr("ui_term_send_key_fail"))
		}
	}
	runTerminalCommand := func(cmd string) {
		if strings.TrimSpace(cmd) == "" {
			return
		}
		sendKey(cmd + "\r")
	}

	ctrlCBtn.OnTapped = func() { sendKey("\u0003") }
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyC,
		Modifier: fyne.KeyModifierControl,
	}, func(fyne.Shortcut) {
		if closed.Load() {
			return
		}
		sendKey("\u0003")
		showStatus(tr("ui_term_ctrl_c_keyboard"))
		clearStatusAfter(tr("ui_term_ctrl_c_keyboard"), 2500*time.Millisecond)
	})
	clearBtn.OnTapped = func() { sendKey("clear\r") }
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyUp}, func(fyne.Shortcut) { sendKey("\x1bOA") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyDown}, func(fyne.Shortcut) { sendKey("\x1bOB") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyRight}, func(fyne.Shortcut) { sendKey("\x1bOC") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeft}, func(fyne.Shortcut) { sendKey("\x1bOD") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageUp}, func(fyne.Shortcut) { sendKey("\x1b[5~") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageDown}, func(fyne.Shortcut) { sendKey("\x1b[6~") })
	btnHtop := widget.NewButtonWithIcon(tr("ui_term_btn_htop"), theme.ComputerIcon(), func() {
		cmd := `command -v htop >/dev/null 2>&1 || { echo '[ContainerWay] htop não encontrado. Instalando...'; if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y htop; elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y htop; elif command -v yum >/dev/null 2>&1; then sudo yum install -y htop; elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm htop; else echo '[ContainerWay] Gerenciador de pacotes não suportado para instalação automática.'; fi; }; command -v htop >/dev/null 2>&1 && htop`
		runTerminalCommand(cmd)
		showStatus(tr("ui_term_htop_opening"))
		clearStatusAfter(tr("ui_term_htop_opening"), 2200*time.Millisecond)
	})
	btnHtop.Importance = widget.MediumImportance
	btnNcdu := widget.NewButtonWithIcon(tr("ui_term_btn_ncdu"), theme.StorageIcon(), func() {
		cmd := `command -v ncdu >/dev/null 2>&1 || { echo '[ContainerWay] ncdu não encontrado. Instalando...'; if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y ncdu; elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y ncdu; elif command -v yum >/dev/null 2>&1; then sudo yum install -y ncdu; elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm ncdu; else echo '[ContainerWay] Gerenciador de pacotes não suportado para instalação automática.'; fi; }; command -v ncdu >/dev/null 2>&1 && cd / && ncdu`
		runTerminalCommand(cmd)
		showStatus(tr("ui_term_ncdu_opening"))
		clearStatusAfter(tr("ui_term_ncdu_opening"), 2200*time.Millisecond)
	})
	btnNcdu.Importance = widget.MediumImportance
	btnCopyOutput.OnTapped = func() {
		content := getTerminalLog()
		if content == "" {
			showStatus(tr("ui_term_no_output_copy"))
			clearStatusAfter(tr("ui_term_no_output_copy"), 2200*time.Millisecond)
			return
		}
		ui.win.Clipboard().SetContent(content)
		showStatus(tr("ui_term_output_copied"))
		clearStatusAfter(tr("ui_term_output_copied"), 2500*time.Millisecond)
	}
	btnCmdList := widget.NewButtonWithIcon(tr("ui_term_btn_cmd_list"), theme.InfoIcon(), func() {
		type commandItem struct {
			label   string
			cmd     string
			quick   bool
			profile string
		}
		sections := []struct {
			title string
			items []commandItem
		}{
			{
				title: "Criar pasta",
				items: []commandItem{
					{label: "Criar nova pasta", cmd: "mkdir nome_pasta", quick: true, profile: "Dev"},
					{label: "Criar com subpastas", cmd: "mkdir -p caminho/pasta/subpasta", profile: "Dev"},
				},
			},
			{
				title: "Permissões",
				items: []commandItem{
					{label: "Liberar tudo (recursivo)", cmd: "chmod -R 777 caminho_diretorio", quick: true, profile: "Infra"},
					{label: "Permissão recomendada (recursivo)", cmd: "chmod -R 755 caminho_diretorio", profile: "Infra"},
				},
			},
			{
				title: "Navegação",
				items: []commandItem{
					{label: "Mostrar caminho atual", cmd: "pwd", profile: "Dev"},
					{label: "Listar arquivos detalhado", cmd: "ls -lah", quick: true, profile: "Dev"},
					{label: "Entrar em diretório", cmd: "cd /caminho", quick: true, profile: "Dev"},
					{label: "Voltar um nível", cmd: "cd ..", quick: true, profile: "Dev"},
				},
			},
			{
				title: "Arquivos e diretórios",
				items: []commandItem{
					{label: "Criar arquivo vazio", cmd: "touch arquivo.txt", profile: "Dev"},
					{label: "Copiar arquivo", cmd: "cp arquivo.txt /destino/", profile: "Dev"},
					{label: "Renomear/mover", cmd: "mv arquivo.txt novo_nome.txt", profile: "Dev"},
					{label: "Remover arquivo", cmd: "rm arquivo.txt", profile: "Dev"},
					{label: "Remover pasta", cmd: "rm -rf pasta_antiga", quick: true, profile: "Infra"},
				},
			},
			{
				title: "Disco e memória",
				items: []commandItem{
					{label: "Uso de disco", cmd: "df -h", quick: true, profile: "Infra"},
					{label: "Tamanho de pasta", cmd: "du -sh /var/log", profile: "Infra"},
					{label: "Uso de memória", cmd: "free -h", quick: true, profile: "Infra"},
				},
			},
			{
				title: "Processos e serviços",
				items: []commandItem{
					{label: "Filtrar processo", cmd: "ps aux | grep nome", profile: "Infra"},
					{label: "Monitor de processos", cmd: "top", quick: true, profile: "Infra"},
					{label: "Monitor avançado", cmd: "htop", quick: true, profile: "Infra"},
					{label: "Status de serviço", cmd: "systemctl status nginx", profile: "Infra"},
				},
			},
			{
				title: "Rede",
				items: []commandItem{
					{label: "Interfaces de rede", cmd: "ip a", profile: "Infra"},
					{label: "Teste de conectividade", cmd: "ping 8.8.8.8", quick: true, profile: "Infra"},
					{label: "Portas abertas", cmd: "ss -tulpen", profile: "Infra"},
				},
			},
			{
				title: "Dono e grupo",
				items: []commandItem{
					{label: "Alterar dono/grupo", cmd: "chown usuario:grupo arquivo_ou_pasta", profile: "Infra"},
					{label: "Alterar dono/grupo recursivo", cmd: "chown -R usuario:grupo caminho_diretorio", profile: "Infra"},
				},
			},
			{
				title: "Docker",
				items: []commandItem{
					{label: "Listar contêineres", cmd: "docker ps -a", quick: true, profile: "Docker"},
					{label: "Ver logs do contêiner", cmd: "docker logs -f nome_container", profile: "Docker"},
					{label: "Entrar no contêiner", cmd: "docker exec -it nome_container bash", quick: true, profile: "Docker"},
					{label: "Reiniciar contêiner", cmd: "docker restart nome_container", profile: "Docker"},
				},
			},
			{
				title: "Docker Compose",
				items: []commandItem{
					{label: "Listar serviços do compose", cmd: "docker compose -f /opt/siplan/docker-compose-orion.yml -p orion ps", profile: "Docker"},
					{label: "Reiniciar tudo (recriar + pull)", cmd: "docker compose -f /opt/siplan/docker-compose-orion.yml -p orion up -d --force-recreate --pull always", quick: true, profile: "Docker"},
					{label: "Reiniciar serviço específico", cmd: "docker compose -f /opt/siplan/docker-compose-orion.yml -p orion up -d --force-recreate --pull always nome_servico", quick: true, profile: "Docker"},
				},
			},
		}
		insertCommand := func(cmd string) {
			if strings.TrimSpace(cmd) == "" || closed.Load() {
				return
			}
			sendKey(cmd)
			status.SetText("Comando inserido no terminal: " + cmd)
		}
		isDangerousCommand := func(cmd string) bool {
			c := strings.ToLower(strings.TrimSpace(cmd))
			if c == "" {
				return false
			}
			dangerPatterns := []string{
				"rm -rf /",
				"rm -rf",
				"chmod -r 777",
				"chmod 777 -r",
				"mkfs",
				"dd if=",
				":(){ :|:& };:",
			}
			for _, p := range dangerPatterns {
				if strings.Contains(c, p) {
					return true
				}
			}
			return false
		}
		insertAndRunCommand := func(cmd string) {
			if strings.TrimSpace(cmd) == "" || closed.Load() {
				return
			}
			runNow := func() {
				sendKey(cmd + "\r")
				status.SetText("Comando executado no terminal: " + cmd)
			}
			if isDangerousCommand(cmd) {
				dialog.ShowConfirm(
					tr("dlg_term_danger_title"),
					fmt.Sprintf(tr("dlg_term_danger_body_fmt"), cmd),
					func(ok bool) {
						if ok {
							runNow()
						}
					},
					ui.win,
				)
				return
			}
			runNow()
		}
		confirmTerminalAction := func(actionLabel, cmd string, willExecute bool, onConfirm func()) {
			modeText := tr("dlg_term_mode_insert")
			if willExecute {
				modeText = tr("dlg_term_mode_exec")
			}
			message := fmt.Sprintf(tr("dlg_term_action_intro_fmt"), actionLabel, modeText, cmd)
			dialog.ShowConfirm(
				tr("dlg_term_action_title"),
				message,
				func(ok bool) {
					if ok && onConfirm != nil {
						onConfirm()
					}
				},
				ui.win,
			)
		}
		composeFileEntry := widget.NewEntry()
		composeFileEntry.SetPlaceHolder("/caminho/docker-compose.yml")
		composeProjectEntry := widget.NewEntry()
		composeProjectEntry.SetPlaceHolder("nome_projeto")
		composeServiceEntry := widget.NewEntry()
		composeServiceEntry.SetPlaceHolder("nome_servico")
		composePrefix := func() string {
			composeFile := strings.TrimSpace(composeFileEntry.Text)
			composeProject := strings.TrimSpace(composeProjectEntry.Text)
			base := "docker compose"
			if composeFile != "" {
				base += " -f " + composeFile
			}
			if composeProject != "" {
				base += " -p " + composeProject
			}
			return base
		}
		composeServiceOrWarn := func() (string, bool) {
			service := strings.TrimSpace(composeServiceEntry.Text)
			if service == "" {
				showStatus("Informe o nome do serviço para este comando Docker Compose.")
				clearStatusAfter("Informe o nome do serviço para este comando Docker Compose.", 2400*time.Millisecond)
				return "", false
			}
			return service, true
		}
		composeToolsCard := func() fyne.CanvasObject {
			composeFields := fynecontainer.NewGridWithColumns(3,
				fynecontainer.NewVBox(
					widget.NewLabelWithStyle("Compose file", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					composeFileEntry,
				),
				fynecontainer.NewVBox(
					widget.NewLabelWithStyle("Projeto (-p)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					composeProjectEntry,
				),
				fynecontainer.NewVBox(
					widget.NewLabelWithStyle("Serviço", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					composeServiceEntry,
				),
			)
			validateCmd := func() string {
				return composePrefix() + " config -q"
			}
			recreateAllCmd := func() string {
				return composePrefix() + " up -d --force-recreate --pull always"
			}
			recreateServiceCmd := func() (string, bool) {
				service, ok := composeServiceOrWarn()
				if !ok {
					return "", false
				}
				return recreateAllCmd() + " " + service, true
			}
			diagnosePsCmd := func() string {
				return composePrefix() + " ps"
			}
			diagnoseLogsCmd := func() (string, bool) {
				service, ok := composeServiceOrWarn()
				if !ok {
					return "", false
				}
				return composePrefix() + " logs -f " + service, true
			}
			diagnoseTopCmd := func() (string, bool) {
				service, ok := composeServiceOrWarn()
				if !ok {
					return "", false
				}
				return composePrefix() + " top " + service, true
			}
			detectComposeCmd := func() string {
				return "find /opt -maxdepth 5 -type f \\( -name 'docker-compose*.yml' -o -name 'docker-compose*.yaml' -o -name 'compose*.yml' -o -name 'compose*.yaml' \\) 2>/dev/null | head -n 20"
			}
			redeployDownUpCmd := func() string {
				return composePrefix() + " down && " + composePrefix() + " up -d --pull always"
			}
			healthCheckCmd := func() string {
				base := composePrefix()
				return base + " ps; " + base + " ps | grep -Ei 'unhealthy|exit|restarting' || echo 'Sem serviços em estado crítico (unhealthy/exited/restarting).'"
			}
			insertValidateBtn := widget.NewButton("Inserir validação", func() {
				cmd := validateCmd()
				confirmTerminalAction("Inserir validação", cmd, false, func() { insertCommand(cmd) })
			})
			runValidateBtn := widget.NewButton("Validar e executar", func() {
				cmd := validateCmd()
				confirmTerminalAction("Validar e executar", cmd, true, func() { insertAndRunCommand(cmd) })
			})
			insertRecreateAllBtn := widget.NewButton("Inserir reiniciar tudo", func() {
				cmd := recreateAllCmd()
				confirmTerminalAction("Inserir reiniciar tudo", cmd, false, func() { insertCommand(cmd) })
			})
			runRecreateAllBtn := widget.NewButton("Executar reiniciar tudo", func() {
				cmd := recreateAllCmd()
				confirmTerminalAction("Executar reiniciar tudo", cmd, true, func() { insertAndRunCommand(cmd) })
			})
			insertRecreateSvcBtn := widget.NewButton("Inserir reiniciar serviço", func() {
				cmd, ok := recreateServiceCmd()
				if ok {
					confirmTerminalAction("Inserir reiniciar serviço", cmd, false, func() { insertCommand(cmd) })
				}
			})
			runRecreateSvcBtn := widget.NewButton("Executar reiniciar serviço", func() {
				cmd, ok := recreateServiceCmd()
				if ok {
					confirmTerminalAction("Executar reiniciar serviço", cmd, true, func() { insertAndRunCommand(cmd) })
				}
			})
			diagPsBtn := widget.NewButton("Diagnóstico: ps", func() {
				cmd := diagnosePsCmd()
				confirmTerminalAction("Diagnóstico: ps", cmd, true, func() { insertAndRunCommand(cmd) })
			})
			diagLogsBtn := widget.NewButton("Diagnóstico: logs serviço", func() {
				cmd, ok := diagnoseLogsCmd()
				if ok {
					confirmTerminalAction("Diagnóstico: logs serviço", cmd, true, func() { insertAndRunCommand(cmd) })
				}
			})
			diagTopBtn := widget.NewButton("Diagnóstico: top serviço", func() {
				cmd, ok := diagnoseTopCmd()
				if ok {
					confirmTerminalAction("Diagnóstico: top serviço", cmd, true, func() { insertAndRunCommand(cmd) })
				}
			})
			detectComposeBtn := widget.NewButton("Buscar compose em /opt", func() {
				cmd := detectComposeCmd()
				confirmTerminalAction("Buscar compose em /opt", cmd, true, func() { insertAndRunCommand(cmd) })
			})
			redeployBtn := widget.NewButton("Executar down && up", func() {
				cmd := redeployDownUpCmd()
				confirmTerminalAction("Executar down && up", cmd, true, func() { insertAndRunCommand(cmd) })
			})
			healthBtn := widget.NewButton("Saúde pós-deploy", func() {
				cmd := healthCheckCmd()
				confirmTerminalAction("Saúde pós-deploy", cmd, true, func() { insertAndRunCommand(cmd) })
			})
			actionGrid := fynecontainer.NewGridWithColumns(3,
				insertValidateBtn,
				runValidateBtn,
				insertRecreateAllBtn,
				runRecreateAllBtn,
				insertRecreateSvcBtn,
				runRecreateSvcBtn,
				diagPsBtn,
				diagLogsBtn,
				diagTopBtn,
				detectComposeBtn,
				redeployBtn,
				healthBtn,
			)
			return fynecontainer.NewVBox(
				widget.NewLabel(tr("ui_linux_cmd_hint_vars")),
				composeFields,
				actionGrid,
			)
		}
		favorites := loadTerminalCommandFavorites()
		favoritesMap := map[string]bool{}
		for _, fav := range favorites {
			favoritesMap[strings.TrimSpace(fav)] = true
		}
		saveFavorites := func() {
			flat := make([]string, 0, len(favoritesMap))
			for cmd, enabled := range favoritesMap {
				if enabled && strings.TrimSpace(cmd) != "" {
					flat = append(flat, cmd)
				}
			}
			saveTerminalCommandFavorites(flat)
		}
		var rebuildResults func(filter string)
		activeFilter := ""
		selectedProfile := "Todos"
		onlyFavorites := false
		buildCommandRow := func(item commandItem) fyne.CanvasObject {
			descLabel := widget.NewLabel(item.label)
			cmdLabel := widget.NewLabelWithStyle(item.cmd, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
			insertBtn := widget.NewButtonWithIcon(tr("ui_linux_cmd_insert"), theme.ContentAddIcon(), func(cmd string) func() {
				return func() {
					confirmTerminalAction("Inserir comando", cmd, false, func() { insertCommand(cmd) })
				}
			}(item.cmd))
			insertBtn.Importance = widget.MediumImportance
			runBtn := widget.NewButtonWithIcon(tr("ui_linux_cmd_insert_run"), theme.MediaPlayIcon(), func(cmd string) func() {
				return func() {
					confirmTerminalAction("Inserir e executar comando", cmd, true, func() { insertAndRunCommand(cmd) })
				}
			}(item.cmd))
			runBtn.Importance = widget.LowImportance
			favIcon := terminalStarOutlineIcon
			if favoritesMap[item.cmd] {
				favIcon = terminalStarFilledIcon
			}
			favBtn := widget.NewButtonWithIcon("", favIcon, func(cmd string) func() {
				return func() {
					favoritesMap[cmd] = !favoritesMap[cmd]
					if favoritesMap[cmd] {
						showStatus("Comando favoritado: " + cmd)
					} else {
						showStatus("Comando removido dos favoritos: " + cmd)
					}
					saveFavorites()
					if rebuildResults != nil {
						rebuildResults(activeFilter)
					}
				}
			}(item.cmd))
			favBtn.Importance = widget.LowImportance
			btns := fynecontainer.NewHBox(insertBtn, runBtn, favBtn)
			return fynecontainer.NewBorder(nil, nil, nil, btns, fynecontainer.NewVBox(descLabel, cmdLabel))
		}
		results := fynecontainer.NewVBox()
		onlyQuick := false
		rebuildResults = func(filter string) {
			filter = strings.ToLower(strings.TrimSpace(filter))
			rows := []fyne.CanvasObject{
				widget.NewLabel(tr("ui_linux_cmd_hint_insert")),
				widget.NewSeparator(),
			}
			matchCount := 0
			quickRows := []fyne.CanvasObject{}
			quickCount := 0
			seenQuick := map[string]bool{}
			sectionBlocks := []fyne.CanvasObject{}
			for _, section := range sections {
				sectionRows := []fyne.CanvasObject{}
				for _, item := range section.items {
					if onlyQuick && !item.quick {
						continue
					}
					if onlyFavorites && !favoritesMap[item.cmd] {
						continue
					}
					if selectedProfile != "Todos" && !strings.EqualFold(strings.TrimSpace(item.profile), selectedProfile) {
						continue
					}
					searchText := strings.ToLower(section.title + " " + item.label + " " + item.cmd)
					if filter != "" && !strings.Contains(searchText, filter) {
						continue
					}
					matchCount++
					sectionRows = append(sectionRows, buildCommandRow(item))
					if item.quick && !seenQuick[item.cmd] {
						seenQuick[item.cmd] = true
						quickRows = append(quickRows, buildCommandRow(item))
						quickCount++
					}
				}
				if len(sectionRows) == 0 {
					continue
				}
				if !onlyQuick {
					sectionBlocks = append(sectionBlocks, widget.NewLabelWithStyle(section.title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
					sectionBlocks = append(sectionBlocks, sectionRows...)
					sectionBlocks = append(sectionBlocks, widget.NewSeparator())
				}
			}
			if quickCount > 0 {
				rows = append(rows, widget.NewLabelWithStyle(tr("ui_linux_most_used"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
				rows = append(rows, quickRows...)
				rows = append(rows, widget.NewSeparator())
			}
			if !onlyQuick {
				rows = append(rows, sectionBlocks...)
			}
			if matchCount == 0 {
				rows = append(rows, widget.NewLabel(tr("ui_linux_cmd_none")))
			}
			results.Objects = rows
			results.Refresh()
		}
		searchEntry := widget.NewEntry()
		searchEntry.SetPlaceHolder(tr("ui_linux_cmd_search_ph"))
		searchEntry.OnChanged = func(s string) {
			activeFilter = s
			rebuildResults(activeFilter)
		}
		profileSelect := widget.NewSelect([]string{"Todos", "Dev", "Infra", "Docker"}, func(v string) {
			if strings.TrimSpace(v) == "" {
				v = "Todos"
			}
			selectedProfile = v
			rebuildResults(activeFilter)
		})
		profileSelect.SetSelected("Todos")
		quickToggle := widget.NewCheck(tr("ui_linux_cmd_quick_only"), func(v bool) {
			onlyQuick = v
			rebuildResults(activeFilter)
		})
		favToggle := widget.NewCheck(tr("ui_linux_cmd_fav_only"), func(v bool) {
			onlyFavorites = v
			rebuildResults(activeFilter)
		})
		rebuildResults("")
		dockerComposeSection := widget.NewAccordion(
			widget.NewAccordionItem(tr("ui_linux_cmd_compose_acc"), composeToolsCard()),
		)
		dockerComposeSection.CloseAll()
		commandsSectionBody := fynecontainer.NewVBox(
			searchEntry,
			fynecontainer.NewHBox(
				fynecontainer.NewGridWrap(fyne.NewSize(220, profileSelect.MinSize().Height), profileSelect),
				layout.NewSpacer(),
				quickToggle,
				favToggle,
			),
			widget.NewSeparator(),
			results,
		)
		commandsSection := widget.NewAccordion(
			widget.NewAccordionItem(tr("ui_linux_cmd_accordion"), commandsSectionBody),
		)
		commandsSection.Open(0)
		content := fynecontainer.NewVBox(
			widget.NewLabelWithStyle(tr("ui_linux_cmd_dlg_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			dockerComposeSection,
			commandsSection,
		)
		contentScroller := fynecontainer.NewVScroll(content)
		contentScroller.SetMinSize(fyne.NewSize(820, 480))
		dialog.ShowCustom(tr("ui_linux_cmd_dlg_title"), tr("compare_close"), contentScroller, ui.win)
		showStatus(tr("ui_term_cmd_list_showing"))
		clearStatusAfter(tr("ui_term_cmd_list_showing"), 1800*time.Millisecond)
	})
	btnCmdList.Importance = widget.MediumImportance
	ui.win.Canvas().SetOnTypedRune(func(r rune) { sendKey(string(r)) })
	ui.win.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k == nil {
			return
		}
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			sendKey("\r")
		case fyne.KeyBackspace:
			sendKey("\x7f")
		case fyne.KeyTab:
			sendKey("\t")
		case fyne.KeyEscape:
			sendKey("\x1b")
		case fyne.KeyUp:
			sendKey("\x1bOA")
		case fyne.KeyDown:
			sendKey("\x1bOB")
		case fyne.KeyRight:
			sendKey("\x1bOC")
		case fyne.KeyLeft:
			sendKey("\x1bOD")
		case fyne.KeyHome:
			sendKey("\x1b[H")
		case fyne.KeyEnd:
			sendKey("\x1b[F")
		case fyne.KeyDelete:
			sendKey("\x1b[3~")
		case fyne.KeyPageUp:
			sendKey("\x1b[5~")
		case fyne.KeyPageDown:
			sendKey("\x1b[6~")
		case fyne.KeyF10:
			sendKey("\x1b[21~")
		}
	})

	const terminalToolBtnW = float32(66)
	const terminalCopyBtnW = float32(98)
	ctrlCWrap := fynecontainer.NewGridWrap(fyne.NewSize(terminalToolBtnW, ctrlCBtn.MinSize().Height), ctrlCBtn)
	copyWrap := fynecontainer.NewGridWrap(fyne.NewSize(terminalCopyBtnW, btnCopyOutput.MinSize().Height), btnCopyOutput)
	clearWrap := fynecontainer.NewGridWrap(fyne.NewSize(terminalToolBtnW+8, clearBtn.MinSize().Height), clearBtn)
	controls := fynecontainer.NewHBox(btnHtop, btnNcdu, btnCmdList, layout.NewSpacer(), ctrlCWrap, copyWrap, clearWrap)
	if ui.useCompactLayout() {
		controls = fynecontainer.NewVBox(
			fynecontainer.NewHScroll(fynecontainer.NewHBox(btnHtop, btnNcdu, btnCmdList)),
			fynecontainer.NewHBox(ctrlCWrap, copyWrap, clearWrap, layout.NewSpacer()),
		)
	}
	toggleHostBtn := widget.NewButtonWithIcon(tr("ui_term_details"), theme.InfoIcon(), func() {
		detailsText := strings.Join([]string{
			hostOSValue.Text,
			hostResValue.Text,
			hostTimeValue.Text,
			hostUserValue.Text,
		}, "\n")
		dialog.ShowInformation(tr("dlg_host_details"), detailsText, ui.win)
	})
	toggleHostBtn.Importance = widget.LowImportance
	hostCompactBar := fynecontainer.NewBorder(nil, nil, nil, toggleHostBtn, hostCompact)
	header := fynecontainer.NewVBox(controls)
	footer = fynecontainer.NewVBox(widget.NewSeparator(), statusRow, hostCompactBar)
	body = fynecontainer.NewBorder(
		header,
		footer,
		nil,
		nil,
		terminalViewport,
	)
	ui.openSettingsFullscreenWithBack(tr("ui_term_title"), body, closeTerminal)
	ttyActiveMsg := fmt.Sprintf(tr("ui_term_tty_active_fmt"), host)
	showStatus(ttyActiveMsg)
	clearStatusAfter(ttyActiveMsg, 2200*time.Millisecond)
	lastCanvas = terminalViewport.Size()
	if lastCanvas.Width > 0 && lastCanvas.Height > 0 {
		applyResize(lastCanvas)
	}
	return nil
}

// newHintIconButton executa parte da logica deste modulo.
func newHintIconButton(icon fyne.Resource, hint string, status *widget.Label, tapped func()) *hintIconButton {
	b := &hintIconButton{
		hint:   strings.TrimSpace(hint),
		status: status,
	}
	b.Text = ""
	b.Icon = icon
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

// MouseIn executa parte da logica deste modulo.
func (b *hintIconButton) MouseIn(ev *desktop.MouseEvent) {
	b.Button.MouseIn(ev)
	if b.status == nil || b.hint == "" {
		return
	}
	b.prevMsg = b.status.Text
	b.hover = true
	b.status.SetText(b.hint)
}

// MouseOut executa parte da logica deste modulo.
func (b *hintIconButton) MouseOut() {
	b.Button.MouseOut()
	if b.status == nil || !b.hover {
		return
	}
	if b.status.Text == b.hint {
		b.status.SetText(b.prevMsg)
	}
	b.hover = false
}

// splitKnownHostsFiles executa parte da logica deste modulo.
func splitKnownHostsFiles(s string) []string {
	var out []string
	for _, part := range strings.Split(s, "|") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// formatConnectionTestStatus executa parte da logica deste modulo.
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

// profileSecretKey executa parte da logica deste modulo.
func profileSecretKey(name, host, user string) string {
	n := strings.TrimSpace(name)
	if n != "" {
		return "name:" + strings.ToLower(n)
	}
	return "hostuser:" + strings.ToLower(strings.TrimSpace(host)) + "|" + strings.ToLower(strings.TrimSpace(user))
}

// getTransientSecret executa parte da logica deste modulo.
func getTransientSecret(key string) (transientSecret, bool) {
	loginSecretMu.Lock()
	defer loginSecretMu.Unlock()
	sec, ok := loginSessionSecrets[key]
	return sec, ok
}

// setTransientSecret executa parte da logica deste modulo.
func setTransientSecret(key string, sec transientSecret) {
	loginSecretMu.Lock()
	defer loginSecretMu.Unlock()
	loginSessionSecrets[key] = sec
}

// deleteTransientSecret executa parte da logica deste modulo.
func deleteTransientSecret(key string) {
	loginSecretMu.Lock()
	defer loginSecretMu.Unlock()
	delete(loginSessionSecrets, key)
}

// parseParallelWorkers executa parte da logica deste modulo.
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

// buildExplorer executa parte da logica deste modulo.
func buildExplorer(w fyne.Window, s *session.Session, parallelJobs int, creds session.Credentials) fyne.CanvasObject {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := s.Docker.ContainerList(ctx, dcontainer.ListOptions{All: false})
	if err != nil {
		errLabel := widget.NewLabel(fmt.Sprintf(tr("ex_docker_err_fmt"), err))
		errLabel.Wrapping = fyne.TextWrapWord
		closeBtn := widget.NewButtonWithIcon(tr("ex_docker_close"), theme.LogoutIcon(), func() {
			finalizeLocalAccessSession(w, s, "Sessão encerrada após erro ao acessar o Docker")
		})
		closeBtn.Importance = widget.DangerImportance
		w.SetCloseIntercept(func() {
			w.SetCloseIntercept(nil)
			finalizeLocalAccessSession(w, s, "Sessão encerrada (fechamento da janela após erro no Docker)")
		})
		inner := fynecontainer.NewVBox(errLabel, widget.NewSeparator(), closeBtn)
		return fynecontainer.NewPadded(widget.NewCard(tr("ex_docker_card_title"), tr("ex_docker_card_sub"), inner))
	}

	ui := &explorer{
		win:                w,
		s:                  s,
		hfs:                &hostfs.FS{Client: s.SFTP},
		connCreds:          creds,
		leftPath:           homeOrRoot(),
		rightPath:          "/",
		hostMode:           true,
		containerOpts:      []string{tr("ex_ctx_host_folders")},
		containerIDs:       []string{""},
		leftSel:            -1,
		rightSel:           -1,
		tm:                 &transfer.Manager{},
		parallelJobs:       parallelJobs,
		activePane:         "left",
		remoteEditSessions: map[string]*remoteEditSession{},
		sudoTTL:            10 * time.Minute,
	}
	ui.opHistory = loadOperationHistoryPreference()
	appendAuditLog("sessao", "Explorador iniciado para "+strings.TrimSpace(creds.Host))
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
	ui.leftSearch.SetPlaceHolder(tr("ex_placeholder_left"))
	ui.leftTypeFilter = widget.NewSelect([]string{tr("ex_filter_all"), tr("ex_filter_dirs"), tr("ex_filter_files")}, func(_ string) {
		ui.applyLeftFilter()
	})
	ui.leftTypeFilter.SetSelected(tr("ex_filter_all"))
	ui.rightSearch = widget.NewEntry()
	ui.rightSearch.SetPlaceHolder(tr("ex_placeholder_right"))
	ui.rightTypeFilter = widget.NewSelect([]string{tr("ex_filter_all"), tr("ex_filter_dirs"), tr("ex_filter_files")}, func(_ string) {
		ui.applyRightFilter()
	})
	ui.rightTypeFilter.SetSelected(tr("ex_filter_all"))
	ui.leftFooterInfo = widget.NewLabel(tr("ui_footer_local_pick"))
	ui.leftFooterInfo.Wrapping = fyne.TextWrapWord
	ui.rightFooterInfo = widget.NewLabel(tr("ui_footer_server_pick"))
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
				ic.SetResource(explorerListIconParent)
			case e.IsDir:
				ic.SetResource(explorerListIconFolder)
			default:
				ic.SetResource(explorerListIconFile)
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
				ic.SetResource(explorerListIconParent)
			case e.IsDir:
				ic.SetResource(explorerListIconFolder)
			default:
				ic.SetResource(explorerListIconFile)
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
		ui.refreshRightShortcutOptions("")
	})
	ui.ctxSelect.SetSelectedIndex(0)

	ui.btnOpenLocal = widget.NewButtonWithIcon(tr("ex_open"), theme.FolderOpenIcon(), func() { ui.onLeftActivate() })
	ui.btnOpenRemote = widget.NewButtonWithIcon(tr("ex_open"), theme.FolderOpenIcon(), func() { ui.onRightActivate() })
	ui.btnLeftSend = widget.NewButtonWithIcon(tr("ex_send"), theme.UploadIcon(), func() { ui.upload() })
	ui.btnRightRecv = widget.NewButtonWithIcon(tr("ex_receive"), theme.DownloadIcon(), func() { ui.download() })
	ui.btnLeftSendBatch = widget.NewButtonWithIcon(tr("ex_send_visible"), theme.ContentAddIcon(), func() { ui.uploadVisibleBatch() })
	ui.btnRightRecvBatch = widget.NewButtonWithIcon(tr("ex_recv_visible"), theme.ContentAddIcon(), func() { ui.downloadVisibleBatch() })
	ui.btnOpenLocal.Importance = widget.MediumImportance
	ui.btnOpenRemote.Importance = widget.MediumImportance
	ui.btnLeftSend.Importance = widget.HighImportance
	ui.btnRightRecv.Importance = widget.HighImportance
	ui.btnLeftSendBatch.Importance = widget.MediumImportance
	ui.btnRightRecvBatch.Importance = widget.MediumImportance
	ui.hintAddLeft = newHintIconButton(theme.ContentAddIcon(), tr("hint_add_left"), ui.status, func() { ui.addLeftFavoriteCurrentPath() })
	ui.hintRemoveLeft = newHintIconButton(theme.ContentRemoveIcon(), tr("hint_remove_left"), ui.status, func() { ui.removeLeftFavoriteCurrentPath() })
	ui.hintAddRight = newHintIconButton(theme.ContentAddIcon(), tr("hint_add_right"), ui.status, func() { ui.addRightFavoriteCurrentPath() })
	ui.hintRemoveRight = newHintIconButton(theme.ContentRemoveIcon(), tr("hint_remove_right"), ui.status, func() { ui.removeRightFavoriteCurrentPath() })
	btnBackLocal := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goLeftBack() })
	btnUpLocal := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goLeftUp() })
	btnHomeLocal := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goLeftHome() })
	btnReloadLocal := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshLeft() })
	btnBackRemote := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { ui.goRightBack() })
	btnUpRemote := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { ui.goRightUp() })
	btnHomeRemote := widget.NewButtonWithIcon("", theme.HomeIcon(), func() { ui.goRightHome() })
	btnReloadRemote := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { ui.refreshRight() })

	ui.btnUp = widget.NewButtonWithIcon(tr("ex_send"), theme.UploadIcon(), func() { ui.upload() })
	ui.btnUp.Importance = widget.HighImportance
	ui.btnDown = widget.NewButtonWithIcon(tr("ex_receive"), theme.DownloadIcon(), func() { ui.download() })
	ui.btnDown.Importance = widget.HighImportance
	ui.btnHistory = widget.NewButtonWithIcon(tr("ex_history"), theme.HistoryIcon(), func() { ui.showOperationHistory() })
	ui.btnCompare = widget.NewButtonWithIcon(tr("ex_compare"), theme.SearchIcon(), func() { ui.showCompareFoldersExplorer() })
	ui.btnCompare.Importance = widget.MediumImportance
	ui.btnDisconnect = widget.NewButtonWithIcon(tr("ex_logout"), theme.LogoutIcon(), func() {
		finalizeLocalAccessSession(w, s, "Sessão encerrada pelo usuário")
	})
	ui.btnDisconnect.Importance = widget.DangerImportance
	ui.lblSudoState = widget.NewLabel(tr("ex_sudo_inactive"))
	ui.btnDisableSudo = widget.NewButtonWithIcon(tr("ex_disable_sudo"), theme.CancelIcon(), func() { ui.disableSudoMode() })

	ui.btnBackToHub = widget.NewButtonWithIcon(tr("ex_back"), theme.NavigateBackIcon(), func() { ui.showSessionHub() })
	ui.btnBackToHub.Importance = widget.MediumImportance
	toolbarItems := []fyne.CanvasObject{
		ui.btnBackToHub,
		ui.btnUp,
		ui.btnDown,
		ui.btnHistory,
		ui.btnCompare,
	}
	toolbarItems = append(toolbarItems,
		layout.NewSpacer(),
		ui.lblSudoState,
		ui.btnDisableSudo,
		layout.NewSpacer(),
		ui.btnDisconnect,
	)
	toolbar := fynecontainer.NewHBox(toolbarItems...)
	if ui.useCompactLayout() {
		primaryRow := []fyne.CanvasObject{
			ui.btnBackToHub,
			ui.btnUp,
			ui.btnDown,
			ui.btnHistory,
			ui.btnCompare,
		}
		secondaryRow := []fyne.CanvasObject{
			ui.lblSudoState,
			ui.btnDisableSudo,
			layout.NewSpacer(),
			ui.btnDisconnect,
		}
		toolbar = fynecontainer.NewVBox(
			fynecontainer.NewHScroll(fynecontainer.NewHBox(primaryRow...)),
			fynecontainer.NewHBox(secondaryRow...),
		)
	}
	top := fynecontainer.NewVBox(
		fynecontainer.NewPadded(toolbar),
		widget.NewSeparator(),
	)

	leftFavs := ui.localShortcutOptions()
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
	ui.leftQuick.SetSelected(tr("sc_home"))

	rightFavs := ui.remoteShortcutOptions()
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

	ui.lblPaneLocal = widget.NewLabelWithStyle(tr("ex_pane_local"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	leftHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			widget.NewIcon(theme.HomeIcon()),
			ui.lblPaneLocal,
			layout.NewSpacer(),
		),
		fynecontainer.NewHBox(
			leftBackWrap,
			leftUpWrap,
			leftHomeWrap,
			leftReloadWrap,
			layout.NewSpacer(),
			leftQuickWrap,
			ui.hintAddLeft,
			ui.hintRemoveLeft,
		),
		fynecontainer.NewBorder(nil, nil, nil, leftTypeFilterWrap, ui.leftSearch),
		fynecontainer.NewHBox(
			ui.btnOpenLocal,
			ui.btnLeftSend,
			ui.btnLeftSendBatch,
		),
	)
	leftPaneBase := fynecontainer.NewBorder(
		fynecontainer.NewPadded(leftHead),
		nil, nil, nil,
		fynecontainer.NewPadded(fynecontainer.NewScroll(ui.leftList)),
	)
	leftPane := panelCard(leftPaneBase)

	ui.lblPaneRemote = widget.NewLabelWithStyle(tr("ex_pane_remote"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	rightHead := fynecontainer.NewVBox(
		fynecontainer.NewHBox(
			widget.NewIcon(theme.StorageIcon()),
			ui.lblPaneRemote,
			layout.NewSpacer(),
		),
		fynecontainer.NewHBox(
			rightBackWrap,
			rightUpWrap,
			rightHomeWrap,
			rightReloadWrap,
			layout.NewSpacer(),
			rightQuickWrap,
			ui.hintAddRight,
			ui.hintRemoveRight,
			ctxSelectWrap,
		),
		fynecontainer.NewBorder(nil, nil, nil, rightTypeFilterWrap, ui.rightSearch),
		fynecontainer.NewHBox(
			ui.btnOpenRemote,
			ui.btnRightRecv,
			ui.btnRightRecvBatch,
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

	w.SetCloseIntercept(func() {
		w.SetCloseIntercept(nil)
		finalizeLocalAccessSession(w, s, "Sessão encerrada (fechamento da janela)")
	})

	ui.explorerMain = fynecontainer.NewBorder(top, bottom, nil, nil, split)
	ui.sessionHub = buildSessionHub(ui)
	ui.explorerOnTop.Store(false)
	return ui.sessionHub
}

// hubSessionCard monta um bloco de módulo com texto que respeita a largura da coluna (evita sobreposição de Cards).
func hubSessionCard(title, description string, footer fyne.CanvasObject) fyne.CanvasObject {
	titleLbl := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleLbl.Wrapping = fyne.TextWrapWord
	descLbl := widget.NewLabel(description)
	descLbl.Wrapping = fyne.TextWrapWord
	inner := fynecontainer.NewVBox(
		titleLbl,
		descLbl,
		widget.NewSeparator(),
		footer,
	)
	return panelCard(fynecontainer.NewPadded(inner))
}

// buildSessionHub executa parte da logica deste modulo.
func buildSessionHub(ui *explorer) fyne.CanvasObject {
	hostDisp := strings.TrimSpace(ui.connCreds.Host)
	if hostDisp == "" {
		hostDisp = tr("hostUnknown")
	}
	sub := widget.NewLabel(fmt.Sprintf(tr("connectedToFmt"), hostDisp))
	sub.Alignment = fyne.TextAlignCenter
	sub.Wrapping = fyne.TextWrapWord

	search := widget.NewEntry()
	search.SetPlaceHolder(tr("searchPlaceholder"))

	openFiles := widget.NewButtonWithIcon(tr("open"), explorerListIconFolder, func() {
		ui.win.SetContent(ui.explorerMain)
		ui.explorerOnTop.Store(true)
		setExplorerWindow(ui.win)
	})
	openFiles.Importance = widget.MediumImportance
	cardFiles := hubSessionCard(
		tr("cardFilesTitle"),
		tr("cardFilesDesc"),
		fynecontainer.NewPadded(openFiles),
	)

	openDocker := widget.NewButtonWithIcon(tr("open"), hubIconDocker, func() {
		ui.showDockerContainerManager()
	})
	openDocker.Importance = widget.MediumImportance
	cardDocker := hubSessionCard(
		tr("cardDockerTitle"),
		tr("cardDockerDesc"),
		fynecontainer.NewPadded(openDocker),
	)

	type hubModule struct {
		wrap fyne.CanvasObject
		blob string
	}
	var mods []hubModule
	mods = append(mods, hubModule{
		wrap: fynecontainer.NewPadded(cardFiles),
		blob: hubSearchBlob("srchFiles"),
	})
	mods = append(mods, hubModule{
		wrap: fynecontainer.NewPadded(cardDocker),
		blob: hubSearchBlob("srchDocker"),
	})

	openDisks := widget.NewButtonWithIcon(tr("open"), hubIconDisks, func() {
		ui.showDiskStorageManager()
	})
	openDisks.Importance = widget.MediumImportance
	cardDisks := hubSessionCard(
		tr("cardDisksTitle"),
		tr("cardDisksDesc"),
		fynecontainer.NewPadded(openDisks),
	)
	mods = append(mods, hubModule{
		wrap: fynecontainer.NewPadded(cardDisks),
		blob: hubSearchBlob("srchDisks"),
	})

	openTerminal := widget.NewButtonWithIcon(tr("open"), hubIconTerminal, func() {
		ui.showTerminalConsole()
	})
	openTerminal.Importance = widget.MediumImportance
	cardTerminal := hubSessionCard(
		tr("cardTerminalTitle"),
		tr("cardTerminalDesc"),
		fynecontainer.NewPadded(openTerminal),
	)
	mods = append(mods, hubModule{
		wrap: fynecontainer.NewPadded(cardTerminal),
		blob: hubSearchBlob("srchTerminal"),
	})

	openAutomations := widget.NewButtonWithIcon(tr("open"), hubIconAutomations, func() {
		ui.showAutomationCenter()
	})
	openAutomations.Importance = widget.MediumImportance
	cardAutomations := hubSessionCard(
		tr("cardAutoTitle"),
		tr("cardAutoDesc"),
		fynecontainer.NewPadded(openAutomations),
	)
	mods = append(mods, hubModule{
		wrap: fynecontainer.NewPadded(cardAutomations),
		blob: hubSearchBlob("srchAuto"),
	})

	if isCurrentAccessAdmin() {
		btnUsers := widget.NewButtonWithIcon(tr("btnUsers"), hubIconUsers, func() { ui.showAccessUserManager() })
		btnMail := widget.NewButtonWithIcon(tr("btnMail"), hubIconMail, func() { ui.showMailNotifySettings() })
		btnUsers.Importance = widget.MediumImportance
		btnMail.Importance = widget.MediumImportance
		settingsBody := fynecontainer.NewVBox(
			fynecontainer.NewPadded(btnUsers),
			fynecontainer.NewPadded(btnMail),
		)
		cardSettings := hubSessionCard(
			tr("cardSettingsTitle"),
			tr("cardSettingsDesc"),
			settingsBody,
		)
		mods = append(mods, hubModule{
			wrap: fynecontainer.NewPadded(cardSettings),
			blob: hubSearchBlob("srchSettings"),
		})
	}

	hint := widget.NewLabel("")
	hint.Wrapping = fyne.TextWrapWord

	matchQuery := func(q, blob string) bool {
		q = strings.TrimSpace(strings.ToLower(q))
		if q == "" {
			return true
		}
		blob = strings.ToLower(blob)
		if strings.Contains(blob, q) {
			return true
		}
		for _, w := range strings.Fields(q) {
			if len(w) < 2 {
				continue
			}
			if strings.Contains(blob, w) {
				return true
			}
		}
		return false
	}

	applyFilter := func(q string) {
		nShown := 0
		for _, m := range mods {
			if matchQuery(q, m.blob) {
				m.wrap.Show()
				nShown++
			} else {
				m.wrap.Hide()
			}
		}
		if nShown == 0 {
			hint.SetText(tr("hintNoModules"))
		} else {
			hint.SetText("")
		}
	}

	search.OnChanged = func(_ string) {
		applyFilter(search.Text)
	}

	ncols := len(mods)
	if ncols > 3 {
		ncols = 3
	}
	grid := fynecontainer.NewGridWithColumns(ncols)
	for _, m := range mods {
		grid.Add(m.wrap)
	}

	btnThemeSys := widget.NewButtonWithIcon("", hubThemeIconSystem, nil)
	btnThemeLight := widget.NewButtonWithIcon("", hubThemeIconLight, nil)
	btnThemeDark := widget.NewButtonWithIcon("", hubThemeIconDark, nil)
	syncHubThemeButtons := func() {
		m := loadThemeMode(fyne.CurrentApp())
		set := func(b *widget.Button, active bool) {
			if active {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.MediumImportance
			}
			b.Refresh()
		}
		set(btnThemeSys, m == themeModeSystem)
		set(btnThemeLight, m == themeModeLight)
		set(btnThemeDark, m == themeModeDark)
	}
	applyHubThemeFromHub := func(mode string) {
		app := fyne.CurrentApp()
		app.Preferences().SetString(themePreferenceKey, mode)
		fyne.Do(func() {
			applyThemeMode(app, mode)
			syncHubThemeButtons()
		})
	}
	btnThemeSys.OnTapped = func() { applyHubThemeFromHub(themeModeSystem) }
	btnThemeLight.OnTapped = func() { applyHubThemeFromHub(themeModeLight) }
	btnThemeDark.OnTapped = func() { applyHubThemeFromHub(themeModeDark) }
	syncHubThemeButtons()

	btnHubManual := widget.NewButtonWithIcon(tr("manualBtn"), theme.HelpIcon(), func() {
		ui.showUserManual()
	})
	btnHubManual.Importance = widget.MediumImportance
	langSelect := widget.NewSelect([]string{
		langLabelForCode(langPTBR),
		langLabelForCode(langEN),
		langLabelForCode(langES),
	}, nil)
	langSelect.SetSelected(langLabelForCode(loadUILanguage(fyne.CurrentApp())))
	langSelect.OnChanged = func(sel string) {
		next := langCodeFromLabel(sel)
		if next == loadUILanguage(fyne.CurrentApp()) {
			return
		}
		saveUILanguage(fyne.CurrentApp(), next)
		ui.refreshSessionHubLanguage()
	}
	themeBar := fynecontainer.NewHBox(
		btnHubManual,
		widget.NewLabel(tr("langLabel")),
		langSelect,
		layout.NewSpacer(),
		widget.NewLabel(tr("themeLabel")),
		btnThemeSys,
		btnThemeLight,
		btnThemeDark,
	)

	head := widget.NewLabelWithStyle(tr("sessionStart"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	top := fynecontainer.NewVBox(
		themeBar,
		head,
		sub,
		widget.NewSeparator(),
		search,
		hint,
		widget.NewSeparator(),
	)

	// NewCenter usaria só o MinSize da grade — com 2 colunas (não admin) ficava estreita e “torta”.
	// NewMax faz a grade usar toda a largura da janela; cada coluna divide o espaço igualmente.
	moduleRow := fynecontainer.NewMax(grid)
	fill := fynecontainer.NewVBox(
		layout.NewSpacer(),
		moduleRow,
		layout.NewSpacer(),
	)
	root := fynecontainer.NewBorder(
		fynecontainer.NewPadded(top),
		nil, nil, nil,
		fill,
	)

	applyFilter("")
	return fynecontainer.NewPadded(root)
}

// showSessionHub executa parte da logica deste modulo.
func (ui *explorer) showSessionHub() {
	if ui.sessionHub == nil || ui.explorerMain == nil {
		return
	}
	ui.explorerOnTop.Store(false)
	ui.win.SetContent(ui.sessionHub)
	setSessionHubWindow(ui.win)
}

// applyExplorerLocale atualiza rótulos do explorador quando o idioma muda.
func (ui *explorer) applyExplorerLocale() {
	if ui == nil {
		return
	}
	if len(ui.containerOpts) > 0 {
		ui.containerOpts[0] = tr("ex_ctx_host_folders")
		if ui.ctxSelect != nil {
			idx := ui.ctxSelect.SelectedIndex()
			if idx < 0 || idx >= len(ui.containerOpts) {
				idx = 0
			}
			ui.ctxSelect.Options = ui.containerOpts
			ui.ctxSelect.SetSelectedIndex(idx)
		}
	}
	if ui.lblPaneLocal != nil {
		ui.lblPaneLocal.SetText(tr("ex_pane_local"))
	}
	if ui.lblPaneRemote != nil {
		ui.lblPaneRemote.SetText(tr("ex_pane_remote"))
	}
	optsFilter := []string{tr("ex_filter_all"), tr("ex_filter_dirs"), tr("ex_filter_files")}
	if ui.leftTypeFilter != nil {
		cur := normalizeExplorerTypeFilter(ui.leftTypeFilter.Selected)
		ui.leftTypeFilter.Options = optsFilter
		switch cur {
		case "dirs":
			ui.leftTypeFilter.SetSelected(tr("ex_filter_dirs"))
		case "files":
			ui.leftTypeFilter.SetSelected(tr("ex_filter_files"))
		default:
			ui.leftTypeFilter.SetSelected(tr("ex_filter_all"))
		}
	}
	if ui.rightTypeFilter != nil {
		cur := normalizeExplorerTypeFilter(ui.rightTypeFilter.Selected)
		ui.rightTypeFilter.Options = optsFilter
		switch cur {
		case "dirs":
			ui.rightTypeFilter.SetSelected(tr("ex_filter_dirs"))
		case "files":
			ui.rightTypeFilter.SetSelected(tr("ex_filter_files"))
		default:
			ui.rightTypeFilter.SetSelected(tr("ex_filter_all"))
		}
	}
	if ui.leftSearch != nil {
		ui.leftSearch.SetPlaceHolder(tr("ex_placeholder_left"))
	}
	if ui.rightSearch != nil {
		ui.rightSearch.SetPlaceHolder(tr("ex_placeholder_right"))
	}
	if ui.btnOpenLocal != nil {
		ui.btnOpenLocal.SetText(tr("ex_open"))
	}
	if ui.btnOpenRemote != nil {
		ui.btnOpenRemote.SetText(tr("ex_open"))
	}
	if ui.btnLeftSend != nil {
		ui.btnLeftSend.SetText(tr("ex_send"))
	}
	if ui.btnRightRecv != nil {
		ui.btnRightRecv.SetText(tr("ex_receive"))
	}
	if ui.btnLeftSendBatch != nil {
		ui.btnLeftSendBatch.SetText(tr("ex_send_visible"))
	}
	if ui.btnRightRecvBatch != nil {
		ui.btnRightRecvBatch.SetText(tr("ex_recv_visible"))
	}
	if ui.btnUp != nil {
		ui.btnUp.SetText(tr("ex_send"))
	}
	if ui.btnDown != nil {
		ui.btnDown.SetText(tr("ex_receive"))
	}
	if ui.btnBackToHub != nil {
		ui.btnBackToHub.SetText(tr("ex_back"))
	}
	if ui.btnHistory != nil {
		ui.btnHistory.SetText(tr("ex_history"))
	}
	if ui.btnCompare != nil {
		ui.btnCompare.SetText(tr("ex_compare"))
	}
	if ui.btnDisconnect != nil {
		ui.btnDisconnect.SetText(tr("ex_logout"))
	}
	if ui.btnDisableSudo != nil {
		ui.btnDisableSudo.SetText(tr("ex_disable_sudo"))
	}
	if ui.hintAddLeft != nil {
		ui.hintAddLeft.setHintText(tr("hint_add_left"))
	}
	if ui.hintRemoveLeft != nil {
		ui.hintRemoveLeft.setHintText(tr("hint_remove_left"))
	}
	if ui.hintAddRight != nil {
		ui.hintAddRight.setHintText(tr("hint_add_right"))
	}
	if ui.hintRemoveRight != nil {
		ui.hintRemoveRight.setHintText(tr("hint_remove_right"))
	}

	ui.refreshLeftShortcutOptions(leftQuickSelectLabelForPath(ui.leftPath))
	ui.refreshRightShortcutOptions(ui.rightPath)
	ui.updateBreadcrumb()
	ui.updateSudoUIState()
	ui.updateFooterPanels()
	if ui.leftList != nil {
		ui.leftList.Refresh()
	}
	if ui.rightList != nil {
		ui.rightList.Refresh()
	}
}

// refreshSessionHubLanguage reconstrói o hub quando o idioma muda (mantém a janela no hub se já estiver lá).
func (ui *explorer) refreshSessionHubLanguage() {
	if ui.win == nil || ui.sessionHub == nil || ui.explorerMain == nil {
		return
	}
	oldHub := ui.sessionHub
	wasHub := ui.win.Content() == oldHub
	ui.sessionHub = buildSessionHub(ui)
	ui.applyExplorerLocale()
	if wasHub {
		ui.win.SetContent(ui.sessionHub)
		setSessionHubWindow(ui.win)
	}
}

// useCompactLayout indica quando a janela exige layout compacto.
func (ui *explorer) useCompactLayout() bool {
	if ui == nil || ui.win == nil || ui.win.Canvas() == nil {
		return false
	}
	sz := ui.win.Canvas().Size()
	if sz.Width <= 0 && sz.Height <= 0 {
		return false
	}
	return sz.Width <= 1280 || sz.Height <= 800
}

// homeOrRoot executa parte da logica deste modulo.
func homeOrRoot() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "."
	}
	return h
}

// sizeLabel executa parte da logica deste modulo.
func sizeLabel(e fsutil.DirEntry) string {
	if e.Name == ".." {
		return ""
	}
	if e.IsDir {
		return ""
	}
	return transfer.FormatBytes(e.Size)
}

// panelCard executa parte da logica deste modulo.
func panelCard(content fyne.CanvasObject) fyne.CanvasObject {
	border := canvas.NewRectangle(theme.DisabledColor())
	border.FillColor = color.Transparent
	border.StrokeColor = theme.DisabledColor()
	border.StrokeWidth = 1.5
	return fynecontainer.NewMax(border, content)
}

// startRowDrag executa parte da logica deste modulo.
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

// updateRowDrag executa parte da logica deste modulo.
func (ui *explorer) updateRowDrag(deltaX float32) {
	if !ui.dragActive {
		return
	}
	ui.dragAccumX += deltaX
}

// finishRowDrag executa parte da logica deste modulo.
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

// copySelectedEntry executa parte da logica deste modulo.
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

// pasteCopiedTo executa parte da logica deste modulo.
func (ui *explorer) pasteCopiedTo(leftTarget bool, targetDir string) {
	if ui.copiedEntry == nil {
		dialog.ShowInformation(tr("dlg_paste_title"), tr("dlg_paste_need_copy"), ui.win)
		return
	}
	if leftTarget && localfs.IsWindowsDrivesVirtual(strings.TrimSpace(targetDir)) {
		dialog.ShowInformation(tr("dlg_paste_title"), tr("dlg_paste_drive_folder"), ui.win)
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
		dialog.ShowInformation(tr("dlg_paste_title"), tr("dlg_paste_same_ctx"), ui.win)
		return
	}
	ui.enqueueRemoteToRemote(src, targetDir)
	ui.startDrain()
}

// copyLocalFile executa parte da logica deste modulo.
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

// copyLocalDir executa parte da logica deste modulo.
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

// leftPathFooterLabel executa parte da logica deste modulo.
func leftPathFooterLabel(p string) string {
	if localfs.IsWindowsDrivesVirtual(p) {
		return tr("ex_path_drives_virtual")
	}
	return p
}

// updateBreadcrumb executa parte da logica deste modulo.
func (ui *explorer) updateBreadcrumb() {
	ui.leftPathLbl.SetText(fmt.Sprintf(tr("ex_foot_path_local"), leftPathFooterLabel(ui.leftPath)))
	ui.leftCrumbs.Objects = ui.makePathButtons(ui.leftPath, true)
	ui.leftCrumbs.Refresh()
	if ui.hostMode {
		ui.breadcrumb.SetText(fmt.Sprintf(tr("ex_breadcrumb_server"), ui.rightPath))
		ui.rightCrumbs.Objects = ui.makePathButtons(ui.rightPath, false)
		ui.rightCrumbs.Refresh()
		return
	}
	short := strings.TrimPrefix(ui.cfs.ID, "sha256:")
	if len(short) > 12 {
		short = short[:12]
	}
	ui.breadcrumb.SetText(fmt.Sprintf(tr("ex_breadcrumb_container"), short, ui.rightPath))
	ui.rightCrumbs.Objects = ui.makePathButtons(ui.rightPath, false)
	ui.rightCrumbs.Refresh()
}

// refreshLeft executa parte da logica deste modulo.
func (ui *explorer) refreshLeft() {
	rows, err := localfs.List(ui.leftPath)
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	ui.leftAll = rows
	ui.applyLeftFilter()
}

// applyLeftFilter executa parte da logica deste modulo.
func (ui *explorer) applyLeftFilter() {
	if ui.leftList == nil {
		return
	}
	selectedPath := ""
	if ui.leftSel >= 0 && ui.leftSel < len(ui.leftRows) {
		selectedPath = ui.leftRows[ui.leftSel].Path
	}
	criteria := parseRightFilterCriteria(ui.leftSearch.Text)
	typeNorm := "all"
	if ui.leftTypeFilter != nil {
		typeNorm = normalizeExplorerTypeFilter(ui.leftTypeFilter.Selected)
	}
	if criteria.term == "" && criteria.ext == "" && typeNorm == "all" {
		ui.leftRows = append([]fsutil.DirEntry(nil), ui.leftAll...)
	} else {
		filtered := make([]fsutil.DirEntry, 0, len(ui.leftAll))
		for _, e := range ui.leftAll {
			if e.Name == ".." || rightEntryMatches(e, criteria, typeNorm) {
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
	if criteria.term != "" || criteria.ext != "" || typeNorm != "all" {
		matches := 0
		for _, e := range ui.leftRows {
			if e.Name != ".." {
				matches++
			}
		}
		if matches == 0 {
			ui.status.SetText(fmt.Sprintf("Nenhum resultado para o filtro atual em %s.", leftPathFooterLabel(ui.leftPath)))
		}
	}
	ui.updateBreadcrumb()
	ui.updateActionState()
	ui.updateSummaryInfo()
}

// refreshRight executa parte da logica deste modulo.
func (ui *explorer) refreshRight() {
	ui.refreshRightImpl(true)
}

// refreshRightQuiet atualiza a lista direita sem alterar a barra de estado (ex.: após transferência, para não apagar "Concluído:").
func (ui *explorer) refreshRightQuiet() {
	ui.refreshRightImpl(false)
}

// refreshRightImpl executa parte da logica deste modulo.
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

// maybePromptRootAccess executa parte da logica deste modulo.
func (ui *explorer) maybePromptRootAccess(listErr error) {
	if listErr == nil || !ui.hostMode {
		return
	}
	if !isPermissionDeniedError(listErr) {
		return
	}
	ui.showSudoCredentialsDialog("Acesso negado")
}

// showSudoCredentialsDialog abre o formulário de credenciais sudo (explorador, discos/LVM, etc.).
func (ui *explorer) showSudoCredentialsDialog(windowTitle string) {
	if ui.rootPromptOpen.Load() {
		return
	}
	ui.rootPromptOpen.Store(true)

	userEntry := widget.NewEntry()
	userEntry.SetText(ui.connCreds.User)
	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder(tr("ui_sudo_pass_ph"))
	userEntry.Resize(fyne.NewSize(260, userEntry.MinSize().Height))
	passEntry.Resize(fyne.NewSize(260, passEntry.MinSize().Height))

	ui.openFormDialogWithShortcuts(
		windowTitle,
		"Aplicar sudo",
		"Cancelar",
		fyne.NewSize(460, 240),
		[]*widget.FormItem{
			widget.NewFormItem(tr("ui_sudo_fi_user"), userEntry),
			widget.NewFormItem("Senha", passEntry),
		},
		func() {
			defer ui.rootPromptOpen.Store(false)
			if strings.TrimSpace(passEntry.Text) == "" {
				dialog.ShowInformation(tr("dlg_sudo_need_pass"), tr("dlg_sudo_body_pass"), ui.win)
				return
			}
			user := strings.TrimSpace(userEntry.Text)
			if user == "" {
				dialog.ShowInformation(tr("dlg_sudo_need_pass"), tr("dlg_sudo_body_user"), ui.win)
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

// isPermissionDeniedError executa parte da logica deste modulo.
func isPermissionDeniedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "permission denied")
}

// enableSudoMode executa parte da logica deste modulo.
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
						tr("ui_sudo_elevate_fail_fmt"),
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

// disableSudoMode executa parte da logica deste modulo.
func (ui *explorer) disableSudoMode() {
	ui.sudoEnabled = false
	ui.sudoUser = ""
	ui.sudoPass = ""
	ui.sudoValidatedAt = time.Time{}
	ui.updateSudoUIState()
	ui.status.SetText(tr("ex_sudo_off_msg"))
}

// updateSudoUIState executa parte da logica deste modulo.
func (ui *explorer) updateSudoUIState() {
	if ui.lblSudoState == nil || ui.btnDisableSudo == nil {
		return
	}
	if ui.sudoEnabled && strings.TrimSpace(ui.sudoUser) != "" {
		ui.lblSudoState.SetText(fmt.Sprintf(tr("ex_sudo_active"), ui.sudoUser))
		ui.btnDisableSudo.Enable()
		return
	}
	ui.lblSudoState.SetText(tr("ex_sudo_inactive"))
	ui.btnDisableSudo.Disable()
}

// shellQuote executa parte da logica deste modulo.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// normalizeTerminalChunk remove sequências ANSI/CSI que quebram o layout no widget textual.
func normalizeTerminalChunk(chunk string) string {
	if chunk == "" {
		return ""
	}
	chunk = strings.ReplaceAll(chunk, "\r\n", "\n")
	chunk = strings.ReplaceAll(chunk, "\r", "\n")
	chunk = ansiEscapeRE.ReplaceAllString(chunk, "")
	// Remove OSC (window title / hyperlinks), comuns em shells modernos.
	for {
		start := strings.Index(chunk, "\x1b]")
		if start < 0 {
			break
		}
		rest := chunk[start+2:]
		endBEL := strings.Index(rest, "\x07")
		endST := strings.Index(rest, "\x1b\\")
		end := -1
		if endBEL >= 0 {
			end = start + 2 + endBEL + 1
		}
		if endST >= 0 {
			cand := start + 2 + endST + 2
			if end < 0 || cand < end {
				end = cand
			}
		}
		if end < 0 {
			chunk = chunk[:start]
			break
		}
		chunk = chunk[:start] + chunk[end:]
	}
	return chunk
}

// decodeTerminalUTF8 junta fragmentos de bytes e evita caracteres quebrados por cortes no meio do rune.
func decodeTerminalUTF8(in []byte, pending []byte) (string, []byte) {
	if len(in) == 0 && len(pending) == 0 {
		return "", pending
	}
	data := append(append([]byte{}, pending...), in...)
	if utf8.Valid(data) {
		return string(data), nil
	}
	// Tenta preservar um sufixo curto para completar no próximo chunk.
	for keep := 1; keep <= 3 && keep < len(data); keep++ {
		prefix := data[:len(data)-keep]
		suffix := data[len(data)-keep:]
		if utf8.Valid(prefix) && !utf8.FullRune(suffix) {
			return string(prefix), append([]byte{}, suffix...)
		}
	}
	// Se ainda houver inválidos, sanitiza para não poluir a tela.
	return string(bytes.ToValidUTF8(data, []byte{})), nil
}

func applyTerminalBackspaces(s string) string {
	if s == "" {
		return s
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\b' || r == 127 {
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// appendSudoDebugLog executa parte da logica deste modulo.
func appendSudoDebugLog(line string) {
	logPath := filepath.Join(os.TempDir(), "containerway-sudo-debug.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " + line + "\n")
}

// formatSudoErrorMessage executa parte da logica deste modulo.
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

// runSSHCommandWithInput executa parte da logica deste modulo.
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

// ensureSudoSession executa parte da logica deste modulo.
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

// copyHostFileWithSudoToLocal executa parte da logica deste modulo.
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
			return errors.New(msg)
		}
	}
	return nil
}

// copyLocalFileToHostWithSudo executa parte da logica deste modulo.
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
			return errors.New(msg)
		}
	}
	return nil
}

// localDirTotalBytes executa parte da logica deste modulo.
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

// copyLocalDirToHostWithSudo executa parte da logica deste modulo.
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
		return 0, errors.New(msg)
	}
	return localDirTotalBytes(localDir), nil
}

// copyHostDirWithSudoToLocal executa parte da logica deste modulo.
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
			return 0, errors.New(msg)
		}
	}

	return written, nil
}

// testSudoAccess executa parte da logica deste modulo.
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

// sudoUID executa parte da logica deste modulo.
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

// listHostWithSudo executa parte da logica deste modulo.
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
		return nil, errors.New(msg)
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

// applyRightFilter executa parte da logica deste modulo.
func (ui *explorer) applyRightFilter() {
	if ui.rightList == nil {
		return
	}
	selectedPath := ""
	if ui.rightSel >= 0 && ui.rightSel < len(ui.rightRows) {
		selectedPath = ui.rightRows[ui.rightSel].Path
	}
	criteria := parseRightFilterCriteria(ui.rightSearch.Text)
	typeNorm := "all"
	if ui.rightTypeFilter != nil {
		typeNorm = normalizeExplorerTypeFilter(ui.rightTypeFilter.Selected)
	}
	if criteria.term == "" && criteria.ext == "" && typeNorm == "all" {
		ui.rightRows = append([]fsutil.DirEntry(nil), ui.rightAll...)
	} else {
		filtered := make([]fsutil.DirEntry, 0, len(ui.rightAll))
		for _, e := range ui.rightAll {
			if e.Name == ".." || rightEntryMatches(e, criteria, typeNorm) {
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
	if criteria.term != "" || criteria.ext != "" || typeNorm != "all" {
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

// parseRightFilterCriteria executa parte da logica deste modulo.
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

// normalizeExplorerTypeFilter converte o rótulo do select (qualquer idioma) para all|dirs|files.
func normalizeExplorerTypeFilter(sel string) string {
	s := strings.ToLower(strings.TrimSpace(sel))
	for _, lang := range []string{langPTBR, langEN, langES} {
		m := explorerStrings[lang]
		if s == strings.ToLower(strings.TrimSpace(m["ex_filter_all"])) {
			return "all"
		}
		if s == strings.ToLower(strings.TrimSpace(m["ex_filter_dirs"])) {
			return "dirs"
		}
		if s == strings.ToLower(strings.TrimSpace(m["ex_filter_files"])) {
			return "files"
		}
	}
	return "all"
}

// rightEntryMatches executa parte da logica deste modulo.
func rightEntryMatches(e fsutil.DirEntry, c rightFilterCriteria, typeNorm string) bool {
	name := strings.ToLower(e.Name)
	if c.term != "" && !strings.Contains(name, c.term) {
		return false
	}
	switch strings.TrimSpace(typeNorm) {
	case "dirs":
		if !e.IsDir {
			return false
		}
	case "files":
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

// goLeftBack executa parte da logica deste modulo.
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

// goRightBack executa parte da logica deste modulo.
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

// goLeftUp executa parte da logica deste modulo.
func (ui *explorer) goLeftUp() {
	if runtime.GOOS == "windows" {
		if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
			return
		}
		parent := filepath.Dir(ui.leftPath)
		if parent == ui.leftPath {
			ui.leftBack = append(ui.leftBack, ui.leftPath)
			ui.leftPath = localfs.WindowsDrivesVirtualPath
			ui.resetLeftSearch()
			ui.refreshLeft()
			if ui.leftQuick != nil {
				ui.leftQuick.SetSelected(tr("sc_disk_drives"))
			}
			return
		}
	}
	parent := filepath.Dir(ui.leftPath)
	if parent == ui.leftPath {
		return
	}
	ui.leftBack = append(ui.leftBack, ui.leftPath)
	ui.leftPath = parent
	ui.resetLeftSearch()
	ui.refreshLeft()
}

// goRightUp executa parte da logica deste modulo.
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

// goLeftHome executa parte da logica deste modulo.
func (ui *explorer) goLeftHome() {
	if ui.leftPath == homeOrRoot() {
		return
	}
	ui.leftBack = append(ui.leftBack, ui.leftPath)
	ui.leftPath = homeOrRoot()
	ui.resetLeftSearch()
	ui.refreshLeft()
}

// goRightHome executa parte da logica deste modulo.
func (ui *explorer) goRightHome() {
	if ui.rightPath == "/" {
		return
	}
	ui.rightBack = append(ui.rightBack, ui.rightPath)
	ui.rightPath = "/"
	ui.resetRightSearch()
	ui.refreshRight()
}

// pushLeftHistory executa parte da logica deste modulo.
func (ui *explorer) pushLeftHistory(next string) {
	if next != "" && next != ui.leftPath {
		ui.leftBack = append(ui.leftBack, ui.leftPath)
	}
}

// pushRightHistory executa parte da logica deste modulo.
func (ui *explorer) pushRightHistory(next string) {
	if next != "" && next != ui.rightPath {
		ui.rightBack = append(ui.rightBack, ui.rightPath)
	}
}

// onLeftActivate executa parte da logica deste modulo.
func (ui *explorer) onLeftActivate() {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_pick_local_dir"), ui.win)
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

// onLeftDoubleAction executa parte da logica deste modulo.
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
		dialog.ShowError(fmt.Errorf(tr("ui_err_open_file"), err), ui.win)
		return
	}
	ui.status.SetText("Arquivo local aberto: " + e.Name)
}

// onRightActivate executa parte da logica deste modulo.
func (ui *explorer) onRightActivate() {
	if ui.rightSel < 0 || ui.rightSel >= len(ui.rightRows) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_pick_remote_dir"), ui.win)
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

// onRightDoubleAction executa parte da logica deste modulo.
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
		tr("dlg_remote_edit_title"),
		fmt.Sprintf(tr("dlg_remote_edit_fmt"), e.Name),
		func(open bool) {
			if !open {
				return
			}
			ui.openRemoteForEdit(e)
		},
		ui.win,
	)
}

// upload executa parte da logica deste modulo.
func (ui *explorer) upload() {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_pick_left_item"), ui.win)
		return
	}
	if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_open_drive_send"), ui.win)
		return
	}
	src := ui.leftRows[ui.leftSel]
	if src.Name == ".." {
		dialog.ShowInformation(tr("app_name"), tr("dlg_pick_valid_item"), ui.win)
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

// download executa parte da logica deste modulo.
func (ui *explorer) download() {
	if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_open_drive_recv"), ui.win)
		return
	}
	if ui.rightSel < 0 || ui.rightSel >= len(ui.rightRows) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_pick_right_item"), ui.win)
		return
	}
	src := ui.rightRows[ui.rightSel]
	if src.Name == ".." {
		dialog.ShowInformation(tr("app_name"), tr("dlg_pick_valid_item"), ui.win)
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

// transferableEntries executa parte da logica deste modulo.
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

// uploadVisibleBatch executa parte da logica deste modulo.
func (ui *explorer) uploadVisibleBatch() {
	if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_batch_send_drive"), ui.win)
		return
	}
	items := transferableEntries(ui.leftRows)
	if len(items) == 0 {
		dialog.ShowInformation(tr("app_name"), tr("dlg_batch_no_local"), ui.win)
		return
	}
	msg := fmt.Sprintf("Enviar %d item(ns) visível(is) para %s?", len(items), ui.rightPath)
	dialog.ShowConfirm(tr("dlg_batch_send_title"), msg, func(ok bool) {
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

// downloadVisibleBatch executa parte da logica deste modulo.
func (ui *explorer) downloadVisibleBatch() {
	if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_batch_recv_drive"), ui.win)
		return
	}
	items := transferableEntries(ui.rightRows)
	if len(items) == 0 {
		dialog.ShowInformation(tr("app_name"), tr("dlg_batch_no_remote"), ui.win)
		return
	}
	msg := fmt.Sprintf("Receber %d item(ns) visível(is) em %s?", len(items), ui.leftPath)
	dialog.ShowConfirm(tr("dlg_batch_recv_title"), msg, func(ok bool) {
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

// beginBatch executa parte da logica deste modulo.
func (ui *explorer) beginBatch(label string, total int) {
	ui.batchMu.Lock()
	defer ui.batchMu.Unlock()
	ui.batchRunning = total > 0
	ui.batchLabel = label
	ui.batchTotal = total
	ui.batchDone = 0
	ui.batchFailures = nil
}

// batchSnapshot executa parte da logica deste modulo.
func (ui *explorer) batchSnapshot() (running bool, done int, total int) {
	ui.batchMu.Lock()
	defer ui.batchMu.Unlock()
	return ui.batchRunning, ui.batchDone, ui.batchTotal
}

// consumeBatchResult executa parte da logica deste modulo.
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

// enqueueLocalToRemote executa parte da logica deste modulo.
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
				if st, err := ui.hfs.Stat(dst); err == nil && !st.IsDir() && st.Size() == src.Size {
					appendAuditLog("transfer", fmt.Sprintf("Omitido upload (destino já existe com mesmo tamanho): %s", dst))
					return nil
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

// enqueueRemoteToLocal executa parte da logica deste modulo.
func (ui *explorer) enqueueRemoteToLocal(src copiedItem, dstDir string) {
	if localfs.IsWindowsDrivesVirtual(strings.TrimSpace(dstDir)) {
		dialog.ShowInformation(tr("app_name"), tr("dlg_recv_here_drive"), ui.win)
		return
	}
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

// enqueueRemoteToRemote executa parte da logica deste modulo.
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

// startDrain executa parte da logica deste modulo.
func (ui *explorer) startDrain() {
	ctx := context.Background()
	ui.tm.DrainAsync(ctx, ui.parallelJobs,
		func(j transfer.Job) {
			fyne.Do(func() {
				appendAuditLog("transfer", "Início | "+j.Name)
				ui.progress.Show()
				batchRunning, batchDone, batchTotal := ui.batchSnapshot()
				if batchRunning && batchTotal > 0 {
					ui.progress.SetValue(float64(batchDone) / float64(batchTotal))
				} else {
					ui.progress.SetValue(0)
				}
				ui.lastJobText.SetText(fmt.Sprintf("[fila:%d exec:%d] %s", ui.tm.Queued(), ui.tm.Running(), j.Name))
				ui.status.SetText("Transferindo…")
			})
		},
		func(j transfer.Job, err error) {
			fyne.Do(func() {
				batchActive, batchFinished, batchDone, batchTotal, batchProgress, batchSummary, batchErr := ui.consumeBatchResult(j, err)
				if batchActive {
					if err != nil {
						ui.rememberFailedJob(j)
						appendAuditLog("transfer", fmt.Sprintf("Falha | %s | %v", j.Name, err))
					}
					ui.progress.Show()
					if batchTotal > 0 {
						ui.progress.SetValue(float64(batchDone) / float64(batchTotal))
					}
					if batchFinished {
						ui.progress.Hide()
						if batchErr != nil {
							ui.status.SetText(batchErr.Error())
							ui.appendOperationHistory("Lote com falha: " + batchErr.Error())
							dialog.ShowError(batchErr, ui.win)
						} else {
							ui.status.SetText(batchSummary)
							ui.appendOperationHistory(batchSummary)
							appendAuditLog("transfer", "Concluído | "+batchSummary)
							dialog.ShowInformation(tr("dlg_transfer_batch_done"), batchSummary, ui.win)
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
					ui.rememberFailedJob(j)
					appendAuditLog("transfer", fmt.Sprintf("Falha | %s | %v", j.Name, err))
					ui.status.SetText(fmt.Sprintf("Erro: %v", err))
					ui.appendOperationHistory(fmt.Sprintf("Falha: %s | %v", j.Name, err))
					dialog.ShowError(err, ui.win)
				} else {
					appendAuditLog("transfer", "Concluído | "+j.Name)
					ui.status.SetText("Concluído: " + j.Name)
					ui.appendOperationHistory("Concluído: " + j.Name)
					dialog.ShowInformation(tr("dlg_transfer_done"), j.Name, ui.win)
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

// selectedLeftEntry executa parte da logica deste modulo.
func (ui *explorer) selectedLeftEntry() (fsutil.DirEntry, bool) {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		return fsutil.DirEntry{}, false
	}
	return ui.leftRows[ui.leftSel], true
}

// selectedRightEntry executa parte da logica deste modulo.
func (ui *explorer) selectedRightEntry() (fsutil.DirEntry, bool) {
	if ui.rightSel < 0 || ui.rightSel >= len(ui.rightRows) {
		return fsutil.DirEntry{}, false
	}
	return ui.rightRows[ui.rightSel], true
}

// updateActionState executa parte da logica deste modulo.
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

// entryTypeLabel executa parte da logica deste modulo.
func entryTypeLabel(e fsutil.DirEntry) string {
	if e.IsDir {
		return tr("ex_type_dir")
	}
	return tr("ex_type_file")
}

// summarizeEntries executa parte da logica deste modulo.
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

// updateSummaryInfo executa parte da logica deste modulo.
func (ui *explorer) updateSummaryInfo() {
	ui.updateFooterPanels()
}

// updateFooterPanels executa parte da logica deste modulo.
func (ui *explorer) updateFooterPanels() {
	if ui.leftFooterInfo == nil || ui.rightFooterInfo == nil {
		return
	}
	leftDirs, leftFiles := summarizeEntries(ui.leftRows)
	rightDirs, rightFiles := summarizeEntries(ui.rightRows)

	leftSelText := tr("ex_foot_sel_none")
	if left, ok := ui.selectedLeftEntry(); ok {
		leftActionHint := tr("ex_foot_act_open_send")
		if left.Name == ".." {
			leftActionHint = tr("ex_foot_act_parent_send")
		}
		leftSelText = fmt.Sprintf(tr("ex_foot_sel_fmt"), left.Name, entryTypeLabel(left), leftActionHint)
	} else {
		leftSelText = tr("ex_foot_sel_none_left")
	}
	rightSelText := tr("ex_foot_sel_none")
	if right, ok := ui.selectedRightEntry(); ok {
		rightActionHint := tr("ex_foot_act_open_recv")
		if right.Name == ".." {
			rightActionHint = tr("ex_foot_act_parent_recv")
		}
		rightSelText = fmt.Sprintf(tr("ex_foot_sel_fmt"), right.Name, entryTypeLabel(right), rightActionHint)
	} else {
		rightSelText = tr("ex_foot_sel_none_right")
	}

	rightPathLabel := fmt.Sprintf(tr("ex_foot_path_server"), ui.rightPath)
	if !ui.hostMode && ui.cfs != nil {
		short := strings.TrimPrefix(ui.cfs.ID, "sha256:")
		if len(short) > 12 {
			short = short[:12]
		}
		rightPathLabel = fmt.Sprintf(tr("ex_foot_path_container"), short, ui.rightPath)
	}

	countsLeft := fmt.Sprintf(tr("ex_foot_items"), leftDirs, leftFiles)
	countsRight := fmt.Sprintf(tr("ex_foot_items"), rightDirs, rightFiles)
	ui.leftFooterInfo.SetText(fmt.Sprintf(
		tr("ex_foot_local_full_fmt"),
		leftPathFooterLabel(ui.leftPath),
		countsLeft,
		leftSelText,
	))
	ui.rightFooterInfo.SetText(fmt.Sprintf(
		tr("ex_foot_remote_full_fmt"),
		rightPathLabel,
		countsRight,
		rightSelText,
	))
}

// registerExplorerShortcuts executa parte da logica deste modulo.
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
		if !ui.explorerOnTop.Load() {
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
		if !ui.explorerOnTop.Load() {
			return
		}
		ui.focusActiveSearch()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(fyne.Shortcut) {
		if !ui.explorerOnTop.Load() {
			return
		}
		ui.createFolderActive()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF6, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(fyne.Shortcut) {
		if !ui.explorerOnTop.Load() {
			return
		}
		if ui.activePane == "left" {
			ui.uploadVisibleBatch()
		} else {
			ui.downloadVisibleBatch()
		}
	})
}

// triggerDialogConfirm executa parte da logica deste modulo.
func (ui *explorer) triggerDialogConfirm() {
	if !ui.dialogShortcutActive.Load() {
		return
	}
	if ui.dialogConfirmAction != nil {
		ui.dialogConfirmAction()
	}
}

// triggerDialogCancel executa parte da logica deste modulo.
func (ui *explorer) triggerDialogCancel() {
	if !ui.dialogShortcutActive.Load() {
		return
	}
	if ui.dialogCancelAction != nil {
		ui.dialogCancelAction()
	}
}

// openFormDialogWithShortcuts executa parte da logica deste modulo.
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

// focusActiveSearch executa parte da logica deste modulo.
func (ui *explorer) focusActiveSearch() {
	if ui.activePane == "left" {
		ui.win.Canvas().Focus(ui.leftSearch)
		return
	}
	ui.win.Canvas().Focus(ui.rightSearch)
}

// resetLeftSearch executa parte da logica deste modulo.
func (ui *explorer) resetLeftSearch() {
	if ui.leftSearch != nil && strings.TrimSpace(ui.leftSearch.Text) != "" {
		ui.leftSearch.SetText("")
	}
	if ui.leftTypeFilter != nil && normalizeExplorerTypeFilter(ui.leftTypeFilter.Selected) != "all" {
		ui.leftTypeFilter.SetSelected(tr("ex_filter_all"))
	}
}

// resetRightSearch executa parte da logica deste modulo.
func (ui *explorer) resetRightSearch() {
	if ui.rightSearch != nil && strings.TrimSpace(ui.rightSearch.Text) != "" {
		ui.rightSearch.SetText("")
	}
	if ui.rightTypeFilter != nil && normalizeExplorerTypeFilter(ui.rightTypeFilter.Selected) != "all" {
		ui.rightTypeFilter.SetSelected(tr("ex_filter_all"))
	}
}

// renameActive executa parte da logica deste modulo.
func (ui *explorer) renameActive() {
	if ui.activePane == "left" {
		e, ok := ui.selectedLeftEntry()
		if !ok || e.Name == ".." {
		dialog.ShowInformation(tr("dlg_rename_title"), tr("dlg_rename_pick_local"), ui.win)
			return
		}
		if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
			dialog.ShowInformation(tr("dlg_rename_title"), tr("dlg_rename_drive_letter"), ui.win)
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
					dialog.ShowError(fmt.Errorf(tr("ui_err_rename_local"), err), ui.win)
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
		dialog.ShowInformation(tr("dlg_rename_title"), tr("dlg_rename_pick_remote"), ui.win)
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
				dialog.ShowError(fmt.Errorf(tr("ui_err_rename_remote"), err), ui.win)
				return
			}
			ui.status.SetText("Item remoto renomeado com sucesso.")
			ui.refreshRight()
		},
		nil,
	)
}

// deleteActive executa parte da logica deste modulo.
func (ui *explorer) deleteActive() {
	if ui.activePane == "left" {
		e, ok := ui.selectedLeftEntry()
		if !ok || e.Name == ".." {
			dialog.ShowInformation(tr("dlg_delete_title"), tr("dlg_rename_pick_local"), ui.win)
			return
		}
		if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
			dialog.ShowInformation(tr("dlg_delete_title"), tr("dlg_delete_drive"), ui.win)
			return
		}
		msg := fmt.Sprintf("Deseja excluir \"%s\" do computador local?", e.Name)
		dialog.ShowConfirm(tr("dlg_confirm_delete"), msg, func(confirm bool) {
			if !confirm {
				ui.status.SetText("Exclusão cancelada.")
				return
			}
			if err := localfs.Remove(e.Path, e.IsDir); err != nil {
				dialog.ShowError(fmt.Errorf(tr("ui_err_delete_local"), err), ui.win)
				return
			}
			ui.status.SetText("Item local excluído com sucesso.")
			ui.refreshLeft()
		}, ui.win)
		return
	}
	e, ok := ui.selectedRightEntry()
	if !ok || e.Name == ".." {
		dialog.ShowInformation(tr("dlg_delete_title"), tr("dlg_rename_pick_remote"), ui.win)
		return
	}
	msg := fmt.Sprintf("Deseja excluir \"%s\" do servidor?", e.Name)
	dialog.ShowConfirm(tr("dlg_confirm_delete"), msg, func(confirm bool) {
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
			dialog.ShowError(fmt.Errorf(tr("ui_err_delete_remote"), err), ui.win)
			return
		}
		ui.status.SetText("Item remoto excluído com sucesso.")
		ui.refreshRight()
	}, ui.win)
}

// createFolderActive executa parte da logica deste modulo.
func (ui *explorer) createFolderActive() {
	name := widget.NewEntry()
	name.SetPlaceHolder("Digite o nome da pasta")
	title := "Nova pasta (local)"
	if ui.activePane == "right" {
		title = "Nova pasta (servidor)"
	}
	dialogSize := fyne.NewSize(520, 220)
	ui.openFormDialogWithShortcuts(
		title,
		"Criar",
		"Cancelar",
		dialogSize,
		[]*widget.FormItem{
			widget.NewFormItem("Nome da pasta", name),
		},
		func() {
			folderName := strings.TrimSpace(name.Text)
			if folderName == "" {
				return
			}
			if ui.activePane == "left" {
				if localfs.IsWindowsDrivesVirtual(ui.leftPath) {
					dialog.ShowInformation(tr("dlg_newfolder_title"), tr("dlg_newfolder_drive"), ui.win)
					return
				}
				target := filepath.Join(ui.leftPath, folderName)
				if err := localfs.Mkdir(target); err != nil {
					dialog.ShowError(fmt.Errorf(tr("ui_err_mkdir_local"), err), ui.win)
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
				dialog.ShowError(fmt.Errorf(tr("ui_err_mkdir_remote"), err), ui.win)
				return
			}
			ui.status.SetText("Pasta remota criada com sucesso.")
			ui.refreshRight()
		},
		nil,
	)
}

// makePathButtons executa parte da logica deste modulo.
func (ui *explorer) makePathButtons(p string, left bool) []fyne.CanvasObject {
	if left {
		if localfs.IsWindowsDrivesVirtual(strings.TrimSpace(p)) {
			btn := widget.NewButton("Unidades de disco", func() {
				ui.pushLeftHistory(p)
				ui.leftPath = localfs.WindowsDrivesVirtualPath
				ui.resetLeftSearch()
				ui.refreshLeft()
			})
			btn.Importance = widget.LowImportance
			return []fyne.CanvasObject{btn}
		}
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

// defaultLocalShortcuts executa parte da logica deste modulo.
func (ui *explorer) defaultLocalShortcuts() []string {
	base := []string{tr("sc_home"), tr("sc_desktop"), tr("sc_documents"), tr("sc_downloads")}
	if runtime.GOOS == "windows" {
		base = append(base, tr("sc_disk_drives"))
	}
	return base
}

// defaultRemoteShortcuts executa parte da logica deste modulo.
func defaultRemoteShortcuts() []string {
	return []string{"/", "/home", "/opt", "/var", "/tmp"}
}

// uniqueNonEmpty executa parte da logica deste modulo.
func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}

// loadStringSlicePreference executa parte da logica deste modulo.
func loadStringSlicePreference(key string) []string {
	app := fyne.CurrentApp()
	if app == nil {
		return nil
	}
	raw := strings.TrimSpace(app.Preferences().StringWithFallback(key, ""))
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return uniqueNonEmpty(values)
}

// saveStringSlicePreference executa parte da logica deste modulo.
func saveStringSlicePreference(key string, values []string) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	clean := uniqueNonEmpty(values)
	if len(clean) == 0 {
		app.Preferences().SetString(key, "")
		return
	}
	buf, err := json.Marshal(clean)
	if err != nil {
		return
	}
	app.Preferences().SetString(key, string(buf))
}

// loadOperationHistoryPreference executa parte da logica deste modulo.
func loadOperationHistoryPreference() []string {
	return loadStringSlicePreference(operationHistoryPreferenceKey)
}

// saveOperationHistoryPreference executa parte da logica deste modulo.
func saveOperationHistoryPreference(values []string) {
	saveStringSlicePreference(operationHistoryPreferenceKey, values)
}

// loadTerminalCommandFavorites executa parte da logica deste modulo.
func loadTerminalCommandFavorites() []string {
	return loadStringSlicePreference(terminalCommandFavoritesKey)
}

// saveTerminalCommandFavorites executa parte da logica deste modulo.
func saveTerminalCommandFavorites(values []string) {
	saveStringSlicePreference(terminalCommandFavoritesKey, values)
}

// defaultAccessAccounts executa parte da logica deste modulo.
func defaultAccessAccounts() []accessUser {
	return []accessUser{
		{
			Username:    defaultAccessUser,
			Password:    defaultAccessPass,
			DisplayName: "Administrador",
		},
	}
}

// normalizeAccessUsername executa parte da logica deste modulo.
func normalizeAccessUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// loadAccessAccounts executa parte da logica deste modulo.
func loadAccessAccounts() []accessUser {
	app := fyne.CurrentApp()
	if app == nil {
		return defaultAccessAccounts()
	}
	raw := strings.TrimSpace(app.Preferences().StringWithFallback(accessUsersPreferenceKey, ""))
	if raw == "" {
		return defaultAccessAccounts()
	}
	var users []accessUser
	if err := json.Unmarshal([]byte(raw), &users); err != nil {
		return defaultAccessAccounts()
	}
	seen := map[string]struct{}{}
	out := make([]accessUser, 0, len(users)+1)
	for _, u := range users {
		name := normalizeAccessUsername(u.Username)
		pass := strings.TrimSpace(u.Password)
		if name == "" || pass == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		display := strings.TrimSpace(u.DisplayName)
		if display == "" {
			display = u.Username
		}
		out = append(out, accessUser{Username: name, Password: pass, DisplayName: display})
	}
	adminName := normalizeAccessUsername(defaultAccessUser)
	if _, ok := seen[adminName]; !ok {
		out = append(out, defaultAccessAccounts()...)
	} else {
		for i := range out {
			if out[i].Username == adminName {
				out[i].Password = defaultAccessPass
				if strings.TrimSpace(out[i].DisplayName) == "" {
					out[i].DisplayName = "Administrador"
				}
				break
			}
		}
	}
	return out
}

// saveAccessAccounts executa parte da logica deste modulo.
func saveAccessAccounts(users []accessUser) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	normalized := make([]accessUser, 0, len(users))
	seen := map[string]struct{}{}
	for _, u := range users {
		name := normalizeAccessUsername(u.Username)
		pass := strings.TrimSpace(u.Password)
		if name == "" || pass == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		display := strings.TrimSpace(u.DisplayName)
		if display == "" {
			display = u.Username
		}
		normalized = append(normalized, accessUser{Username: name, Password: pass, DisplayName: display})
	}
	if _, ok := seen[normalizeAccessUsername(defaultAccessUser)]; !ok {
		normalized = append(normalized, defaultAccessAccounts()...)
	}
	buf, err := json.Marshal(normalized)
	if err != nil {
		return
	}
	app.Preferences().SetString(accessUsersPreferenceKey, string(buf))
}

// findAccessAccount executa parte da logica deste modulo.
func findAccessAccount(users []accessUser, username string) (accessUser, bool) {
	key := normalizeAccessUsername(username)
	for _, u := range users {
		if normalizeAccessUsername(u.Username) == key {
			return u, true
		}
	}
	return accessUser{}, false
}

// upsertAccessAccount executa parte da logica deste modulo.
func upsertAccessAccount(users []accessUser, user accessUser) []accessUser {
	key := normalizeAccessUsername(user.Username)
	if key == "" || strings.TrimSpace(user.Password) == "" {
		return users
	}
	user.Username = key
	if strings.TrimSpace(user.DisplayName) == "" {
		user.DisplayName = user.Username
	}
	for i := range users {
		if normalizeAccessUsername(users[i].Username) == key {
			users[i] = user
			return users
		}
	}
	return append(users, user)
}

// setAuditActor executa parte da logica deste modulo.
func setAuditActor(name string) {
	auditActorMu.Lock()
	defer auditActorMu.Unlock()
	clean := strings.TrimSpace(name)
	if clean == "" {
		clean = "desconhecido"
	}
	auditActorName = clean
}

// setCurrentAccessUser executa parte da logica deste modulo.
func setCurrentAccessUser(username string) {
	accessUserMu.Lock()
	defer accessUserMu.Unlock()
	currentAccessUserName = normalizeAccessUsername(username)
}

// currentAccessUser executa parte da logica deste modulo.
func currentAccessUser() string {
	accessUserMu.Lock()
	defer accessUserMu.Unlock()
	return currentAccessUserName
}

// isCurrentAccessAdmin executa parte da logica deste modulo.
func isCurrentAccessAdmin() bool {
	return currentAccessUser() == normalizeAccessUsername(defaultAccessUser)
}

// currentAuditActor executa parte da logica deste modulo.
func currentAuditActor() string {
	auditActorMu.Lock()
	defer auditActorMu.Unlock()
	return auditActorName
}

// auditLogPath executa parte da logica deste modulo.
func auditLogPath() string {
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "ContainerWay")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, auditLogFileName)
}

// resetSessionAuditBuffer executa parte da logica deste modulo.
func resetSessionAuditBuffer() {
	sessionAuditMu.Lock()
	sessionAuditLines = nil
	sessionAuditMu.Unlock()
}

// appendSessionAuditLine executa parte da logica deste modulo.
func appendSessionAuditLine(line string) {
	if currentAccessUser() == "" {
		return
	}
	t := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
	if t == "" {
		return
	}
	sessionAuditMu.Lock()
	sessionAuditLines = append(sessionAuditLines, t)
	sessionAuditMu.Unlock()
}

// snapshotSessionAuditLines executa parte da logica deste modulo.
func snapshotSessionAuditLines() []string {
	sessionAuditMu.Lock()
	defer sessionAuditMu.Unlock()
	out := make([]string, len(sessionAuditLines))
	copy(out, sessionAuditLines)
	return out
}

// loadMailRecipientListFromPrefs executa parte da logica deste modulo.
func loadMailRecipientListFromPrefs(p fyne.Preferences) []string {
	raw := strings.TrimSpace(p.String(notifyRecipientsJSONPreferenceKey))
	if raw != "" {
		var emails []string
		if err := json.Unmarshal([]byte(raw), &emails); err == nil {
			// Lista vazia em JSON (ex.: "[]") é válida: deve retornar vazio e não cair no legado
			// notify.email.to, que pode ainda guardar o primeiro endereço antigo.
			return mailnotify.NormalizeRecipients(emails)
		}
	}
	legacy := strings.TrimSpace(p.String(notifyRecipientPreferenceKey))
	if legacy == "" {
		return nil
	}
	var parts []string
	for _, seg := range strings.Split(legacy, ",") {
		if t := strings.TrimSpace(seg); t != "" {
			parts = append(parts, t)
		}
	}
	return mailnotify.NormalizeRecipients(parts)
}

// loadMailNotifySettings executa parte da logica deste modulo.
func loadMailNotifySettings() mailnotify.Settings {
	a := fyne.CurrentApp()
	if a == nil {
		return mailnotify.Settings{}
	}
	p := a.Preferences()
	port, _ := strconv.Atoi(strings.TrimSpace(p.StringWithFallback(notifySMTPPortPreferenceKey, "587")))
	if port <= 0 {
		port = 587
	}
	return mailnotify.Settings{
		Enabled:    strings.EqualFold(strings.TrimSpace(p.StringWithFallback(notifyEnabledPreferenceKey, "")), "true"),
		Host:       strings.TrimSpace(p.String(notifySMTPHostPreferenceKey)),
		Port:       port,
		User:       strings.TrimSpace(p.String(notifySMTPUserPreferenceKey)),
		Password:   p.String(notifySMTPPasswordPreferenceKey),
		From:       strings.TrimSpace(p.String(notifySMTPFromPreferenceKey)),
		Recipients: loadMailRecipientListFromPrefs(p),
	}
}

// saveMailNotifySettings executa parte da logica deste modulo.
func saveMailNotifySettings(s mailnotify.Settings) {
	a := fyne.CurrentApp()
	if a == nil {
		return
	}
	p := a.Preferences()
	if s.Enabled {
		p.SetString(notifyEnabledPreferenceKey, "true")
	} else {
		p.SetString(notifyEnabledPreferenceKey, "false")
	}
	rec := mailnotify.NormalizeRecipients(s.Recipients)
	if buf, err := json.Marshal(rec); err == nil {
		p.SetString(notifyRecipientsJSONPreferenceKey, string(buf))
	}
	if len(rec) > 0 {
		p.SetString(notifyRecipientPreferenceKey, rec[0])
	} else {
		p.SetString(notifyRecipientPreferenceKey, "")
	}
	p.SetString(notifySMTPHostPreferenceKey, strings.TrimSpace(s.Host))
	p.SetString(notifySMTPPortPreferenceKey, strconv.Itoa(s.Port))
	p.SetString(notifySMTPUserPreferenceKey, strings.TrimSpace(s.User))
	p.SetString(notifySMTPPasswordPreferenceKey, s.Password)
	p.SetString(notifySMTPFromPreferenceKey, strings.TrimSpace(s.From))
}

// copyMailNotifySettingsForAsync executa parte da logica deste modulo.
func copyMailNotifySettingsForAsync(s mailnotify.Settings) mailnotify.Settings {
	out := s
	out.Recipients = append([]string(nil), s.Recipients...)
	return out
}

// sendNotifyLoginEmailWithConfig executa parte da logica deste modulo.
func sendNotifyLoginEmailWithConfig(cfg mailnotify.Settings, acc accessUser) {
	if !cfg.Valid() {
		return
	}
	host, _ := os.Hostname()
	body := fmt.Sprintf(
		"Um usuário acabou de entrar no ContainerWay neste computador.\n\n"+
			"Conta (login): %s\n"+
			"Nome exibido: %s\n"+
			"Computador: %s\n"+
			"Data/hora: %s\n",
		acc.Username,
		strings.TrimSpace(acc.DisplayName),
		host,
		time.Now().Format(time.RFC3339),
	)
	if err := cfg.Send("ContainerWay: login no aplicativo", body); err != nil {
		appendAuditLog("email", "Falha ao enviar aviso de login: "+err.Error())
	}
}

// clipSessionLinesForEmail executa parte da logica deste modulo.
func clipSessionLinesForEmail(lines []string, maxLines int) ([]string, bool) {
	if maxLines < 1 {
		maxLines = sessionEmailMaxLines
	}
	if len(lines) <= maxLines {
		return lines, false
	}
	return lines[len(lines)-maxLines:], true
}

// sendNotifySessionEndWithConfig executa parte da logica deste modulo.
func sendNotifySessionEndWithConfig(cfg mailnotify.Settings, lines []string) {
	if !cfg.Valid() {
		return
	}
	host, _ := os.Hostname()
	lines, clipped := clipSessionLinesForEmail(lines, sessionEmailMaxLines)
	body := fmt.Sprintf(
		"Sessão do ContainerWay encerrada neste computador.\n\nComputador: %s\nData/hora: %s\n\n",
		host,
		time.Now().Format(time.RFC3339),
	)
	if clipped {
		body += fmt.Sprintf(
			"[Atenção: o e-mail inclui só as últimas %d linhas do registro da sessão; o arquivo completo continua no log de atividades do aplicativo.]\n\n",
			sessionEmailMaxLines,
		)
	}
	body += "--- Registro desta sessão ---\n"
	if len(lines) == 0 {
		body += "(Nenhuma linha adicional no registro da sessão.)\n"
	} else {
		body += strings.Join(lines, "\n") + "\n"
	}
	if err := cfg.Send("ContainerWay: fim de sessão e registro de atividades", body); err != nil {
		appendAuditLog("email", "Falha ao enviar resumo de sessão por e-mail: "+err.Error())
		return
	}
	appendAuditLog("email", "Resumo de sessão enviado por e-mail aos destinatários configurados")
}

// finalizeLocalAccessSession executa parte da logica deste modulo.
func finalizeLocalAccessSession(w fyne.Window, s *session.Session, endMsg string) {
	appendAuditLog("sessao", endMsg)
	lines := snapshotSessionAuditLines()
	cfg := loadMailNotifySettings()
	if !cfg.Valid() {
		appendAuditLog("email", "Resumo de sessão por e-mail não enviado: ative os alertas, cadastre destinatários e configure o SMTP.")
	} else {
		// Envio síncrono na thread da UI: evita a goroutine terminar depois que a janela já mudou
		// e garante que o SMTP conclua antes de fechar a sessão SSH.
		sendNotifySessionEndWithConfig(copyMailNotifySettingsForAsync(cfg), append([]string(nil), lines...))
	}
	if s != nil {
		s.Close()
	}
	setAuditActor("desconhecido")
	setCurrentAccessUser("")
	goToLogin(w)
}

// appendAuditLog executa parte da logica deste modulo.
func appendAuditLog(scope, message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	level := "INFO"
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "erro") || strings.Contains(lower, "falha") {
		level = "ERROR"
	}
	line := fmt.Sprintf(
		"%s | %s | %s | operador=%s | %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		level,
		strings.TrimSpace(scope),
		currentAuditActor(),
		msg,
	)
	appendSessionAuditLine(line)
	auditLogMu.Lock()
	defer auditLogMu.Unlock()
	f, err := os.OpenFile(auditLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// readAuditLogLines executa parte da logica deste modulo.
func readAuditLogLines(maxLines int, filter string) []string {
	if maxLines < 1 {
		maxLines = 1
	}
	raw, err := os.ReadFile(auditLogPath())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	term := strings.ToLower(strings.TrimSpace(filter))
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if term != "" && !strings.Contains(strings.ToLower(t), term) {
			continue
		}
		filtered = append(filtered, t)
	}
	if len(filtered) > maxLines {
		filtered = filtered[len(filtered)-maxLines:]
	}
	return filtered
}

// remoteFavoritesPreferenceKey devolve chave de preferências para atalhos do painel direito (host + contexto).
func (ui *explorer) remoteFavoritesPreferenceKey() string {
	h := strings.TrimSpace(ui.connCreds.Host)
	if h == "" {
		h = "host"
	}
	h = strings.ToLower(h)
	h = strings.ReplaceAll(h, ":", "_")
	h = strings.ReplaceAll(h, "/", "_")
	h = strings.ReplaceAll(h, "\\", "_")
	h = strings.ReplaceAll(h, " ", "_")
	if ui.hostMode {
		return rightFavoritesPreferenceKey + "." + h + ".host"
	}
	cid := "ctx"
	if ui.cfs != nil {
		cid = strings.TrimPrefix(ui.cfs.ID, "sha256:")
		if len(cid) > 16 {
			cid = cid[:16]
		}
	}
	return rightFavoritesPreferenceKey + "." + h + ".c." + cid
}

// localShortcutOptions executa parte da logica deste modulo.
func (ui *explorer) localShortcutOptions() []string {
	base := ui.defaultLocalShortcuts()
	saved := loadStringSlicePreference(leftFavoritesPreferenceKey)
	return uniqueNonEmpty(append(base, saved...))
}

// remoteShortcutOptions executa parte da logica deste modulo.
func (ui *explorer) remoteShortcutOptions() []string {
	base := defaultRemoteShortcuts()
	key := ui.remoteFavoritesPreferenceKey()
	saved := uniqueNonEmpty(append(loadStringSlicePreference(key), loadStringSlicePreference(rightFavoritesPreferenceKey)...))
	return uniqueNonEmpty(append(base, saved...))
}

// leftQuickSelectLabelForPath devolve o texto que corresponde ao caminho no select de atalhos locais.
func leftQuickSelectLabelForPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if runtime.GOOS == "windows" && localfs.IsWindowsDrivesVirtual(p) {
		return tr("sc_disk_drives")
	}
	home := homeOrRoot()
	switch p {
	case home:
		return tr("sc_home")
	case filepath.Join(home, "Desktop"):
		return tr("sc_desktop")
	case filepath.Join(home, "Documents"):
		return tr("sc_documents")
	case filepath.Join(home, "Downloads"):
		return tr("sc_downloads")
	default:
		return p
	}
}

// refreshLeftShortcutOptions executa parte da logica deste modulo.
func (ui *explorer) refreshLeftShortcutOptions(selectPath string) {
	if ui.leftQuick == nil {
		return
	}
	ui.leftQuick.Options = ui.localShortcutOptions()
	ui.leftQuick.Refresh()
	if strings.TrimSpace(selectPath) != "" {
		ui.leftQuick.SetSelected(selectPath)
	}
}

// refreshRightShortcutOptions executa parte da logica deste modulo.
func (ui *explorer) refreshRightShortcutOptions(selectPath string) {
	if ui.rightQuick == nil {
		return
	}
	ui.rightQuick.Options = ui.remoteShortcutOptions()
	ui.rightQuick.Refresh()
	if strings.TrimSpace(selectPath) != "" {
		ui.rightQuick.SetSelected(selectPath)
	}
}

// addLeftFavoriteCurrentPath executa parte da logica deste modulo.
func (ui *explorer) addLeftFavoriteCurrentPath() {
	p := strings.TrimSpace(ui.leftPath)
	if p == "" {
		return
	}
	if localfs.IsWindowsDrivesVirtual(p) {
		ui.status.SetText("Abra uma unidade (ex.: D:\\) para salvar atalho; a lista de unidades não pode ser favorita.")
		return
	}
	if _, builtIn := ui.resolveLocalShortcut(p); builtIn {
		ui.refreshLeftShortcutOptions(p)
		ui.status.SetText("Atalho local já disponível.")
		return
	}
	saved := append(loadStringSlicePreference(leftFavoritesPreferenceKey), p)
	saveStringSlicePreference(leftFavoritesPreferenceKey, saved)
	ui.refreshLeftShortcutOptions(p)
	ui.status.SetText("Atalho local salvo: " + p)
	appendAuditLog("favoritos", "Atalho local salvo: "+p)
}

// addRightFavoriteCurrentPath executa parte da logica deste modulo.
func (ui *explorer) addRightFavoriteCurrentPath() {
	p := strings.TrimSpace(ui.rightPath)
	if p == "" {
		return
	}
	key := ui.remoteFavoritesPreferenceKey()
	saved := append(loadStringSlicePreference(key), p)
	saveStringSlicePreference(key, uniqueNonEmpty(saved))
	ui.refreshRightShortcutOptions(p)
	ui.status.SetText("Atalho do servidor salvo: " + p)
	appendAuditLog("favoritos", "Atalho remoto salvo: "+p)
}

// removePathFromList executa parte da logica deste modulo.
func removePathFromList(values []string, target string) []string {
	want := strings.TrimSpace(strings.ToLower(target))
	if want == "" {
		return uniqueNonEmpty(values)
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.ToLower(strings.TrimSpace(v)) == want {
			continue
		}
		out = append(out, v)
	}
	return uniqueNonEmpty(out)
}

// removeLeftFavoriteCurrentPath executa parte da logica deste modulo.
func (ui *explorer) removeLeftFavoriteCurrentPath() {
	p := strings.TrimSpace(ui.leftPath)
	if p == "" {
		return
	}
	saved := loadStringSlicePreference(leftFavoritesPreferenceKey)
	next := removePathFromList(saved, p)
	if len(next) == len(saved) {
		ui.status.SetText("Atalho local não está na lista de favoritos.")
		return
	}
	saveStringSlicePreference(leftFavoritesPreferenceKey, next)
	ui.refreshLeftShortcutOptions("")
	ui.status.SetText("Atalho local removido: " + p)
	appendAuditLog("favoritos", "Atalho local removido: "+p)
}

// removeRightFavoriteCurrentPath executa parte da logica deste modulo.
func (ui *explorer) removeRightFavoriteCurrentPath() {
	p := strings.TrimSpace(ui.rightPath)
	if p == "" {
		return
	}
	key := ui.remoteFavoritesPreferenceKey()
	saved := loadStringSlicePreference(key)
	next := removePathFromList(saved, p)
	if len(next) == len(saved) {
		legacy := loadStringSlicePreference(rightFavoritesPreferenceKey)
		nextL := removePathFromList(legacy, p)
		if len(nextL) == len(legacy) {
			ui.status.SetText("Atalho do servidor não está na lista de favoritos.")
			return
		}
		saveStringSlicePreference(rightFavoritesPreferenceKey, nextL)
		ui.refreshRightShortcutOptions("")
		ui.status.SetText("Atalho removido (lista global legada): " + p)
		appendAuditLog("favoritos", "Atalho remoto removido (global): "+p)
		return
	}
	saveStringSlicePreference(key, next)
	ui.refreshRightShortcutOptions("")
	ui.status.SetText("Atalho do servidor removido: " + p)
	appendAuditLog("favoritos", "Atalho remoto removido: "+p)
}

// localeExplorerShortcutMatch reconhece rótulos de atalho local em qualquer idioma suportado.
func localeExplorerShortcutMatch(sel, key string) bool {
	sel = strings.TrimSpace(sel)
	if sel == tr(key) {
		return true
	}
	for _, lang := range []string{langPTBR, langEN, langES} {
		if m := explorerStrings[lang]; m != nil && sel == m[key] {
			return true
		}
	}
	return false
}

// resolveLocalShortcut executa parte da logica deste modulo.
func (ui *explorer) resolveLocalShortcut(sel string) (string, bool) {
	sel = strings.TrimSpace(sel)
	if localeExplorerShortcutMatch(sel, "sc_disk_drives") && runtime.GOOS == "windows" {
		return localfs.WindowsDrivesVirtualPath, true
	}
	home := homeOrRoot()
	if localeExplorerShortcutMatch(sel, "sc_home") {
		return home, true
	}
	if localeExplorerShortcutMatch(sel, "sc_desktop") {
		return filepath.Join(home, "Desktop"), true
	}
	if localeExplorerShortcutMatch(sel, "sc_documents") {
		return filepath.Join(home, "Documents"), true
	}
	if localeExplorerShortcutMatch(sel, "sc_downloads") {
		return filepath.Join(home, "Downloads"), true
	}
	if sel != "" && localfs.IsWindowsDrivesVirtual(sel) {
		return localfs.WindowsDrivesVirtualPath, true
	}
	if sel != "" {
		return sel, true
	}
	return "", false
}

// showRowContextMenu executa parte da logica deste modulo.
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
			fyne.NewMenuItem(tr("ctx_left_open"), func() { ui.onLeftActivate() }),
			fyne.NewMenuItem(tr("ctx_left_send_srv"), func() { ui.upload() }),
			fyne.NewMenuItem(tr("ctx_left_send_vis"), func() { ui.uploadVisibleBatch() }),
			fyne.NewMenuItem(tr("ctx_copy"), func() { ui.copySelectedEntry(true, id) }),
			fyne.NewMenuItem(tr("ctx_paste"), func() { ui.pasteCopiedTo(true, targetDir) }),
			fyne.NewMenuItem(tr("ctx_rename"), func() { ui.renameActive() }),
			fyne.NewMenuItem(tr("ctx_delete"), func() { ui.deleteActive() }),
			fyne.NewMenuItem(tr("ctx_new_folder"), func() { ui.createFolderActive() }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem(tr("ctx_refresh_local"), func() { ui.refreshLeft() }),
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
		fyne.NewMenuItem(tr("ctx_right_open"), func() { ui.onRightActivate() }),
		fyne.NewMenuItem(tr("ctx_right_recv"), func() { ui.download() }),
		fyne.NewMenuItem(tr("ctx_right_recv_vis"), func() { ui.downloadVisibleBatch() }),
		fyne.NewMenuItem(tr("ctx_copy"), func() { ui.copySelectedEntry(false, id) }),
		fyne.NewMenuItem(tr("ctx_paste"), func() { ui.pasteCopiedTo(false, targetDir) }),
		fyne.NewMenuItem(tr("ctx_rename"), func() { ui.renameActive() }),
		fyne.NewMenuItem(tr("ctx_delete"), func() { ui.deleteActive() }),
		fyne.NewMenuItem(tr("ctx_new_folder"), func() { ui.createFolderActive() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(tr("ctx_refresh_remote"), func() { ui.refreshRight() }),
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

// appendOperationHistory executa parte da logica deste modulo.
func (ui *explorer) appendOperationHistory(entry string) {
	msg := strings.TrimSpace(entry)
	if msg == "" {
		return
	}
	stamp := time.Now().Format("02/01 15:04:05")
	ui.opHistory = append(ui.opHistory, fmt.Sprintf("%s | %s", stamp, msg))
	const maxEntries = 500
	if len(ui.opHistory) > maxEntries {
		ui.opHistory = ui.opHistory[len(ui.opHistory)-maxEntries:]
	}
	saveOperationHistoryPreference(ui.opHistory)
	appendAuditLog("operacao", msg)
}

// rememberFailedJob executa parte da logica deste modulo.
func (ui *explorer) rememberFailedJob(job transfer.Job) {
	if job.Run == nil {
		return
	}
	ui.failedJobs = append(ui.failedJobs, job)
	const maxFailedJobs = 20
	if len(ui.failedJobs) > maxFailedJobs {
		ui.failedJobs = ui.failedJobs[len(ui.failedJobs)-maxFailedJobs:]
	}
}

// retryLastFailedOperation executa parte da logica deste modulo.
func (ui *explorer) retryLastFailedOperation() {
	if len(ui.failedJobs) == 0 {
		dialog.ShowInformation(tr("dlg_history_title"), tr("dlg_history_no_fail"), ui.win)
		return
	}
	job := ui.failedJobs[len(ui.failedJobs)-1]
	ui.tm.Enqueue(job)
	ui.appendOperationHistory("Reexecução solicitada: " + job.Name)
	ui.status.SetText("Reexecutando última falha: " + job.Name)
	ui.startDrain()
}

// retryAllFailedOperations executa parte da logica deste modulo.
func (ui *explorer) retryAllFailedOperations() {
	if len(ui.failedJobs) == 0 {
		dialog.ShowInformation(tr("dlg_history_title"), tr("dlg_history_no_fail"), ui.win)
		return
	}
	dialog.ShowConfirm(
		tr("dlg_history_retry_all_title"),
		fmt.Sprintf(tr("dlg_history_retry_all_fmt"), len(ui.failedJobs)),
		func(ok bool) {
			if !ok {
				return
			}
			jobs := append([]transfer.Job(nil), ui.failedJobs...)
			for _, job := range jobs {
				ui.tm.Enqueue(job)
			}
			ui.appendOperationHistory(fmt.Sprintf("Reexecução em lote solicitada: %d falha(s)", len(jobs)))
			ui.status.SetText(fmt.Sprintf("Reexecutando %d falha(s) recentes...", len(jobs)))
			ui.startDrain()
		},
		ui.win,
	)
}

// showOperationHistory executa parte da logica deste modulo.
func (ui *explorer) showOperationHistory() {
	buildSessionContent := func(term string) string {
		lines := ui.opHistory
		if t := strings.ToLower(strings.TrimSpace(term)); t != "" {
			filtered := make([]string, 0, len(lines))
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), t) {
					filtered = append(filtered, line)
				}
			}
			lines = filtered
		}
		if len(lines) == 0 {
			return "Sem operações registradas para este filtro."
		}
		if len(lines) > 120 {
			lines = lines[len(lines)-120:]
		}
		return strings.Join(lines, "\n")
	}
	buildFullLogContent := func(term string) string {
		lines := readAuditLogLines(600, term)
		if len(lines) == 0 {
			return "Sem registros no log geral para este filtro."
		}
		return strings.Join(lines, "\n")
	}
	sessionLog := widget.NewLabel(buildSessionContent(""))
	sessionLog.Wrapping = fyne.TextWrapWord
	sessionFilter := widget.NewEntry()
	sessionFilter.SetPlaceHolder("Filtrar sessão (ex.: erro, concluído, upload)")
	sessionFilter.OnChanged = func(text string) {
		sessionLog.SetText(buildSessionContent(text))
	}
	fullLog := widget.NewLabel(buildFullLogContent(""))
	fullLog.Wrapping = fyne.TextWrapWord
	fullLogFilter := widget.NewEntry()
	fullLogFilter.SetPlaceHolder("Filtrar log geral (ex.: ERROR, login, transferência)")
	fullLogFilter.OnChanged = func(text string) {
		fullLog.SetText(buildFullLogContent(text))
	}
	sessionPane := fynecontainer.NewBorder(
		fynecontainer.NewPadded(sessionFilter),
		nil,
		nil,
		nil,
		fynecontainer.NewScroll(sessionLog),
	)
	fullLogPane := fynecontainer.NewBorder(
		fynecontainer.NewPadded(fullLogFilter),
		nil,
		nil,
		nil,
		fynecontainer.NewScroll(fullLog),
	)
	tabs := fynecontainer.NewAppTabs(
		fynecontainer.NewTabItem("Sessão", sessionPane),
		fynecontainer.NewTabItem("Log geral", fullLogPane),
	)
	tabs.SetTabLocation(fynecontainer.TabLocationTop)
	btnExport := widget.NewButtonWithIcon("Exportar .log", theme.DocumentSaveIcon(), func() {
		target := filepath.Join(filepath.Dir(auditLogPath()), "historico-operacoes.log")
		content := buildSessionContent(sessionFilter.Text)
		if err := os.WriteFile(target, []byte(content+"\n"), 0o644); err != nil {
			dialog.ShowError(fmt.Errorf("não foi possível exportar histórico: %w", err), ui.win)
			return
		}
		ui.status.SetText("Histórico exportado: " + target)
		appendAuditLog("historico", "Histórico exportado para "+target)
	})
	btnExportAuditCSV := widget.NewButtonWithIcon("Exportar trilha CSV", theme.DownloadIcon(), func() {
		target := filepath.Join(filepath.Dir(auditLogPath()), "containerway-audit.csv")
		b, err := writeAuditLogCSVBytes(8000, fullLogFilter.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("não foi possível gerar CSV: %w", err), ui.win)
			return
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			dialog.ShowError(fmt.Errorf("não foi possível gravar CSV: %w", err), ui.win)
			return
		}
		ui.status.SetText("Trilha de auditoria exportada: " + target)
		appendAuditLog("historico", "Trilha exportada em CSV: "+target)
	})
	btnOpenFullLog := widget.NewButtonWithIcon("Abrir log geral", theme.DocumentIcon(), func() {
		logPath := auditLogPath()
		if _, err := os.Stat(logPath); err != nil {
			dialog.ShowError(fmt.Errorf("log geral ainda não existe: %w", err), ui.win)
			return
		}
		if err := openWithDefaultApp(logPath); err != nil {
			dialog.ShowError(fmt.Errorf("não foi possível abrir log geral: %w", err), ui.win)
			return
		}
		ui.status.SetText("Log geral aberto: " + logPath)
	})
	btnOpenLogFolder := widget.NewButtonWithIcon("Abrir pasta de logs", theme.FolderOpenIcon(), func() {
		logDir := filepath.Dir(auditLogPath())
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			dialog.ShowError(fmt.Errorf("não foi possível preparar pasta de logs: %w", err), ui.win)
			return
		}
		if err := openWithDefaultApp(logDir); err != nil {
			dialog.ShowError(fmt.Errorf("não foi possível abrir pasta de logs: %w", err), ui.win)
			return
		}
		ui.status.SetText("Pasta de logs aberta: " + logDir)
	})
	btnRetry := widget.NewButtonWithIcon("Tentar novamente última falha", theme.ViewRefreshIcon(), func() {
		ui.retryLastFailedOperation()
	})
	btnRetryAll := widget.NewButtonWithIcon("Tentar novamente todas as falhas", theme.MediaReplayIcon(), func() {
		ui.retryAllFailedOperations()
	})
	btnClear := widget.NewButtonWithIcon("Limpar histórico", theme.DeleteIcon(), func() {
		ui.opHistory = nil
		ui.failedJobs = nil
		saveOperationHistoryPreference(nil)
		ui.status.SetText("Histórico de operações limpo.")
		sessionLog.SetText("Sem operações registradas para este filtro.")
		appendAuditLog("historico", "Histórico de operações limpo")
	})
	if len(ui.failedJobs) == 0 {
		btnRetry.Disable()
		btnRetryAll.Disable()
	}
	contentWrap := fynecontainer.NewBorder(
		nil,
		fynecontainer.NewHBox(btnExport, btnExportAuditCSV, btnOpenFullLog, btnOpenLogFolder, btnRetry, btnRetryAll, btnClear),
		nil,
		nil,
		tabs,
	)
	historyDialog := dialog.NewCustom(
		"Histórico de operações",
		"Fechar",
		contentWrap,
		ui.win,
	)
	historyDialog.Resize(fyne.NewSize(860, 420))
	historyDialog.Show()
}

// showTerminalConsole abre um console SSH básico reaproveitando a sessão já autenticada.
func (ui *explorer) showTerminalConsole() {
	defer func() {
		if r := recover(); r != nil {
			dialog.ShowError(fmt.Errorf(tr("ui_term_open_fail_fmt"), r), ui.win)
		}
	}()
	if ui.s == nil || ui.s.SSH == nil {
		dialog.ShowError(fmt.Errorf("%s", tr("ui_term_ssh_unavail")), ui.win)
		return
	}

	currentDir := strings.TrimSpace(ui.rightPath)
	if currentDir == "" {
		currentDir = "/"
	}

	host := strings.TrimSpace(ui.connCreds.Host)
	if host == "" {
		host = ui.s.HostAddr()
	}
	if err := ui.showTerminalConsoleVT(currentDir, host); err == nil {
		return
	}
	ui.showTerminalConsoleCompat(currentDir, host)
}

func (ui *explorer) showTerminalConsoleCompat(currentDir, host string) {
	terminal := newTerminalEntry()
	terminal.Wrapping = fyne.TextWrapOff
	terminal.SetMinRowsVisible(22)

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.SetText(fmt.Sprintf(tr("ui_term_compat_connecting_fmt"), host, currentDir))

	var (
		textMu      sync.Mutex
		internalSet bool
		baseText    = "# Abrindo shell interativo...\n"
		pendingUTF8 []byte
	)

	setTerminalText := func(v string) {
		internalSet = true
		terminal.SetText(v)
		lines := strings.Split(v, "\n")
		terminal.CursorRow = len(lines) - 1
		terminal.CursorColumn = len([]rune(lines[len(lines)-1]))
		terminal.Refresh()
		internalSet = false
	}
	setTerminalText(baseText)

	clearBtn := widget.NewButton(tr("ui_term_clear"), nil)
	clearBtn.Importance = widget.MediumImportance
	ctrlCBtn := widget.NewButton(tr("ex_back"), nil)
	ctrlCBtn.Importance = widget.MediumImportance

	sess, err := ui.s.SSH.NewSession()
	if err != nil {
		dialog.ShowError(fmt.Errorf("erro ao abrir sessão SSH: %w", err), ui.win)
		return
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		dialog.ShowError(fmt.Errorf("erro ao abrir stdin SSH: %w", err), ui.win)
		return
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		dialog.ShowError(fmt.Errorf("erro ao abrir stdout SSH: %w", err), ui.win)
		return
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		dialog.ShowError(fmt.Errorf("erro ao abrir stderr SSH: %w", err), ui.win)
		return
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", 40, 140, modes); err != nil {
		_ = sess.Close()
		dialog.ShowError(fmt.Errorf("erro ao requisitar TTY SSH: %w", err), ui.win)
		return
	}

	startCmd := "cd -- " + shellQuote(currentDir) + " 2>/dev/null || cd /; export TERM=xterm; export PS1='$ '; export PROMPT_COMMAND=''; exec bash --noprofile --norc -i"
	if err := sess.Start(startCmd); err != nil {
		_ = sess.Close()
		dialog.ShowError(fmt.Errorf("erro ao iniciar shell remoto: %w", err), ui.win)
		return
	}

	closed := atomic.Bool{}
	closeTerminal := func() {
		if closed.Swap(true) {
			return
		}
		ui.win.Canvas().SetOnTypedRune(nil)
		ui.win.Canvas().SetOnTypedKey(nil)
		_, _ = io.WriteString(stdin, "exit\n")
		_ = stdin.Close()
		_ = sess.Close()
	}

	appendFromRemote := func(chunk []byte) {
		textMu.Lock()
		decoded, rest := decodeTerminalUTF8(chunk, pendingUTF8)
		pendingUTF8 = rest
		textMu.Unlock()
		decoded = normalizeTerminalChunk(decoded)
		decoded = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\t' {
				return r
			}
			if r < 32 {
				return -1
			}
			return r
		}, decoded)
		chunkText := applyTerminalBackspaces(decoded)
		chunkText = strings.ReplaceAll(chunkText, "\t", "    ")
		chunkText = strings.ReplaceAll(chunkText, "\uFFFD", "")
		if chunkText == "" {
			return
		}
		textMu.Lock()
		baseText += chunkText
		textMu.Unlock()
		fyne.Do(func() { setTerminalText(baseText) })
	}
	streamLoop := func(r io.Reader) {
		buf := make([]byte, 4096)
		for {
			n, rerr := r.Read(buf)
			if n > 0 {
				appendFromRemote(buf[:n])
			}
			if rerr != nil {
				return
			}
		}
	}
	go streamLoop(stdout)
	go streamLoop(stderr)
	go func() {
		waitErr := sess.Wait()
		fyne.Do(func() {
			if waitErr != nil && !closed.Load() {
				status.SetText(fmt.Sprintf(tr("ui_term_session_closed_fmt"), strings.TrimSpace(waitErr.Error())))
			} else if !closed.Load() {
				status.SetText(tr("ui_term_session_closed"))
			}
			shouldAutoBack := !closed.Load()
			if shouldAutoBack {
				closeTerminal()
				ui.closeSettingsFullscreen()
				return
			}
			ui.win.Canvas().SetOnTypedRune(nil)
			ui.win.Canvas().SetOnTypedKey(nil)
			ctrlCBtn.Disable()
			clearBtn.Disable()
			terminal.Disable()
		})
	}()

	sendKey := func(seq string) {
		if closed.Load() || seq == "" {
			return
		}
		if _, err := io.WriteString(stdin, seq); err != nil {
			status.SetText(fmt.Sprintf(tr("ui_term_send_key_fail_fmt"), strings.TrimSpace(err.Error())))
		}
	}
	runTerminalCommand := func(cmd string) {
		if strings.TrimSpace(cmd) == "" {
			return
		}
		sendKey(cmd + "\r")
	}
	ctrlCBtn.OnTapped = func() {
		sendKey("\u0003")
		status.SetText(tr("ui_term_ctrl_c_keyboard"))
	}
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyC,
		Modifier: fyne.KeyModifierControl,
	}, func(fyne.Shortcut) {
		if closed.Load() {
			return
		}
		sendKey("\u0003")
		status.SetText(tr("ui_term_ctrl_c_keyboard"))
	})
	clearBtn.OnTapped = func() {
		sendKey("clear\r")
		status.SetText("Comando clear enviado.")
	}
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyUp}, func(fyne.Shortcut) { sendKey("\x1bOA") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyDown}, func(fyne.Shortcut) { sendKey("\x1bOB") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyRight}, func(fyne.Shortcut) { sendKey("\x1bOC") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeft}, func(fyne.Shortcut) { sendKey("\x1bOD") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageUp}, func(fyne.Shortcut) { sendKey("\x1b[5~") })
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPageDown}, func(fyne.Shortcut) { sendKey("\x1b[6~") })
	btnHtop := widget.NewButtonWithIcon(tr("ui_term_btn_htop"), theme.ComputerIcon(), func() {
		cmd := `command -v htop >/dev/null 2>&1 || { echo '[ContainerWay] htop não encontrado. Instalando...'; if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y htop; elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y htop; elif command -v yum >/dev/null 2>&1; then sudo yum install -y htop; elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm htop; else echo '[ContainerWay] Gerenciador de pacotes não suportado para instalação automática.'; fi; }; command -v htop >/dev/null 2>&1 && htop`
		runTerminalCommand(cmd)
		status.SetText(tr("ui_term_htop_opening"))
	})
	btnHtop.Importance = widget.MediumImportance
	btnNcdu := widget.NewButtonWithIcon(tr("ui_term_btn_ncdu"), theme.StorageIcon(), func() {
		cmd := `command -v ncdu >/dev/null 2>&1 || { echo '[ContainerWay] ncdu não encontrado. Instalando...'; if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y ncdu; elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y ncdu; elif command -v yum >/dev/null 2>&1; then sudo yum install -y ncdu; elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm ncdu; else echo '[ContainerWay] Gerenciador de pacotes não suportado para instalação automática.'; fi; }; command -v ncdu >/dev/null 2>&1 && cd / && ncdu`
		runTerminalCommand(cmd)
		status.SetText(tr("ui_term_ncdu_opening"))
	})
	btnNcdu.Importance = widget.MediumImportance
	terminal.onCtrlC = func() {
		sendKey("\u0003")
		status.SetText("Sinal Ctrl+C enviado (terminal).")
	}
	terminal.onF10 = func() {
		sendKey("\x1b[21~")
		status.SetText("F10 enviado (terminal).")
	}

	terminal.onTab = func() {
		if closed.Load() {
			return
		}
		textMu.Lock()
		snapshot := baseText
		textMu.Unlock()
		current := terminal.Text
		pending := ""
		if strings.HasPrefix(current, snapshot) {
			pending = strings.TrimPrefix(current, snapshot)
			if idx := strings.Index(pending, "\n"); idx >= 0 {
				pending = pending[:idx]
			}
		}
		if pending != "" {
			sendKey(pending)
			setTerminalText(snapshot)
		}
		sendKey("\t")
		status.SetText("TAB enviado.")
	}
	terminal.OnChanged = func(v string) {
		if internalSet || closed.Load() {
			return
		}
		textMu.Lock()
		snapshot := baseText
		textMu.Unlock()
		if !strings.HasPrefix(v, snapshot) {
			setTerminalText(snapshot)
			return
		}
		tail := strings.TrimPrefix(v, snapshot)
		if !strings.Contains(tail, "\n") {
			return
		}
		line := tail
		if idx := strings.Index(line, "\n"); idx >= 0 {
			line = line[:idx]
		}
		setTerminalText(snapshot)
		sendKey(line + "\r")
		status.SetText("Comando enviado.")
	}

	ui.win.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k == nil || closed.Load() {
			return
		}
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			sendKey("\r")
		case fyne.KeyBackspace:
			sendKey("\x7f")
		case fyne.KeyTab:
			sendKey("\t")
		case fyne.KeyEscape:
			sendKey("\x1b")
		case fyne.KeyUp:
			sendKey("\x1bOA")
		case fyne.KeyDown:
			sendKey("\x1bOB")
		case fyne.KeyRight:
			sendKey("\x1bOC")
		case fyne.KeyLeft:
			sendKey("\x1bOD")
		case fyne.KeyHome:
			sendKey("\x1b[H")
		case fyne.KeyEnd:
			sendKey("\x1b[F")
		case fyne.KeyDelete:
			sendKey("\x1b[3~")
		case fyne.KeyPageUp:
			sendKey("\x1b[5~")
		case fyne.KeyPageDown:
			sendKey("\x1b[6~")
		}
	})

	fyne.Do(func() {
		status.SetText(fmt.Sprintf(tr("ui_term_compat_active_fmt"), host))
	})

	const terminalToolBtnW = float32(66)
	ctrlCWrap := fynecontainer.NewGridWrap(fyne.NewSize(terminalToolBtnW, ctrlCBtn.MinSize().Height), ctrlCBtn)
	clearWrap := fynecontainer.NewGridWrap(fyne.NewSize(terminalToolBtnW+8, clearBtn.MinSize().Height), clearBtn)
	controls := fynecontainer.NewHBox(btnHtop, btnNcdu, layout.NewSpacer(), ctrlCWrap, clearWrap)
	if ui.useCompactLayout() {
		controls = fynecontainer.NewVBox(
			fynecontainer.NewHScroll(fynecontainer.NewHBox(btnHtop, btnNcdu)),
			fynecontainer.NewHBox(ctrlCWrap, clearWrap, layout.NewSpacer()),
		)
	}
	header := fynecontainer.NewVBox(controls)
	footer := fynecontainer.NewVBox(widget.NewSeparator(), status)
	body := fynecontainer.NewBorder(
		header,
		footer,
		nil,
		nil,
		fynecontainer.NewPadded(fynecontainer.NewMax(terminal)),
	)
	ui.openSettingsFullscreenWithBack(tr("ui_term_title"), body, closeTerminal)
}

// userManualText devolve o manual no idioma atual.
func userManualText() string {
	return strings.TrimSpace(tr("manual_body"))
}

// showUserManual executa parte da logica deste modulo.
func (ui *explorer) showUserManual() {
	appendAuditLog("manual", tr("manual_window_title")+" aberto")
	text := widget.NewRichTextWithText(userManualText())
	text.Wrapping = fyne.TextWrapWord
	scroll := fynecontainer.NewScroll(text)
	scroll.SetMinSize(fyne.NewSize(900, 500))
	dialog.NewCustom(
		tr("manual_window_title"),
		tr("manual_close"),
		scroll,
		ui.win,
	).Show()
}

// snapshotSettingsReturnTarget executa parte da logica deste modulo.
func (ui *explorer) snapshotSettingsReturnTarget() {
	switch ui.win.Content() {
	case ui.explorerMain:
		ui.settingsReturnToExplorer = true
	case ui.sessionHub:
		ui.settingsReturnToExplorer = false
	default:
		// Ex.: abrir «Alertas por e-mail» a partir da tela cheia de usuários — mantém o destino já definido.
	}
}

// closeSettingsFullscreen executa parte da logica deste modulo.
func (ui *explorer) closeSettingsFullscreen() {
	if ui.settingsReturnToExplorer {
		ui.win.SetContent(ui.explorerMain)
		ui.explorerOnTop.Store(true)
		setExplorerWindow(ui.win)
		return
	}
	ui.win.SetContent(ui.sessionHub)
	ui.explorerOnTop.Store(false)
	setSessionHubWindow(ui.win)
}

// openSettingsFullscreenWithBack troca o conteúdo da janela por uma tela cheia maximizada
// e permite executar uma ação antes de voltar.
func (ui *explorer) openSettingsFullscreenWithBack(title string, content fyne.CanvasObject, onBack func()) {
	ui.snapshotSettingsReturnTarget()
	btnBack := widget.NewButtonWithIcon(tr("ex_back"), theme.NavigateBackIcon(), func() {
		if onBack != nil {
			onBack()
		}
		ui.closeSettingsFullscreen()
	})
	btnBack.Importance = widget.MediumImportance
	titleLbl := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	titleLbl.Wrapping = fyne.TextWrapWord
	top := fynecontainer.NewBorder(nil, nil, fynecontainer.NewPadded(btnBack), nil, titleLbl)
	header := fynecontainer.NewVBox(top, widget.NewSeparator())
	var centerContent fyne.CanvasObject = content
	if ui.useCompactLayout() {
		centerContent = fynecontainer.NewVScroll(content)
	}
	center := fynecontainer.NewMax(fynecontainer.NewPadded(centerContent))
	root := fynecontainer.NewBorder(header, nil, nil, nil, center)
	ui.win.SetContent(root)
	ui.explorerOnTop.Store(false)
	setExplorerWindow(ui.win)
}

// openSettingsFullscreen troca o conteúdo da janela por uma tela cheia maximizada (admin: usuários / e-mail).
func (ui *explorer) openSettingsFullscreen(title string, content fyne.CanvasObject) {
	ui.openSettingsFullscreenWithBack(title, content, nil)
}

// showAccessUserManager executa parte da logica deste modulo.
func (ui *explorer) showAccessUserManager() {
	if !isCurrentAccessAdmin() {
		dialog.ShowInformation(tr("dlg_users_admin"), tr("dlg_users_admin_only"), ui.win)
		return
	}
	newUser := widget.NewEntry()
	newUser.SetPlaceHolder(tr("ui_users_ph_new"))
	newPass := widget.NewPasswordEntry()
	newPass.SetPlaceHolder(tr("ui_users_ph_pass"))
	newName := widget.NewEntry()
	newName.SetPlaceHolder(tr("ui_users_ph_display"))
	removeUser := widget.NewEntry()
	removeUser.SetPlaceHolder(tr("ui_users_ph_remove"))
	info := widget.NewLabel(tr("ui_users_info"))
	info.Wrapping = fyne.TextWrapWord
	usersList := widget.NewMultiLineEntry()
	usersList.Disable()
	usersList.Wrapping = fyne.TextWrapWord
	usersList.SetMinRowsVisible(14)

	refreshUsersList := func() {
		users := loadAccessAccounts()
		if len(users) == 0 {
			usersList.SetText(tr("ui_users_none"))
			return
		}
		lines := make([]string, 0, len(users)+1)
		lines = append(lines, tr("ui_users_list_header"))
		for _, u := range users {
			lines = append(lines, fmt.Sprintf(tr("ui_users_list_line_fmt"), u.Username, strings.TrimSpace(u.DisplayName)))
		}
		usersList.SetText(strings.Join(lines, "\n"))
	}
	refreshUsersList()

	btnSave := widget.NewButtonWithIcon(tr("ui_users_btn_save"), theme.DocumentSaveIcon(), func() {
		u := normalizeAccessUsername(newUser.Text)
		p := strings.TrimSpace(newPass.Text)
		n := strings.TrimSpace(newName.Text)
		if u == "" || p == "" {
			info.SetText(tr("ui_users_need_both"))
			return
		}
		if len(p) < 4 {
			info.SetText(tr("ui_users_pass_short"))
			return
		}
		if n == "" {
			n = u
		}
		users := loadAccessAccounts()
		users = upsertAccessAccount(users, accessUser{
			Username:    u,
			Password:    p,
			DisplayName: n,
		})
		saveAccessAccounts(users)
		refreshUsersList()
		appendAuditLog("acesso", "Usuário cadastrado/atualizado: "+u)
		info.SetText(fmt.Sprintf(tr("ui_users_saved_fmt"), u))
	})

	btnRemove := widget.NewButtonWithIcon(tr("ui_users_btn_remove"), theme.DeleteIcon(), func() {
		target := normalizeAccessUsername(removeUser.Text)
		if target == "" {
			info.SetText(tr("ui_users_need_remove"))
			return
		}
		if target == normalizeAccessUsername(defaultAccessUser) {
			info.SetText(tr("ui_users_admin_protected"))
			return
		}
		users := loadAccessAccounts()
		filtered := make([]accessUser, 0, len(users))
		removed := false
		for _, u := range users {
			if normalizeAccessUsername(u.Username) == target {
				removed = true
				continue
			}
			filtered = append(filtered, u)
		}
		if !removed {
			info.SetText(tr("ui_users_not_found"))
			return
		}
		saveAccessAccounts(filtered)
		refreshUsersList()
		appendAuditLog("acesso", "Usuário removido: "+target)
		info.SetText(fmt.Sprintf(tr("ui_users_removed_fmt"), target))
	})

	btnOpenMailNotify := widget.NewButtonWithIcon(tr("ui_users_btn_mail"), theme.MailComposeIcon(), func() {
		ui.showMailNotifySettings()
	})
	btnOpenMailNotify.Importance = widget.MediumImportance

	body := fynecontainer.NewVBox(
		btnOpenMailNotify,
		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem(tr("ui_users_fi_new"), newUser),
			widget.NewFormItem(tr("ui_users_fi_pass"), newPass),
			widget.NewFormItem(tr("ui_users_fi_name"), newName),
		),
		fynecontainer.NewHBox(btnSave),
		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem(tr("ui_users_fi_remove"), removeUser),
		),
		fynecontainer.NewHBox(btnRemove),
		widget.NewSeparator(),
		usersList,
		widget.NewSeparator(),
		info,
	)
	scroll := fynecontainer.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(320, 200))
	ui.openSettingsFullscreen(tr("ui_users_screen_title"), scroll)
}

// showMailNotifySettings executa parte da logica deste modulo.
func (ui *explorer) showMailNotifySettings() {
	if !isCurrentAccessAdmin() {
		dialog.ShowInformation(tr("dlg_mail_admin_title"), tr("dlg_mail_admin_only"), ui.win)
		return
	}
	cur := loadMailNotifySettings()
	recipients := append([]string(nil), cur.Recipients...)
	var selectedRecipient widget.ListItemID = -1

	enabled := widget.NewCheck(tr("ui_mail_enabled_check"), nil)
	enabled.SetChecked(cur.Enabled)

	lblRecipientCount := widget.NewLabel("")
	updateRecipientCount := func() {
		n := len(recipients)
		if n == 0 {
			lblRecipientCount.SetText(tr("ui_mail_rcpt_none"))
			return
		}
		if n == 1 {
			lblRecipientCount.SetText(tr("ui_mail_rcpt_one"))
			return
		}
		lblRecipientCount.SetText(fmt.Sprintf(tr("ui_mail_rcpt_many_fmt"), n))
	}
	updateRecipientCount()

	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder(tr("ui_mail_ph_host"))
	hostEntry.SetText(cur.Host)
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("587")
	if cur.Port > 0 {
		portEntry.SetText(strconv.Itoa(cur.Port))
	} else {
		portEntry.SetText("587")
	}
	userEntry := widget.NewEntry()
	userEntry.SetPlaceHolder(tr("ui_mail_ph_smtp_user"))
	userEntry.SetText(cur.User)
	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder(tr("ui_mail_ph_smtp_pass"))
	passEntry.SetText(cur.Password)
	fromEntry := widget.NewEntry()
	fromEntry.SetPlaceHolder(tr("ui_mail_ph_from"))
	fromEntry.SetText(cur.From)
	info := widget.NewLabel(tr("ui_mail_info_body"))
	info.Wrapping = fyne.TextWrapWord

	readForm := func() mailnotify.Settings {
		port, _ := strconv.Atoi(strings.TrimSpace(portEntry.Text))
		if port <= 0 {
			port = 587
		}
		recCopy := append([]string(nil), recipients...)
		return mailnotify.Settings{
			Enabled:    enabled.Checked,
			Host:       strings.TrimSpace(hostEntry.Text),
			Port:       port,
			User:       strings.TrimSpace(userEntry.Text),
			Password:   passEntry.Text,
			From:       strings.TrimSpace(fromEntry.Text),
			Recipients: mailnotify.NormalizeRecipients(recCopy),
		}
	}

	mailList := widget.NewList(
		func() int { return len(recipients) },
		func() fyne.CanvasObject {
			return widget.NewLabel(tr("ui_mail_list_placeholder"))
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id < 0 || int(id) >= len(recipients) {
				return
			}
			o.(*widget.Label).SetText(recipients[id])
		},
	)
	mailList.OnSelected = func(id widget.ListItemID) { selectedRecipient = id }
	mailList.OnUnselected = func(_ widget.ListItemID) { selectedRecipient = -1 }

	newAddrEntry := widget.NewEntry()
	newAddrEntry.SetPlaceHolder(tr("ui_mail_ph_new_addr"))

	refreshMailList := func() {
		updateRecipientCount()
		mailList.Refresh()
	}

	persistRecipients := func() {
		refreshMailList()
		s := readForm()
		autoOff := false
		if s.Enabled && len(s.Recipients) == 0 {
			enabled.SetChecked(false)
			s = readForm()
			autoOff = true
		}
		saveMailNotifySettings(s)
		recipients = append([]string(nil), loadMailNotifySettings().Recipients...)
		refreshMailList()
		if autoOff {
			dialog.ShowInformation(tr("dlg_rcpt_title"), tr("dlg_rcpt_none_saved"), ui.win)
		}
		appendAuditLog("acesso", "Lista de destinatários de alertas atualizada")
	}

	btnAddAddr := widget.NewButtonWithIcon(tr("ui_mail_btn_add"), theme.ContentAddIcon(), func() {
		e := strings.TrimSpace(newAddrEntry.Text)
		if e == "" || !strings.Contains(e, "@") {
			dialog.ShowInformation(tr("dlg_rcpt_title"), tr("dlg_rcpt_invalid"), ui.win)
			return
		}
		for _, x := range recipients {
			if strings.EqualFold(strings.TrimSpace(x), e) {
				dialog.ShowInformation(tr("dlg_rcpt_title"), tr("dlg_rcpt_dup"), ui.win)
				return
			}
		}
		recipients = append(recipients, e)
		newAddrEntry.SetText("")
		mailList.UnselectAll()
		selectedRecipient = -1
		persistRecipients()
	})

	btnRemoveAddr := widget.NewButtonWithIcon(tr("ui_mail_btn_remove_sel"), theme.ContentRemoveIcon(), func() {
		if selectedRecipient < 0 || int(selectedRecipient) >= len(recipients) {
			dialog.ShowInformation(tr("dlg_rcpt_title"), tr("dlg_rcpt_pick"), ui.win)
			return
		}
		i := int(selectedRecipient)
		recipients = append(recipients[:i], recipients[i+1:]...)
		mailList.UnselectAll()
		selectedRecipient = -1
		persistRecipients()
	})
	btnRemoveAddr.Importance = widget.MediumImportance

	btnRemoveByEmail := widget.NewButtonWithIcon(tr("ui_mail_btn_remove_typed"), theme.ContentRemoveIcon(), func() {
		e := strings.TrimSpace(newAddrEntry.Text)
		if e == "" || !strings.Contains(e, "@") {
			dialog.ShowInformation(tr("dlg_rcpt_title"), tr("dlg_rcpt_type_remove"), ui.win)
			return
		}
		idx := -1
		for i, x := range recipients {
			if strings.EqualFold(strings.TrimSpace(x), e) {
				idx = i
				break
			}
		}
		if idx < 0 {
			dialog.ShowInformation(tr("dlg_rcpt_title"), tr("dlg_rcpt_not_in_list"), ui.win)
			return
		}
		recipients = append(recipients[:idx], recipients[idx+1:]...)
		newAddrEntry.SetText("")
		mailList.UnselectAll()
		selectedRecipient = -1
		persistRecipients()
	})
	btnRemoveByEmail.Importance = widget.MediumImportance

	btnSave := widget.NewButtonWithIcon(tr("ui_mail_btn_save"), theme.DocumentSaveIcon(), func() {
		s := readForm()
		if s.Enabled && len(s.Recipients) == 0 {
			dialog.ShowInformation(tr("dlg_rcpt_title"), tr("dlg_rcpt_need_one"), ui.win)
			return
		}
		saveMailNotifySettings(s)
		recipients = append([]string(nil), s.Recipients...)
		refreshMailList()
		appendAuditLog("acesso", "Configuração de alertas por e-mail atualizada")
		dialog.ShowInformation(tr("dlg_mail_admin_title"), tr("dlg_mail_saved"), ui.win)
	})

	btnTest := widget.NewButtonWithIcon(tr("ui_mail_btn_test"), theme.MailComposeIcon(), func() {
		s := readForm()
		if !s.Valid() {
			dialog.ShowInformation(
				tr("dlg_mail_admin_title"),
				tr("dlg_mail_test_fill_form"),
				ui.win,
			)
			return
		}
		go func() {
			err := s.Send(tr("ui_mail_test_subject"), tr("ui_mail_test_body"))
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf(tr("ui_mail_test_fail_fmt"), err), ui.win)
				})
				return
			}
			fyne.Do(func() {
				dest := strings.Join(mailnotify.NormalizeRecipients(s.Recipients), "\n")
				msg := fmt.Sprintf(tr("dlg_mail_test_accepted_fmt"), dest)
				dialog.ShowInformation(tr("dlg_mail_admin_title"), msg, ui.win)
			})
		}()
	})

	btnTestSelf := widget.NewButton(tr("ui_mail_btn_test_self"), func() {
		s := readForm()
		if !s.ValidTransport() {
			dialog.ShowInformation(
				tr("dlg_mail_admin_title"),
				tr("dlg_mail_test_self_prereq"),
				ui.win,
			)
			return
		}
		go func() {
			fromAddr := s.EnvelopeFromAddress()
			err := s.SendTestToSelf(
				tr("ui_mail_test_self_subject"),
				fmt.Sprintf(tr("ui_mail_test_self_body"), fromAddr),
			)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf(tr("ui_mail_test_self_fail_fmt"), err), ui.win)
				})
				return
			}
			addr := fromAddr
			fyne.Do(func() {
				dialog.ShowInformation(
					tr("dlg_mail_admin_title"),
					fmt.Sprintf(tr("dlg_mail_test_self_ok_fmt"), addr),
					ui.win,
				)
			})
		}()
	})

	scrollRecipients := fynecontainer.NewScroll(mailList)
	scrollRecipients.SetMinSize(fyne.NewSize(220, 120))
	addRow := fynecontainer.NewBorder(nil, nil, nil, btnAddAddr, newAddrEntry)
	removeRow := fynecontainer.NewHBox(btnRemoveAddr, btnRemoveByEmail, layout.NewSpacer())

	recipientsCol := fynecontainer.NewVBox(
		widget.NewLabelWithStyle(tr("ui_mail_rcpt_title_bold"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		lblRecipientCount,
		fynecontainer.NewPadded(scrollRecipients),
		widget.NewSeparator(),
		addRow,
		removeRow,
	)

	portWrap := fynecontainer.NewGridWrap(fyne.NewSize(76, 0), portEntry)
	smtpLbl := func(text string) fyne.CanvasObject {
		return fynecontainer.NewGridWrap(fyne.NewSize(100, 0), widget.NewLabel(text))
	}
	smtpFieldRow := func(label string, field fyne.CanvasObject) fyne.CanvasObject {
		return fynecontainer.NewBorder(nil, nil, smtpLbl(label), nil, field)
	}
	hostPortRow := fynecontainer.NewBorder(nil, nil, nil, fynecontainer.NewHBox(widget.NewLabel(tr("ui_mail_fi_port")), portWrap), hostEntry)
	smtpCol := fynecontainer.NewVBox(
		widget.NewLabelWithStyle(tr("ui_mail_smtp_title_bold"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		fynecontainer.NewBorder(nil, nil, smtpLbl(tr("ui_mail_fi_host")), nil, hostPortRow),
		smtpFieldRow(tr("ui_mail_fi_user"), userEntry),
		smtpFieldRow(tr("ui_mail_fi_pass"), passEntry),
		smtpFieldRow(tr("ui_mail_fi_from"), fromEntry),
	)

	cols := fynecontainer.NewHSplit(
		fynecontainer.NewPadded(recipientsCol),
		fynecontainer.NewPadded(smtpCol),
	)
	cols.SetOffset(0.42)

	infoScroll := fynecontainer.NewScroll(info)
	infoScroll.SetMinSize(fyne.NewSize(200, 88))

	scrollContent := fynecontainer.NewVBox(
		infoScroll,
		widget.NewSeparator(),
		enabled,
		widget.NewSeparator(),
		fynecontainer.NewMax(cols),
	)
	scrollCentral := fynecontainer.NewScroll(scrollContent)
	scrollCentral.SetMinSize(fyne.NewSize(400, 200))

	actionBar := fynecontainer.NewVBox(
		widget.NewSeparator(),
		fynecontainer.NewHBox(btnSave, btnTest, layout.NewSpacer(), btnTestSelf),
	)
	fullBody := fynecontainer.NewBorder(nil, fynecontainer.NewPadded(actionBar), nil, nil, scrollCentral)

	ui.openSettingsFullscreen(tr("ui_mail_screen_title"), fullBody)
}

// openRemoteForEdit executa parte da logica deste modulo.
func (ui *explorer) openRemoteForEdit(e fsutil.DirEntry) {
	go func() {
		fyne.Do(func() {
			ui.status.SetText(tr("ui_remote_opening"))
		})
		ext := filepath.Ext(e.Name)
		tmp, err := os.CreateTemp("", "containerway-open-*"+ext)
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf(tr("ui_err_temp_create"), err), ui.win)
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
						dialog.ShowError(fmt.Errorf(tr("ui_err_remote_sudo_read"), err), ui.win)
					})
					return
				}
			} else {
				rf, err := ui.hfs.OpenReader(e.Path)
				if err != nil {
					fyne.Do(func() {
						if isPermissionDeniedError(err) {
							ui.maybePromptRootAccess(err)
							dialog.ShowInformation(
								tr("dlg_perm_denied_title"),
								tr("dlg_perm_denied_body"),
								ui.win,
							)
							return
						}
						dialog.ShowError(fmt.Errorf(tr("ui_err_remote_read"), err), ui.win)
					})
					return
				}
				defer rf.Close()
				out, err := os.Create(tmpPath)
				if err != nil {
					fyne.Do(func() {
						dialog.ShowError(fmt.Errorf(tr("ui_err_temp_write"), err), ui.win)
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
					dialog.ShowError(fmt.Errorf(tr("ui_err_container_read"), err), ui.win)
				})
				return
			}
			defer rc.Close()
			out, err := os.Create(tmpPath)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf(tr("ui_err_temp_write"), err), ui.win)
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
				dialog.ShowError(fmt.Errorf(tr("ui_err_download_open"), err), ui.win)
			})
			return
		}
		st, err := os.Stat(tmpPath)
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf(tr("ui_err_watch_start"), err), ui.win)
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

// trackRemoteEditSession executa parte da logica deste modulo.
func (ui *explorer) trackRemoteEditSession(s *remoteEditSession) {
	ui.remoteEditMu.Lock()
	defer ui.remoteEditMu.Unlock()
	if old, ok := ui.remoteEditSessions[s.tempPath]; ok {
		old.stopped.Store(true)
	}
	ui.remoteEditSessions[s.tempPath] = s
}

// startRemoteEditWatcher executa parte da logica deste modulo.
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
						dialog.ShowError(fmt.Errorf(tr("ui_err_remote_sync"), err), ui.win)
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

// syncEditedFileBack executa parte da logica deste modulo.
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

// openWithDefaultApp executa parte da logica deste modulo.
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
