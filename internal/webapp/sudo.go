package webapp

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"containerway/internal/fsutil"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func (b *sshBundle) runSSHCommand(ctx context.Context, cmd, input string) (stdout, stderr string, err error) {
	if b == nil || b.Sess == nil || b.Sess.SSH == nil {
		return "", "", fmt.Errorf("sessão SSH indisponível")
	}
	ch := make(chan struct {
		out string
		err string
		e   error
	}, 1)
	go func() {
		sess, err := b.Sess.SSH.NewSession()
		if err != nil {
			ch <- struct{ out, err string; e error }{"", "", err}
			return
		}
		defer sess.Close()
		var outBuf, errBuf strings.Builder
		sess.Stdout = &outBuf
		sess.Stderr = &errBuf
		stdin, err := sess.StdinPipe()
		if err != nil {
			ch <- struct{ out, err string; e error }{"", "", err}
			return
		}
		if err := sess.Start(cmd); err != nil {
			ch <- struct{ out, err string; e error }{"", "", err}
			return
		}
		if input != "" {
			_, _ = io.WriteString(stdin, input+"\n")
		}
		_ = stdin.Close()
		err = sess.Wait()
		ch <- struct{ out, err string; e error }{outBuf.String(), errBuf.String(), err}
	}()
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case res := <-ch:
		return res.out, res.err, res.e
	}
}

func (b *sshBundle) sudoUID(ctx context.Context, user, password string) (string, error) {
	cmd := fmt.Sprintf("sudo -k -S -p '' -u %s sh -lc 'id -u'", shellQuote(user))
	out, stderr, err := b.runSSHCommand(ctx, cmd, password)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return "", fmt.Errorf("%s", strings.TrimSpace(stderr))
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (b *sshBundle) testSudoAccess(ctx context.Context, user, password string) (string, error) {
	target := strings.TrimSpace(user)
	if target == "" {
		target = "root"
	}
	uid, err := b.sudoUID(ctx, target, password)
	if err == nil && uid == "0" {
		return target, nil
	}
	if !strings.EqualFold(target, "root") {
		uidRoot, errRoot := b.sudoUID(ctx, "root", password)
		if errRoot == nil && uidRoot == "0" {
			return "root", nil
		}
		if errRoot != nil {
			return "", errRoot
		}
		return "", fmt.Errorf("sudo não elevou privilégios (uid=%s)", uidRoot)
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("sudo não elevou privilégios (uid=%s)", uid)
}

func (b *sshBundle) ensureSudoSession(ctx context.Context) error {
	b.sudoMu.Lock()
	enabled := b.sudoEnabled
	user := b.sudoUser
	pass := b.sudoPass
	validated := b.sudoValidatedAt
	b.sudoMu.Unlock()
	if !enabled || user == "" || pass == "" {
		return fmt.Errorf("sudo não configurado")
	}
	if !validated.IsZero() && time.Since(validated) < sudoSessionTTL {
		return nil
	}
	cmd := fmt.Sprintf("sudo -S -p '' -u %s -v", shellQuote(user))
	_, stderr, err := b.runSSHCommand(ctx, cmd, pass)
	if err != nil {
		b.clearSudo()
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("%s", strings.TrimSpace(stderr))
		}
		return err
	}
	b.sudoMu.Lock()
	b.sudoValidatedAt = time.Now()
	b.sudoMu.Unlock()
	return nil
}

func (b *sshBundle) clearSudo() {
	b.sudoMu.Lock()
	b.sudoEnabled = false
	b.sudoUser = ""
	b.sudoPass = ""
	b.sudoValidatedAt = time.Time{}
	b.sudoMu.Unlock()
}

func (b *sshBundle) setSudo(user, password string) {
	b.sudoMu.Lock()
	b.sudoEnabled = true
	b.sudoUser = user
	b.sudoPass = password
	b.sudoValidatedAt = time.Now()
	b.sudoMu.Unlock()
}

func (b *sshBundle) sudoStatus() map[string]any {
	b.sudoMu.Lock()
	defer b.sudoMu.Unlock()
	return map[string]any{
		"enabled": b.sudoEnabled,
		"user":    b.sudoUser,
	}
}

func (b *sshBundle) listHostWithSudo(ctx context.Context, dir string) ([]fsutil.DirEntry, error) {
	if err := b.ensureSudoSession(ctx); err != nil {
		return nil, err
	}
	b.sudoMu.Lock()
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()

	clean := path.Clean(strings.TrimSpace(dir))
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	listCmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(user),
		shellQuote("id -u; id -un; ls -1Ap -- "+shellQuote(clean)),
	)
	stdout, stderr, err := b.runSSHCommand(ctx, listCmd, pass)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	lines := strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n")
	if len(lines) < 3 {
		return nil, fmt.Errorf("resposta inesperada do sudo ao listar pasta")
	}
	if strings.TrimSpace(lines[0]) != "0" {
		return nil, fmt.Errorf("sudo não elevou privilégios (uid=%s)", strings.TrimSpace(lines[0]))
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
		if name == "" {
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

// readHostFileWithSudo lê ficheiro do host via cat com sudo.
func (b *sshBundle) readHostFileWithSudo(ctx context.Context, remotePath string) ([]byte, error) {
	if err := b.ensureSudoSession(ctx); err != nil {
		return nil, err
	}
	b.sudoMu.Lock()
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()

	clean := path.Clean(strings.TrimSpace(remotePath))
	cmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(user),
		shellQuote("cat -- "+shellQuote(clean)),
	)
	stdout, stderr, err := b.runSSHCommand(ctx, cmd, pass)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	data := []byte(stdout)
	if int64(len(data)) > maxEditorFileBytes {
		return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", maxEditorFileBytes/(1<<20))
	}
	if !isTextContent(data) {
		return nil, fmt.Errorf("ficheiro binário — não pode ser editado como texto")
	}
	return data, nil
}

// writeHostFileWithSudo grava ficheiro no host via cat com sudo.
func (b *sshBundle) writeHostFileWithSudo(ctx context.Context, remotePath string, data []byte) error {
	if err := b.ensureSudoSession(ctx); err != nil {
		return err
	}
	b.sudoMu.Lock()
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()
	if b.Sess == nil || b.Sess.SSH == nil {
		return fmt.Errorf("sessão SSH indisponível")
	}

	sess, err := b.Sess.SSH.NewSession()
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

	clean := path.Clean(strings.TrimSpace(remotePath))
	cmd := fmt.Sprintf(
		"sudo -S -p '' -u %s sh -lc %s",
		shellQuote(user),
		shellQuote("cat > "+shellQuote(clean)),
	)
	if err := sess.Start(cmd); err != nil {
		return err
	}
	if _, err := io.WriteString(stdin, pass+"\n"); err != nil {
		return err
	}
	if _, err := stdin.Write(data); err != nil {
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}

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
			return fmt.Errorf("%s", msg)
		}
	}
	b.sudoMu.Lock()
	b.sudoValidatedAt = time.Now()
	b.sudoMu.Unlock()
	return nil
}

// isPermissionDenied indica erro de permissão SFTP/SSH.
func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied")
}
