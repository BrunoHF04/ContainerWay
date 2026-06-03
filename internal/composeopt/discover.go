package composeopt

import (
	"path/filepath"
	"strings"
)

// ComposeFile entrada de ficheiro encontrado.
type ComposeFile struct {
	Path    string `json:"path"`
	Dir     string `json:"dir"`
	Name    string `json:"name"`
	Kind    string `json:"kind"` // compose | swarm
	HasDeploy bool `json:"hasDeploy,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// StackRef stack Docker Swarm activa no host.
type StackRef struct {
	Name string `json:"name"`
}

// DiscoverMeta metadados da descoberta.
type DiscoverMeta struct {
	Roots       string `json:"roots"`
	SwarmActive bool   `json:"swarmActive"`
	SwarmNodes  int    `json:"swarmNodes"`
	Stacks      []StackRef `json:"stacks,omitempty"`
}

// ParseDiscoverLines interpreta saída do find (path por linha).
func ParseDiscoverLines(raw string) []ComposeFile {
	var out []ComposeFile
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, ComposeFileFromPath(line))
	}
	return out
}

// ComposeFileFromPath cria entrada com nome e tipo heurístico.
func ComposeFileFromPath(path string) ComposeFile {
	dir := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		dir = path[:i]
	}
	name := filepath.Base(path)
	kind := "compose"
	low := strings.ToLower(name)
	if strings.Contains(low, "stack") || strings.Contains(low, "swarm") {
		kind = "swarm"
	}
	return ComposeFile{Path: path, Dir: dir, Name: name, Kind: kind}
}

// ParseClassifyLines interpreta path|kind do script de classificação.
func ParseClassifyLines(raw string, files []ComposeFile) []ComposeFile {
	byPath := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) == 2 && parts[1] == "swarm" {
			byPath[parts[0]] = "swarm"
		}
	}
	for i := range files {
		if k, ok := byPath[files[i].Path]; ok {
			files[i].Kind = k
			files[i].HasDeploy = true
		}
	}
	return files
}
