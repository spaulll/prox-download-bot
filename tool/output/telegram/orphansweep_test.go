package telegram

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeAged creates file with content and backdates its mtime by age.
func writeAged(t *testing.T, path, content string, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-age)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}

func TestSweepOrphans(t *testing.T) {
	old := 2 * time.Hour
	fresh := time.Duration(0)

	t.Run("stale pair deleted", func(t *testing.T) {
		dl := t.TempDir()
		writeAged(t, filepath.Join(dl, "old.zip"), "data", old)
		writeAged(t, filepath.Join(dl, "old.zip.aria2"), "ctrl", old)
		got := sweepOrphansIn(dl, map[string]bool{}, time.Hour)
		if len(got) != 2 {
			t.Fatalf("deleted = %v, want both files", got)
		}
		if _, err := os.Stat(filepath.Join(dl, "old.zip")); !os.IsNotExist(err) {
			t.Errorf("base file survived")
		}
		if _, err := os.Stat(filepath.Join(dl, "old.zip.aria2")); !os.IsNotExist(err) {
			t.Errorf("control file survived")
		}
	})

	t.Run("fresh pair kept", func(t *testing.T) {
		dl := t.TempDir()
		writeAged(t, filepath.Join(dl, "new.zip"), "data", fresh)
		writeAged(t, filepath.Join(dl, "new.zip.aria2"), "ctrl", fresh)
		if got := sweepOrphansIn(dl, map[string]bool{}, time.Hour); len(got) != 0 {
			t.Errorf("deleted fresh = %v, want none", got)
		}
	})

	t.Run("lone stale control deleted", func(t *testing.T) {
		dl := t.TempDir()
		writeAged(t, filepath.Join(dl, "moved.zip.aria2"), "ctrl", old)
		got := sweepOrphansIn(dl, map[string]bool{}, time.Hour)
		if len(got) != 1 {
			t.Fatalf("deleted = %v, want the lone control file", got)
		}
	})

	t.Run("lone data file kept", func(t *testing.T) {
		dl := t.TempDir()
		writeAged(t, filepath.Join(dl, "complete.zip"), "data", old)
		if got := sweepOrphansIn(dl, map[string]bool{}, time.Hour); len(got) != 0 {
			t.Errorf("deleted complete data = %v, want none (recovery owns it)", got)
		}
	})

	t.Run("referenced pair kept", func(t *testing.T) {
		dl := t.TempDir()
		writeAged(t, filepath.Join(dl, "live.zip"), "data", old)
		writeAged(t, filepath.Join(dl, "live.zip.aria2"), "ctrl", old)
		ref := map[string]bool{"live.zip": true}
		if got := sweepOrphansIn(dl, ref, time.Hour); len(got) != 0 {
			t.Errorf("deleted referenced = %v, want none", got)
		}
	})

	t.Run("directory base kept", func(t *testing.T) {
		dl := t.TempDir()
		if err := os.Mkdir(filepath.Join(dl, "Show"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeAged(t, filepath.Join(dl, "Show.aria2"), "ctrl", old)
		if got := sweepOrphansIn(dl, map[string]bool{}, time.Hour); len(got) != 0 {
			t.Errorf("deleted dir-anchored = %v, want none", got)
		}
	})
}
