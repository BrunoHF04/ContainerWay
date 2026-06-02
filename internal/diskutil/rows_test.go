package diskutil

import "testing"

func TestBuildRows_filtersLoop(t *testing.T) {
	lsblk := `{"blockdevices":[{"name":"sda","path":"/dev/sda","type":"disk","size":1000,"children":[{"name":"sda1","path":"/dev/sda1","type":"part","size":500,"mountpoint":"/"}]}]}`
	df := "Filesystem Type 1B-blocks Used Available Use% Mounted on\n/dev/sda1 ext4 500 100 400 20% /\n"
	rows := BuildRows(lsblk, df, "", SortLsblkOrder, "", false)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	loopJSON := `{"blockdevices":[{"name":"loop0","path":"/dev/loop0","type":"loop","size":100}]}`
	rowsLoop := BuildRows(loopJSON, "", "", SortLsblkOrder, "", false)
	if len(rowsLoop) != 0 {
		t.Fatalf("expected loop hidden, got %d rows", len(rowsLoop))
	}
	rowsShow := BuildRows(loopJSON, "", "", SortLsblkOrder, "", true)
	if len(rowsShow) != 1 {
		t.Fatalf("expected 1 loop row when shown, got %d", len(rowsShow))
	}
}
