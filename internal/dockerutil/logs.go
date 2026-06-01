package dockerutil

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"

	dcontainertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// LoadContainerLogs obtém logs com demux stdout/stderr quando aplicável.
// sinceRFC3339, se não vazio, limita logs a partir desse instante (ex.: último arranque).
func LoadContainerLogs(ctx context.Context, cli client.APIClient, containerID string, tail int, sinceRFC3339 string) (string, error) {
	if tail < 1 {
		tail = 200
	}
	opts := dcontainertypes.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       strconv.Itoa(tail),
	}
	sinceRFC3339 = strings.TrimSpace(sinceRFC3339)
	if sinceRFC3339 != "" {
		opts.Since = sinceRFC3339
	}
	resp, err := cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return "", err
	}
	defer resp.Close()

	raw, err := io.ReadAll(resp)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}

	var outBuf, errBuf bytes.Buffer
	if _, demuxErr := stdcopy.StdCopy(&outBuf, &errBuf, bytes.NewReader(raw)); demuxErr != nil {
		return strings.TrimSpace(string(raw)), nil
	}
	text := strings.TrimSpace(outBuf.String())
	stderrText := strings.TrimSpace(errBuf.String())
	if stderrText != "" {
		if text != "" {
			text += "\n\n"
		}
		text += "[stderr]\n" + stderrText
	}
	return strings.TrimSpace(text), nil
}
