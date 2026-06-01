package webapp

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"containerway/internal/dockerutil"

	"github.com/docker/docker/api/types"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func (b *sshBundle) RunSSH(ctx context.Context, cmd, input string) (stdout, stderr string, err error) {
	return b.runSSHCommand(ctx, cmd, input)
}

type dockerContainerJSON struct {
	ID          string                     `json:"id"`
	IDFull      string                     `json:"idFull"`
	Name        string                     `json:"name"`
	DisplayName string                     `json:"displayName"`
	Image       string                     `json:"image"`
	State       string                     `json:"state"`
	StateLabel  string                     `json:"stateLabel"`
	Status      string                     `json:"status"`
	Running     bool                       `json:"running"`
	Restarting  bool                       `json:"restarting"`
	ComposeSvc     string                     `json:"composeService,omitempty"`
	ComposeProject string                     `json:"composeProject,omitempty"`
	SwarmSvc       string                     `json:"swarmService,omitempty"`
	Ports          string                     `json:"ports,omitempty"`
	Health         string                     `json:"health,omitempty"`
	Metrics        *dockerutil.MetricsSummary `json:"metrics,omitempty"`
}

func formatContainerPorts(ports []types.Port) string {
	if len(ports) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.PublicPort == 0 {
			continue
		}
		host := strings.TrimSpace(p.IP)
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "*"
		}
		parts = append(parts, fmt.Sprintf("%s:%d→%d/%s", host, p.PublicPort, p.PrivatePort, p.Type))
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 2 {
		return strings.Join(parts[:2], ", ") + fmt.Sprintf(" +%d", len(parts)-2)
	}
	return strings.Join(parts, ", ")
}

func healthFromDockerStatus(status string) string {
	s := strings.ToLower(status)
	switch {
	case strings.Contains(s, "(healthy)"):
		return "healthy"
	case strings.Contains(s, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(s, "health: starting"), strings.Contains(s, "(health: starting)"):
		return "starting"
	default:
		return ""
	}
}

func listDockerContainers(ctx context.Context, cli client.APIClient, all, withMetrics bool) ([]dockerContainerJSON, error) {
	list, err := cli.ContainerList(ctx, dcontainer.ListOptions{All: all})
	if err != nil {
		return nil, err
	}
	out := make([]dockerContainerJSON, 0, len(list))
	for _, c := range list {
		if !all && c.State != dcontainer.StateRunning && c.State != dcontainer.StateRestarting {
			continue
		}
		id := strings.TrimPrefix(c.ID, "sha256:")
		disp := dockerutil.DisplayName(c)
		if disp == "" {
			disp = dockerutil.PrimaryName(c)
		}
		row := dockerContainerJSON{
			ID:          dockerutil.ShortID(id),
			IDFull:      c.ID,
			Name:        dockerutil.PrimaryName(c),
			DisplayName: disp,
			Image:       c.Image,
			State:       string(c.State),
			StateLabel:  dockerutil.StateLabelPT(string(c.State)),
			Status:      c.Status,
			Running:     c.State == dcontainer.StateRunning,
			Restarting:  c.State == dcontainer.StateRestarting,
			Ports:       formatContainerPorts(c.Ports),
			Health:      healthFromDockerStatus(c.Status),
		}
		if c.Labels != nil {
			row.ComposeSvc = strings.TrimSpace(c.Labels["com.docker.compose.service"])
			row.ComposeProject = strings.TrimSpace(c.Labels["com.docker.compose.project"])
			row.SwarmSvc = strings.TrimSpace(c.Labels["com.docker.swarm.service.name"])
		}
		if withMetrics && (row.Running || row.Restarting) {
			if m, err := fetchContainerMetrics(ctx, cli, c.ID); err == nil {
				row.Metrics = &m
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func fetchContainerMetrics(ctx context.Context, cli client.APIClient, containerID string) (dockerutil.MetricsSummary, error) {
	st, err := cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return dockerutil.MetricsSummary{}, err
	}
	defer st.Body.Close()
	bb, err := io.ReadAll(st.Body)
	if err != nil {
		return dockerutil.MetricsSummary{}, err
	}
	stats, err := dockerutil.DecodeStatsJSON(bb)
	if err != nil {
		return dockerutil.MetricsSummary{}, err
	}
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return dockerutil.MetricsSummary{}, err
	}
	started := ""
	restarts := 0
	if inspect.State != nil {
		started = inspect.State.StartedAt
		restarts = inspect.RestartCount
	}
	return dockerutil.ParseStatsResponse(stats, started, restarts), nil
}

func resolveContainerID(ctx context.Context, cli client.APIClient, idPrefix string) (string, error) {
	idPrefix = strings.TrimSpace(idPrefix)
	if len(idPrefix) >= 64 {
		return idPrefix, nil
	}
	list, err := cli.ContainerList(ctx, dcontainer.ListOptions{All: true})
	if err != nil {
		return "", err
	}
	for _, c := range list {
		cid := strings.TrimPrefix(c.ID, "sha256:")
		if strings.HasPrefix(cid, idPrefix) || strings.HasPrefix(c.ID, idPrefix) {
			return c.ID, nil
		}
	}
	return idPrefix, nil
}

func restartComposeProject(ctx context.Context, b *sshBundle, containerID string) error {
	if b == nil || b.Sess == nil || b.Sess.Docker == nil {
		return errors.New("Docker indisponível")
	}
	fullID, err := resolveContainerID(ctx, b.Sess.Docker, containerID)
	if err != nil {
		return err
	}
	return dockerutil.ForceRecreateComposeProject(ctx, b.Sess.Docker, b, fullID)
}

func smartRestartContainer(ctx context.Context, b *sshBundle, containerID string) error {
	if b == nil || b.Sess == nil || b.Sess.Docker == nil {
		return errors.New("Docker indisponível")
	}
	cli := b.Sess.Docker
	fullID, err := resolveContainerID(ctx, cli, containerID)
	if err != nil {
		return err
	}
	err = dockerutil.ForceRecreateContainer(ctx, cli, b, fullID)
	if errors.Is(err, dockerutil.ErrComposeMetadataMissing) {
		timeout := 30
		return cli.ContainerRestart(ctx, fullID, dcontainer.StopOptions{Timeout: &timeout})
	}
	return err
}

func writeContainersCSV(w io.Writer, rows []dockerContainerJSON) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"nome", "display", "id", "estado", "imagem", "cpu_%", "ram_%", "uptime", "restarts"})
	for _, r := range rows {
		cpu, mem, uptime, restarts := "", "", "", ""
		if r.Metrics != nil {
			cpu = strconv.FormatFloat(r.Metrics.CPUPercent, 'f', 1, 64)
			mem = strconv.FormatFloat(r.Metrics.MemPercent, 'f', 1, 64)
			uptime = r.Metrics.Uptime
			restarts = strconv.Itoa(r.Metrics.RestartCount)
		}
		_ = cw.Write([]string{r.Name, r.DisplayName, r.ID, r.StateLabel, r.Image, cpu, mem, uptime, restarts})
	}
	cw.Flush()
	return cw.Error()
}
