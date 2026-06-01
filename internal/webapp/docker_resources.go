package webapp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"containerway/internal/dockerutil"

	"github.com/docker/docker/api/types/image"
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

type dockerVolumeJSON struct {
	Name         string `json:"name"`
	Driver       string `json:"driver"`
	Mountpoint   string `json:"mountpoint"`
	Scope        string `json:"scope"`
	CreatedLabel string `json:"createdLabel"`
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

func listDockerVolumes(ctx context.Context, cli client.APIClient) ([]dockerVolumeJSON, error) {
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
		out = append(out, dockerVolumeJSON{
			Name:         v.Name,
			Driver:       v.Driver,
			Mountpoint:   v.Mountpoint,
			Scope:        v.Scope,
			CreatedLabel: created,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
