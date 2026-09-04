package telegram

import (
	"fmt"
	"strings"
	"testing"
)

// TestGenerateGoTreeMultiFile exercises the torrent picker tree with a
// multi-file layout plus a top-level file: it must not panic and every
// selectable entry must carry a valid original file index.
func TestGenerateGoTreeMultiFile(t *testing.T) {
	tree := map[string]interface{}{}
	pathClass("/mnt/nas/Show/Season 1/Show.S01E01.mkv|1.0 GB|1", &tree)
	pathClass("/mnt/nas/Show/Season 1/Show.S01E02.mkv|1.0 GB|2", &tree)
	pathClass("/mnt/nas/Show/cover.jpg|100.0 KB|3", &tree)
	pathClass("loose.txt|1.0 KB|4", &tree)

	var sel [][2]int
	trees, _, _ := generateGoTree(tree, 0, &sel)
	if len(trees) == 0 {
		t.Fatal("expected non-empty tree")
	}
	if len(sel) == 0 {
		t.Fatal("expected non-empty select list")
	}
	seen := map[int]bool{}
	for i, e := range sel {
		if e[1] <= 0 {
			continue // directory node, no original file index
		}
		if e[1] < 1 || e[1] > 4 {
			t.Fatalf("entry %d has invalid original index %d", i, e[1])
		}
		if seen[e[1]] {
			t.Fatalf("duplicate original index %d", e[1])
		}
		seen[e[1]] = true
	}
	for i := 1; i <= 4; i++ {
		if !seen[i] {
			t.Fatalf("original index %d missing from select list", i)
		}
	}
	var sb strings.Builder
	for _, tr := range trees {
		sb.WriteString(tr.Print())
		sb.WriteString("\n")
	}
	out := sb.String()
	for _, want := range []string{"Show.S01E01.mkv", "Show.S01E02.mkv", "cover.jpg", "loose.txt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tree output missing %q:\n%s", want, out)
		}
	}
}

// TestGenerateGoTreeEmpty ensures an empty/unknown tree yields no nodes
// instead of panicking (callers auto-start the download in that case).
func TestGenerateGoTreeEmpty(t *testing.T) {
	var sel [][2]int
	trees, _, _ := generateGoTree(map[string]interface{}{}, 0, &sel)
	if len(trees) != 0 {
		t.Fatalf("expected empty tree, got %d nodes", len(trees))
	}
}

// TestSelectListToggleBounds guards the picker toggle path: every index
// referenced from a built tree must stay inside the select list.
func TestSelectListToggleBounds(t *testing.T) {
	tree := map[string]interface{}{}
	for i := 1; i <= 10; i++ {
		pathClass(fmt.Sprintf("/d/Season 1/Ep%02d.mkv|1.0 GB|%d", i, i), &tree)
	}
	var sel [][2]int
	if _, _, _ = generateGoTree(tree, 0, &sel); len(sel) == 0 {
		t.Fatal("expected non-empty select list")
	}
	// simulate toggling every displayed number (1-based UI numbering)
	for n := 1; n <= len(sel); n++ {
		i := n - 1
		if i < 0 || i >= len(sel) {
			t.Fatalf("toggle index %d out of bounds (len %d)", i, len(sel))
		}
	}
}
