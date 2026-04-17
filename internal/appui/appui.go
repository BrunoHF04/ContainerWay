package appui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	dcontainer "github.com/docker/docker/api/types/container"

	"containerway/internal/containerfs"
	"containerway/internal/fsutil"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
	"containerway/internal/session"
	"containerway/internal/transfer"
)

// Run inicia a aplicação Fyne.
func Run() {
	a := app.NewWithID("io.containerway.app")
	w := a.NewWindow("ContainerWay")
	w.Resize(fyne.NewSize(1100, 700))
	w.SetContent(buildLogin(w))
	w.ShowAndRun()
}

func buildLogin(w fyne.Window) fyne.CanvasObject {
	host := widget.NewEntry()
	host.SetPlaceHolder("ex.: 192.168.1.10 ou servidor:22")
	user := widget.NewEntry()
	user.SetPlaceHolder("utilizador SSH")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("senha (opcional se usar chave)")
	keyPath := widget.NewEntry()
	keyPath.SetPlaceHolder("caminho para .pem / id_rsa (OpenSSH)")
	keyPass := widget.NewPasswordEntry()
	keyPass.SetPlaceHolder("passphrase da chave (se aplicável)")
	status := widget.NewLabel("")

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Host", Widget: host},
			{Text: "Utilizador", Widget: user},
			{Text: "Senha", Widget: pass},
			{Text: "Chave PEM", Widget: keyPath},
			{Text: "Passphrase chave", Widget: keyPass},
		},
	}

	connect := widget.NewButton("Ligar", func() {
		status.SetText("A ligar…")
		creds := session.Credentials{
			Host:     host.Text,
			User:     user.Text,
			Password: pass.Text,
			KeyPath:  strings.TrimSpace(keyPath.Text),
			KeyPass:  keyPass.Text,
		}
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
				w.SetContent(buildExplorer(w, sess))
			})
		}()
	})

	return fynecontainer.NewVBox(
		widget.NewLabelWithStyle("ContainerWay — SSH + Docker + SFTP", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		form,
		connect,
		status,
	)
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
	leftSel   int
	rightSel  int

	leftList    *widget.List
	rightList   *widget.List
	breadcrumb  *widget.Label
	ctxSelect   *widget.Select
	status      *widget.Label
	progress    *widget.ProgressBar
	lastJobText *widget.Label

	tm *transfer.Manager
}

func buildExplorer(w fyne.Window, s *session.Session) fyne.CanvasObject {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := s.Docker.ContainerList(ctx, dcontainer.ListOptions{All: true})
	if err != nil {
		errLabel := widget.NewLabel(fmt.Sprintf("Docker: %v", err))
		closeBtn := widget.NewButton("Fechar sessão", func() {
			s.Close()
			w.SetContent(buildLogin(w))
		})
		return fynecontainer.NewVBox(errLabel, closeBtn)
	}

	ui := &explorer{
		win:           w,
		s:             s,
		hfs:           &hostfs.FS{Client: s.SFTP},
		leftPath:      homeOrRoot(),
		rightPath:     "/",
		hostMode:      true,
		containerOpts: []string{"Host (SFTP)"},
		containerIDs:  []string{""},
		leftSel:       -1,
		rightSel:      -1,
		tm:            &transfer.Manager{},
	}

	for _, c := range list {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if name == "" {
			name = "(sem nome)"
		}
		st := c.Status
		if len(st) > 24 {
			st = st[:21] + "…"
		}
		short := c.ID
		if len(short) > 12 {
			short = short[:12]
		}
		label := fmt.Sprintf("%s — %s [%s]", short, name, st)
		ui.containerOpts = append(ui.containerOpts, label)
		ui.containerIDs = append(ui.containerIDs, c.ID)
	}

	ui.breadcrumb = widget.NewLabel("")
	ui.status = widget.NewLabel("")
	ui.progress = widget.NewProgressBar()
	ui.progress.Hide()
	ui.lastJobText = widget.NewLabel("")

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
		ui.refreshRight()
		ui.updateBreadcrumb()
	})
	ui.ctxSelect.SetSelectedIndex(0)

	ui.leftList = widget.NewList(
		func() int { return len(ui.leftRows) },
		func() fyne.CanvasObject {
			return fynecontainer.NewHBox(widget.NewLabel("nome"), widget.NewLabel("tamanho"))
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			if id < 0 || id >= len(ui.leftRows) {
				return
			}
			e := ui.leftRows[id]
			l1 := box.Objects[0].(*widget.Label)
			l2 := box.Objects[1].(*widget.Label)
			suffix := ""
			if e.IsDir {
				suffix = " /"
			}
			l1.SetText(e.Name + suffix)
			l2.SetText(sizeLabel(e))
		},
	)
	ui.leftList.OnSelected = func(id widget.ListItemID) { ui.leftSel = int(id) }

	ui.rightList = widget.NewList(
		func() int { return len(ui.rightRows) },
		func() fyne.CanvasObject {
			return fynecontainer.NewHBox(widget.NewLabel("nome"), widget.NewLabel("tamanho"))
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			if id < 0 || id >= len(ui.rightRows) {
				return
			}
			e := ui.rightRows[id]
			suffix := ""
			if e.IsDir {
				suffix = " /"
			}
			l1 := box.Objects[0].(*widget.Label)
			l2 := box.Objects[1].(*widget.Label)
			l1.SetText(e.Name + suffix)
			l2.SetText(sizeLabel(e))
		},
	)
	ui.rightList.OnSelected = func(id widget.ListItemID) { ui.rightSel = int(id) }

	btnOpenLocal := widget.NewButton("Abrir pasta (local)", func() { ui.onLeftActivate() })
	btnOpenRemote := widget.NewButton("Abrir pasta (remoto)", func() { ui.onRightActivate() })

	btnUp := widget.NewButton("Enviar → (local para remoto)", func() { ui.upload() })
	btnDown := widget.NewButton("← Receber (remoto para local)", func() { ui.download() })
	btnRefresh := widget.NewButton("Atualizar", func() {
		ui.refreshLeft()
		ui.refreshRight()
	})
	btnDisconnect := widget.NewButton("Desligar", func() {
		s.Close()
		w.SetContent(buildLogin(w))
	})

	toolbar := fynecontainer.NewHBox(btnUp, btnDown, btnRefresh, layout.NewSpacer(), btnDisconnect)

	leftPane := fynecontainer.NewBorder(
		fynecontainer.NewHBox(widget.NewLabel("Local (Windows)"), layout.NewSpacer(), btnOpenLocal),
		nil, nil, nil,
		fynecontainer.NewScroll(ui.leftList),
	)
	rightPane := fynecontainer.NewBorder(
		fynecontainer.NewVBox(
			ui.ctxSelect,
			fynecontainer.NewHBox(ui.breadcrumb, layout.NewSpacer(), btnOpenRemote),
		),
		nil, nil, nil,
		fynecontainer.NewScroll(ui.rightList),
	)

	split := fynecontainer.NewHSplit(leftPane, rightPane)
	split.SetOffset(0.45)

	bottom := fynecontainer.NewVBox(ui.status, ui.progress, ui.lastJobText)

	ui.refreshLeft()
	ui.refreshRight()
	ui.updateBreadcrumb()

	return fynecontainer.NewBorder(toolbar, bottom, nil, nil, split)
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
	if ui.hostMode {
		ui.breadcrumb.SetText(fmt.Sprintf("Host: %s", ui.rightPath))
		return
	}
	short := ui.cfs.ID
	if len(short) > 12 {
		short = short[:12]
	}
	ui.breadcrumb.SetText(fmt.Sprintf("[%s]:%s", short, ui.rightPath))
}

func (ui *explorer) refreshLeft() {
	rows, err := localfs.List(ui.leftPath)
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	ui.leftRows = rows
	ui.leftList.Refresh()
}

func (ui *explorer) refreshRight() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var rows []fsutil.DirEntry
	var err error
	if ui.hostMode {
		rows, err = ui.hfs.List(ctx, ui.rightPath)
	} else {
		if ui.cfs == nil {
			return
		}
		rows, err = ui.cfs.List(ctx, ui.rightPath)
	}
	if err != nil {
		fyne.Do(func() {
			ui.status.SetText(err.Error())
		})
		return
	}
	ui.rightRows = rows
	fyne.Do(func() {
		ui.rightList.Refresh()
		ui.updateBreadcrumb()
	})
}

func (ui *explorer) onLeftActivate() {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		return
	}
	e := ui.leftRows[ui.leftSel]
	if e.IsDir {
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
		ui.rightPath = e.Path
		ui.refreshRight()
	}
}

func (ui *explorer) upload() {
	if ui.leftSel < 0 || ui.leftSel >= len(ui.leftRows) {
		dialog.ShowInformation("ContainerWay", "Selecione um ficheiro no painel local.", ui.win)
		return
	}
	src := ui.leftRows[ui.leftSel]
	if src.IsDir || src.Name == ".." {
		dialog.ShowInformation("ContainerWay", "Apenas ficheiros (não pastas) nesta versão.", ui.win)
		return
	}
	dstName := filepath.Base(src.Path)
	if ui.hostMode {
		dst := path.Join(ui.rightPath, dstName)
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Enviar %s → host:%s", src.Path, dst),
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
			Name: fmt.Sprintf("Enviar %s → contentor:%s", src.Path, ui.rightPath),
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
		dialog.ShowInformation("ContainerWay", "Selecione um ficheiro no painel remoto.", ui.win)
		return
	}
	src := ui.rightRows[ui.rightSel]
	if src.IsDir || src.Name == ".." {
		dialog.ShowInformation("ContainerWay", "Apenas ficheiros (não pastas) nesta versão.", ui.win)
		return
	}
	dstPath := filepath.Join(ui.leftPath, src.Name)

	if ui.hostMode {
		ui.tm.Enqueue(transfer.Job{
			Name: fmt.Sprintf("Receber host:%s → %s", src.Path, dstPath),
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
			Name: fmt.Sprintf("Receber contentor:%s → %s", src.Path, dstPath),
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
	ui.tm.DrainAsync(ctx,
		func(j transfer.Job) {
			fyne.Do(func() {
				ui.progress.Show()
				ui.progress.SetValue(0)
				ui.lastJobText.SetText(j.Name)
				ui.status.SetText("A transferir…")
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
				ui.refreshRight()
			})
		},
		func(done, total int64) {
			fyne.Do(func() {
				if total > 0 {
					ui.progress.SetValue(float64(done) / float64(total))
				}
			})
		},
	)
}
