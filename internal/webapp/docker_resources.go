package webapp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"containerway/internal/dockerutil"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

type dockerImageJSON struct {
	ID           string   `json:"id"`
	IDFull       string   `json:"idFull"`
	Tags         []string `json:"tags"`
	Display      string   `json:"display"`
	Size         int64    `json:"size"`
	SizeHuman    string   `json:"sizeHuman"`
	Created      int64    `json:"created"`
	CreatedLabel string   `json:"createdLabel"`
	Containers   int64    `json:"containers"`
	Dangling     bool     `json:"dangling"`
}

type dockerVolumeContainerRef struct {
	ID          string `json:"id"`
	IDFull      string `json:"idFull"`
	DisplayName string `json:"displayName"`
	Running     bool   `json:"running"`
}

type dockerVolumeJSON struct {
	Name         string                     `json:"name"`
	Driver       string                     `json:"driver"`
	Mountpoint   string                     `json:"mountpoint"`
	Scope        string                     `json:"scope"`
	CreatedLabel string                     `json:"createdLabel"`
	Containers   []dockerVolumeContainerRef `json:"containers,omitempty"`
}

type dockerNetworkJSON struct {
	ID         string `json:"id"`
	IDFull     string `json:"idFull"`
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Scope      string `json:"scope"`
	Internal   bool   `json:"internal"`
	Attachable bool   `json:"attachable"`
	Containers int    `json:"containers"`
	Protected  bool   `json:"protected"`
}

type dockerSystemJSON struct {
	ImagesCount         int    `json:"imagesCount"`
	ImagesSize          int64  `json:"imagesSize"`
	ImagesSizeHuman     string `json:"imagesSizeHuman"`
	ImagesDangling      int    `json:"imagesDangling"`
	ImagesDanglingHuman string `json:"imagesDanglingHuman"`
	ContainersCount     int    `json:"containersCount"`
	ContainersSize      int64  `json:"containersSize"`
	ContainersSizeHuman string `json:"containersSizeHuman"`
	VolumesCount        int    `json:"volumesCount"`
	VolumesSize         int64  `json:"volumesSize"`
	VolumesSizeHuman    string `json:"volumesSizeHuman"`
	BuildCacheSize      int64  `json:"buildCacheSize"`
	BuildCacheSizeHuman string `json:"buildCacheSizeHuman"`
}

func formatBytesHuman(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	const unit = 1024
	sizes := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	f := float64(n)
	i := 0
	for f >= unit && i < len(sizes)-1 {
		f /= unit
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, sizes[0])
	}
	return fmt.Sprintf("%.1f %s", f, sizes[i])
}

func imageDisplay(tags []string) (display string, dangling bool) {
	if len(tags) == 0 {
		return "<sem tag>", true
	}
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || t == "<none>:<none>" {
			continue
		}
		clean = append(clean, t)
	}
	if len(clean) == 0 {
		return "<sem tag>", true
	}
	return clean[0], false
}

func listDockerImages(ctx context.Context, cli client.APIClient) ([]dockerImageJSON, error) {
	imgs, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]dockerImageJSON, 0, len(imgs))
	for _, img := range imgs {
		id := strings.TrimPrefix(img.ID, "sha256:")
		display, dangling := imageDisplay(img.RepoTags)
		created := time.Unix(img.Created, 0)
		out = append(out, dockerImageJSON{
			ID:           dockerutil.ShortID(id),
			IDFull:       img.ID,
			Tags:         append([]string(nil), img.RepoTags...),
			Display:      display,
			Size:         img.Size,
			SizeHuman:    formatBytesHuman(img.Size),
			Created:      img.Created,
			CreatedLabel: created.Format("2006-01-02 15:04"),
			Containers:   img.Containers,
			Dangling:     dangling,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Display) < strings.ToLower(out[j].Display)
	})
	return out, nil
}

func mapVolumeContainerUsage(ctx context.Context, cli client.APIClient) (map[string][]dockerVolumeContainerRef, error) {
	list, err := cli.ContainerList(ctx, dcontainer.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	byVol := make(map[string][]dockerVolumeContainerRef)
	seen := make(map[string]map[string]bool)
	for _, c := range list {
		id := strings.TrimPrefix(c.ID, "sha256:")
		disp := dockerutil.DisplayName(c)
		if disp == "" {
			disp = dockerutil.PrimaryName(c)
		}
		running := c.State == dcontainer.StateRunning || c.State == dcontainer.StateRestarting
		ref := dockerVolumeContainerRef{
			ID:          dockerutil.ShortID(id),
			IDFull:      c.ID,
			DisplayName: disp,
			Running:     running,
		}
		for _, m := range c.Mounts {
			if m.Type != "volume" || strings.TrimSpace(m.Name) == "" {
				continue
			}
			volName := m.Name
			if seen[volName] == nil {
				seen[volName] = make(map[string]bool)
			}
			if seen[volName][c.ID] {
				continue
			}
			seen[volName][c.ID] = true
			byVol[volName] = append(byVol[volName], ref)
		}
	}
	for volName := range byVol {
		sort.Slice(byVol[volName], func(i, j int) bool {
			ri, rj := byVol[volName][i].Running, byVol[volName][j].Running
			if ri != rj {
				return ri
			}
			return strings.ToLower(byVol[volName][i].DisplayName) < strings.ToLower(byVol[volName][j].DisplayName)
		})
	}
	return byVol, nil
}

func listDockerVolumes(ctx context.Context, cli client.APIClient) ([]dockerVolumeJSON, error) {
	usage, err := mapVolumeContainerUsage(ctx, cli)
	if err != nil {
		return nil, err
	}
	resp, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]dockerVolumeJSON, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		if v == nil {
			continue
		}
		created := ""
		if v.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
				created = t.Format("2006-01-02 15:04")
			} else {
				created = v.CreatedAt
			}
		}
		refs := usage[v.Name]
		if refs == nil {
			refs = []dockerVolumeContainerRef{}
		}
		out = append(out, dockerVolumeJSON{
			Name:         v.Name,
			Driver:       v.Driver,
			Mountpoint:   v.Mountpoint,
			Scope:        v.Scope,
			CreatedLabel: created,
			Containers:   refs,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func listDockerNetworks(ctx context.Context, cli client.APIClient) ([]dockerNetworkJSON, error) {
	nets, err := cli.NetworkList(ctx, networktypes.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]dockerNetworkJSON, 0, len(nets))
	for _, n := range nets {
		out = append(out, dockerNetworkJSON{
			ID:         dockerutil.ShortID(n.ID),
			IDFull:     n.ID,
			Name:       n.Name,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Internal:   n.Internal,
			Attachable: n.Attachable,
			Containers: len(n.Containers),
			Protected:  isProtectedNetwork(n.Name),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func dockerSystemUsage(ctx context.Context, cli client.APIClient) (dockerSystemJSON, error) {
	du, err := cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return dockerSystemJSON{}, err
	}
	out := dockerSystemJSON{
		ImagesCount:     len(du.Images),
		ContainersCount: len(du.Containers),
		VolumesCount:    len(du.Volumes),
	}
	var danglingSize int64
	for _, img := range du.Images {
		out.ImagesSize += img.Size
		_, dangling := imageDisplay(img.RepoTags)
		if dangling {
			out.ImagesDangling++
			danglingSize += img.Size
		}
	}
	for _, c := range du.Containers {
		out.ContainersSize += c.SizeRw
	}
	for _, v := range du.Volumes {
		if v.UsageData != nil && v.UsageData.Size > 0 {
			out.VolumesSize += v.UsageData.Size
		}
	}
	for _, rec := range du.BuildCache {
		out.BuildCacheSize += rec.Size
	}
	out.ImagesSizeHuman = formatBytesHuman(out.ImagesSize)
	out.ImagesDanglingHuman = formatBytesHuman(danglingSize)
	out.ContainersSizeHuman = formatBytesHuman(out.ContainersSize)
	out.VolumesSizeHuman = formatBytesHuman(out.VolumesSize)
	out.BuildCacheSizeHuman = formatBytesHuman(out.BuildCacheSize)
	return out, nil
}

func pruneDockerImages(ctx context.Context, cli client.APIClient, danglingOnly bool) (int64, error) {
	args := filters.NewArgs()
	if danglingOnly {
		args.Add("dangling", "true")
	}
	report, err := cli.ImagesPrune(ctx, args)
	if err != nil {
		return 0, err
	}
	return int64(report.SpaceReclaimed), nil
}

func pruneDockerVolumes(ctx context.Context, cli client.APIClient) (int64, error) {
	report, err := cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, err
	}
	return int64(report.SpaceReclaimed), nil
}
