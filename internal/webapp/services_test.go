package webapp

import "testing"

func TestParseServicesList(t *testing.T) {
	raw := `FILE|nginx.service|enabled
FILE|ssh.service|enabled
UNIT|nginx.service|active|running|A high performance web server
UNIT|ssh.service|active|running|OpenBSD Secure Shell server
`
	rows, err := parseServicesList(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	var nginx *serviceRow
	for i := range rows {
		if rows[i].Unit == "nginx.service" {
			nginx = &rows[i]
		}
	}
	if nginx == nil || nginx.Active != "active" || nginx.Enabled != "enabled" {
		t.Fatalf("nginx row: %+v", nginx)
	}
}

func TestParseServicesListError(t *testing.T) {
	_, err := parseServicesList("ERR|systemctl não disponível\n")
	if err == nil {
		t.Fatal("expected error")
	}
}
