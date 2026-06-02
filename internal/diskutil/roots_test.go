package diskutil

import "testing"

func TestUsageRootsFromRows_mounts(t *testing.T) {
	rows := []Row{
		{Mount: "/"},
		{Mount: "/boot"},
		{Mount: "/home"},
	}
	roots := UsageRootsFromRows(rows)
	if len(roots) < 3 {
		t.Fatalf("expected at least 3 roots, got %d", len(roots))
	}
	seen := map[string]bool{}
	for _, r := range roots {
		seen[r["path"]] = true
	}
	if !seen["/"] || !seen["/boot"] || !seen["/home"] {
		t.Fatalf("missing mounts: %+v", roots)
	}
}
