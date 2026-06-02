package dockerutil

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/docker/docker/client"
)

// ErrComposeMetadataMissing indica que o contêiner não tem labels Compose para recreate.
var ErrComposeMetadataMissing = errors.New("metadados do docker compose ausentes")

// SSHRunner executa comandos no host remoto (sessão SSH).
type SSHRunner interface {
	RunSSH(ctx context.Context, cmd, input string) (stdout, stderr string, err error)
}

// ShellQuote escapa argumentos para sh.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

type composeProjectMeta struct {
	Project    string
	Service    string
	WorkingDir string
	BaseCmd    string
}

// isWindowsStylePath detecta caminhos tipo C:\foo gravados nas labels (inválidos em SSH Linux).
func isWindowsStylePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if strings.Contains(p, `\`) {
		return true
	}
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) {
		return true
	}
	return false
}

func sanitizeComposePath(p string) string {
	if isWindowsStylePath(p) {
		return ""
	}
	return strings.TrimSpace(p)
}

// windowsPathToLinuxGuess converte C:\SPCM\Docker\sga → /SPCM/Docker/sga (estrutura comum no host).
func windowsPathToLinuxGuess(p string) []string {
	if !isWindowsStylePath(p) {
		return nil
	}
	p = strings.TrimSpace(p)
	seen := make(map[string]struct{})
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.Contains(s, ":") {
			return
		}
		if !strings.HasPrefix(s, "/") {
			s = "/" + strings.TrimPrefix(s, "/")
		}
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
		}
	}
	unix := strings.ReplaceAll(p, `\`, "/")
	if i := strings.Index(unix, ":"); i >= 0 && i+1 < len(unix) {
		unix = unix[i+1:]
	}
	add(unix)
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	return out
}

func resolveLinuxComposeDir(ctx context.Context, ssh SSHRunner, project, windowsWorkingDir string) string {
	candidates := windowsPathToLinuxGuess(windowsWorkingDir)
	extra := []string{
		"/SPCM/Docker/" + project,
		"/opt/SPCM/Docker/" + project,
		"/opt/docker/" + project,
		"/var/docker/" + project,
	}
	candidates = append(candidates, extra...)
	for _, dir := range candidates {
		dir = strings.TrimSuffix(strings.TrimSpace(dir), "/")
		if dir == "" {
			continue
		}
		check := "test -d " + ShellQuote(dir) + " && ( test -f " + ShellQuote(dir+"/docker-compose.yml") +
			" || test -f " + ShellQuote(dir+"/docker-compose.yaml") + " || test -f " + ShellQuote(dir+"/compose.yml") + " )"
		if _, _, err := ssh.RunSSH(ctx, "sh -lc "+ShellQuote(check), ""); err == nil {
			return dir
		}
	}
	return ""
}

func enrichMetaFromRemote(ctx context.Context, ssh SSHRunner, meta *composeProjectMeta, labels map[string]string) {
	if meta == nil || meta.WorkingDir != "" {
		return
	}
	rawWD := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
	if !isWindowsStylePath(rawWD) {
		return
	}
	resolved := resolveLinuxComposeDir(ctx, ssh, meta.Project, rawWD)
	if resolved == "" {
		return
	}
	meta.WorkingDir = resolved
	meta.BaseCmd = "docker compose -p " + ShellQuote(meta.Project)
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml"} {
		path := resolved + "/" + name
		check := "test -f " + ShellQuote(path)
		if _, _, err := ssh.RunSSH(ctx, "sh -lc "+ShellQuote(check), ""); err == nil {
			meta.BaseCmd += " -f " + ShellQuote(path)
			break
		}
	}
}

func composeMetaFromLabels(labels map[string]string) (composeProjectMeta, error) {
	if labels == nil {
		return composeProjectMeta{}, ErrComposeMetadataMissing
	}
	project := strings.TrimSpace(labels["com.docker.compose.project"])
	if project == "" {
		return composeProjectMeta{}, ErrComposeMetadataMissing
	}
	workingDir := sanitizeComposePath(labels["com.docker.compose.project.working_dir"])
	configFilesRaw := strings.TrimSpace(labels["com.docker.compose.project.config_files"])
	configFiles := make([]string, 0, 4)
	for _, f := range strings.Split(configFilesRaw, ",") {
		f = sanitizeComposePath(f)
		if f == "" {
			continue
		}
		if !filepath.IsAbs(f) && workingDir != "" {
			f = filepath.Join(workingDir, f)
		}
		if isWindowsStylePath(f) {
			continue
		}
		configFiles = append(configFiles, f)
	}
	base := "docker compose -p " + ShellQuote(project)
	for _, f := range configFiles {
		base += " -f " + ShellQuote(f)
	}
	return composeProjectMeta{
		Project:    project,
		Service:    strings.TrimSpace(labels["com.docker.compose.service"]),
		WorkingDir: workingDir,
		BaseCmd:    base,
	}, nil
}

func runComposeSSH(ctx context.Context, ssh SSHRunner, workingDir string, commandVariants []string) error {
	workingDir = sanitizeComposePath(workingDir)
	if workingDir != "" && !isWindowsStylePath(workingDir) {
		for i, cmd := range commandVariants {
			commandVariants[i] = "cd " + ShellQuote(workingDir) + " && " + cmd
		}
	}
	var lastErr error
	for _, cmd := range commandVariants {
		stdout, stderr, runErr := ssh.RunSSH(ctx, "sh -lc "+ShellQuote(cmd), "")
		if runErr == nil {
			_ = stdout
			return nil
		}
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail != "" {
			lastErr = fmt.Errorf("%v: %s", runErr, detail)
		} else {
			lastErr = runErr
		}
	}
	return lastErr
}

// ForceRecreateContainer tenta recriar via Docker Compose no host remoto.
func ForceRecreateContainer(ctx context.Context, cli client.APIClient, ssh SSHRunner, containerID string) error {
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}
	meta, err := composeMetaFromLabels(inspect.Config.Labels)
	if err != nil || meta.Service == "" {
		return ErrComposeMetadataMissing
	}
	enrichMetaFromRemote(ctx, ssh, &meta, inspect.Config.Labels)
	commandVariants := []string{
		meta.BaseCmd + " up -d --force-recreate --pull always " + ShellQuote(meta.Service),
		meta.BaseCmd + " up -d --force-recreate " + ShellQuote(meta.Service),
		"docker-compose -p " + ShellQuote(meta.Project) + " up -d --force-recreate " + ShellQuote(meta.Service),
	}
	if err := runComposeSSH(ctx, ssh, meta.WorkingDir, commandVariants); err != nil {
		return fmt.Errorf("falha ao recriar serviço %s: %w", meta.Service, err)
	}
	return nil
}

// ForceRecreateComposeProject recria todos os serviços do projeto Compose.
func ForceRecreateComposeProject(ctx context.Context, cli client.APIClient, ssh SSHRunner, containerID string) error {
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}
	meta, err := composeMetaFromLabels(inspect.Config.Labels)
	if err != nil {
		return err
	}
	enrichMetaFromRemote(ctx, ssh, &meta, inspect.Config.Labels)
	commandVariants := []string{
		meta.BaseCmd + " up -d --force-recreate --pull always",
		meta.BaseCmd + " up -d --force-recreate",
		"docker-compose -p " + ShellQuote(meta.Project) + " up -d --force-recreate",
	}
	if err := runComposeSSH(ctx, ssh, meta.WorkingDir, commandVariants); err != nil {
		rawWD := strings.TrimSpace(inspect.Config.Labels["com.docker.compose.project.working_dir"])
		if isWindowsStylePath(rawWD) {
			return fmt.Errorf("falha ao recriar projeto %s: %w (o caminho nas labels do Docker é Windows %q; no host Linux use pasta tipo /SPCM/Docker/%s ou reimplemente o stack a partir do servidor)",
				meta.Project, err, rawWD, meta.Project)
		}
		return fmt.Errorf("falha ao recriar projeto %s: %w", meta.Project, err)
	}
	return nil
}
