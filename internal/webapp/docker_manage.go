package webapp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type dockerCreateContainerReq struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	Cmd           string `json:"cmd"`
	Env           string `json:"env"`
	Ports         string `json:"ports"`
	Restart       string `json:"restart"`
	Network       string `json:"network"`
	Detach        bool   `json:"detach"`
}

func parseEnvLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parsePortSpecs(raw string) (nat.PortSet, nat.PortMap, error) {
	exposed := nat.PortSet{}
	bindings := nat.PortMap{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return exposed, bindings, nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		hostPort := ""
		containerPort := part
		if strings.Contains(part, ":") {
			hp, cp, ok := strings.Cut(part, ":")
			if !ok {
				return nil, nil, fmt.Errorf("porta inválida: %q", part)
			}
			hostPort = strings.TrimSpace(hp)
			containerPort = strings.TrimSpace(cp)
		}
		proto := "tcp"
		if strings.Contains(containerPort, "/") {
			p, pr, ok := strings.Cut(containerPort, "/")
			if !ok {
				return nil, nil, fmt.Errorf("porta inválida: %q", part)
			}
			containerPort = p
			proto = strings.ToLower(pr)
		}
		if _, err := strconv.Atoi(containerPort); err != nil {
			return nil, nil, fmt.Errorf("porta inválida: %q", part)
		}
		port, err := nat.NewPort(proto, containerPort)
		if err != nil {
			return nil, nil, err
		}
		exposed[port] = struct{}{}
		if hostPort != "" {
			bindings[port] = []nat.PortBinding{{HostPort: hostPort}}
		}
	}
	return exposed, bindings, nil
}

func restartPolicyFromName(name string) container.RestartPolicy {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "always":
		return container.RestartPolicy{Name: "always"}
	case "unless-stopped":
		return container.RestartPolicy{Name: "unless-stopped"}
	case "on-failure":
		return container.RestartPolicy{Name: "on-failure"}
	default:
		return container.RestartPolicy{Name: "no"}
	}
}

func createDockerContainer(ctx context.Context, cli client.APIClient, req dockerCreateContainerReq) (string, error) {
	image := strings.TrimSpace(req.Image)
	if image == "" {
		return "", fmt.Errorf("imagem obrigatória")
	}
	exposed, portBindings, err := parsePortSpecs(req.Ports)
	if err != nil {
		return "", err
	}
	config := &container.Config{
		Image:        image,
		Env:          parseEnvLines(req.Env),
		ExposedPorts: exposed,
	}
	if cmd := strings.TrimSpace(req.Cmd); cmd != "" {
		config.Cmd = strings.Fields(cmd)
	}
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		RestartPolicy: restartPolicyFromName(req.Restart),
	}
	var networking *networktypes.NetworkingConfig
	netName := strings.TrimSpace(req.Network)
	if netName != "" {
		networking = &networktypes.NetworkingConfig{
			EndpointsConfig: map[string]*networktypes.EndpointSettings{
				netName: {},
			},
		}
	}
	name := strings.TrimSpace(req.Name)
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, networking, nil, name)
	if err != nil {
		return "", err
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", err
	}
	return resp.ID, nil
}

func createDockerNetwork(ctx context.Context, cli client.APIClient, name, driver string, internal bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("nome obrigatório")
	}
	if isProtectedNetwork(name) {
		return fmt.Errorf("nome de rede reservado")
	}
	driver = strings.TrimSpace(driver)
	if driver == "" {
		driver = "bridge"
	}
	_, err := cli.NetworkCreate(ctx, name, networktypes.CreateOptions{
		Driver:   driver,
		Internal: internal,
	})
	return err
}

func isProtectedNetwork(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bridge", "host", "none", "ingress":
		return true
	default:
		return false
	}
}

func removeDockerNetwork(ctx context.Context, cli client.APIClient, idOrName string) error {
	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		return fmt.Errorf("rede obrigatória")
	}
	if isProtectedNetwork(idOrName) {
		return fmt.Errorf("não é possível remover a rede predefinida «%s»", idOrName)
	}
	return cli.NetworkRemove(ctx, idOrName)
}

func createDockerVolume(ctx context.Context, cli client.APIClient, name, driver string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("nome obrigatório")
	}
	driver = strings.TrimSpace(driver)
	if driver == "" {
		driver = "local"
	}
	_, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: name, Driver: driver})
	return err
}

func pruneDockerBuildCache(ctx context.Context, cli client.APIClient) (int64, error) {
	report, err := cli.BuildCachePrune(ctx, build.CachePruneOptions{All: true})
	if err != nil {
		return 0, err
	}
	return int64(report.SpaceReclaimed), nil
}
