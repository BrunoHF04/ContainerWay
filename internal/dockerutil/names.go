package dockerutil

import (
	"strings"

	dcontainer "github.com/docker/docker/api/types/container"
)

// DisplayName devolve um nome curto (Swarm/Compose ou prefixo do nome Docker).
func DisplayName(c dcontainer.Summary) string {
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
	if len(name) > 48 {
		if i := strings.IndexByte(name, '.'); i > 0 {
			return name[:i]
		}
	}
	return name
}

// PrimaryName devolve o primeiro nome Docker completo (sem barra inicial).
func PrimaryName(c dcontainer.Summary) string {
	if len(c.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// ShortID trunca o ID do contêiner para exibição.
func ShortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
