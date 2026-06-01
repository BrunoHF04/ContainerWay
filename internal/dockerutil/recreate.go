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

func composeMetaFromLabels(labels map[string]string) (composeProjectMeta, error) {
	if labels == nil {
		return composeProjectMeta{}, ErrComposeMetadataMissing
	}
	project := strings.TrimSpace(labels["com.docker.compose.project"])
	if project == "" {
		return composeProjectMeta{}, ErrComposeMetadataMissing
	}
	workingDir := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
	configFilesRaw := strings.TrimSpace(labels["com.docker.compose.project.config_files"])
	configFiles := make([]string, 0, 4)
	for _, f := range strings.Split(configFilesRaw, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !filepath.IsAbs(f) && workingDir != "" {
			f = filepath.Join(workingDir, f)
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
	if workingDir != "" {
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
	commandVariants := []string{
		meta.BaseCmd + " up -d --force-recreate",
		"docker-compose -p " + ShellQuote(meta.Project) + " up -d --force-recreate",
	}
	if err := runComposeSSH(ctx, ssh, meta.WorkingDir, commandVariants); err != nil {
		return fmt.Errorf("falha ao recriar projeto %s: %w", meta.Project, err)
	}
	return nil
}
