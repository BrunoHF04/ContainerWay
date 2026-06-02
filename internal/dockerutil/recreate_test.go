package dockerutil

import "testing"

func TestIsWindowsStylePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\SPCM\Docker\sga`, true},
		{`/opt/orion/docker`, false},
		{`./compose`, false},
		{"", false},
		{`\\server\share`, true},
	}
	for _, tc := range tests {
		if got := isWindowsStylePath(tc.path); got != tc.want {
			t.Errorf("isWindowsStylePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestWindowsPathToLinuxGuess(t *testing.T) {
	got := windowsPathToLinuxGuess(`C:\SPCM\Docker\sga`)
	if len(got) == 0 || got[0] != "/SPCM/Docker/sga" {
		t.Fatalf("guess = %v", got)
	}
}

func TestComposeMetaSkipsWindowsPaths(t *testing.T) {
	meta, err := composeMetaFromLabels(map[string]string{
		"com.docker.compose.project":              "sga",
		"com.docker.compose.project.working_dir":  `C:\SPCM\Docker\sga`,
		"com.docker.compose.project.config_files": `C:\SPCM\Docker\sga\docker-compose.yml`,
		"com.docker.compose.service":              "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.WorkingDir != "" {
		t.Fatalf("WorkingDir = %q, want empty", meta.WorkingDir)
	}
	if meta.BaseCmd != "docker compose -p 'sga'" {
		t.Fatalf("BaseCmd = %q", meta.BaseCmd)
	}
}
