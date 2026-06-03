package diskutil

import "testing"

func TestParseVGSBlock(t *testing.T) {
	block := "ubuntu-vg;231.20;59.50\n"
	stats := ParseVGSBlock(block)
	if len(stats) != 1 || stats[0].Name != "ubuntu-vg" {
		t.Fatalf("got %+v", stats)
	}
}

func TestParseLVSBlock_snapshot(t *testing.T) {
	block := "/dev/ubuntu-vg/root;ubuntu-vg;100.00;wi-a-----;;root\n/dev/ubuntu-vg/snap1;ubuntu-vg;10.00;swi-a-sni;/dev/ubuntu-vg/root;snap1\n"
	recs := ParseLVSBlock(block)
	if len(recs) < 2 {
		t.Fatalf("got %d records", len(recs))
	}
	var snap LVRecord
	for _, r := range recs {
		if r.IsSnapshot {
			snap = r
		}
	}
	if snap.Origin != "/dev/ubuntu-vg/root" {
		t.Fatalf("snapshot origin=%q", snap.Origin)
	}
	snaps := SnapshotsForOrigin(recs, "/dev/ubuntu-vg/root")
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snap, got %d", len(snaps))
	}
}
