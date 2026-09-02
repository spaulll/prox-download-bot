package extract

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fw, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractNativeZip(t *testing.T) {
	src := createTestZip(t, map[string]string{
		"dir/a.txt": "aaa",
		"b.txt":     "bbb",
	})
	dest := t.TempDir()
	e := &Extractor{}
	var lastDone, lastTotal int
	err := e.Extract(src, dest, func(done, total int, current string) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "dir", "a.txt"))
	if err != nil || string(data) != "aaa" {
		t.Errorf("a.txt missing or wrong: %v %q", err, data)
	}
	data, _ = os.ReadFile(filepath.Join(dest, "b.txt"))
	if string(data) != "bbb" {
		t.Errorf("b.txt wrong: %q", data)
	}
	if lastDone != 2 || lastTotal != 2 {
		t.Errorf("progress = %d/%d, want 2/2", lastDone, lastTotal)
	}
}

func TestZipSlipProtection(t *testing.T) {
	src := createTestZip(t, map[string]string{
		"../evil.txt": "evil",
		"ok.txt":      "ok",
	})
	dest := t.TempDir()
	outside := filepath.Dir(dest)
	e := &Extractor{}
	if err := e.Extract(src, dest, nil); err != nil {
		t.Fatal(err)
	}
	// the file must never escape the destination directory
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Error("zip slip: file escaped destination")
	}
	if _, err := os.Stat(filepath.Join(dest, "ok.txt")); err != nil {
		t.Error("ok.txt not extracted")
	}
}

func TestMultiVolumeDetection(t *testing.T) {
	cases := []struct {
		path    string
		isFirst bool
		prefix  string
	}{
		{"Show.part1.rar", true, "Show"},
		{"Show.part01.rar", true, "Show"},
		{"Show.part2.rar", false, "Show"},
		{"Show.part02.rar", false, "Show"},
		{"Single.rar", true, "Single"},
		{"Show.r00", false, "Show"},
	}
	for _, c := range cases {
		if got := IsFirstVolume(c.path); got != c.isFirst {
			t.Errorf("IsFirstVolume(%q) = %v, want %v", c.path, got, c.isFirst)
		}
		if p := MultiVolumePrefix(c.path); p != c.prefix {
			t.Errorf("MultiVolumePrefix(%q) = %q, want %q", c.path, p, c.prefix)
		}
	}
}

func TestIsArchivePath(t *testing.T) {
	yes := []string{"a.zip", "b.rar", "c.7z", "d.tar", "e.tar.gz", "f.tgz", "g.bz2", "h.xz"}
	no := []string{"a.mkv", "b.mp4", "c.txt", "d"}
	for _, n := range yes {
		if !IsArchivePath(n) {
			t.Errorf("IsArchivePath(%q) = false", n)
		}
	}
	for _, n := range no {
		if IsArchivePath(n) {
			t.Errorf("IsArchivePath(%q) = true", n)
		}
	}
}

func TestExtract7zWithReal7z(t *testing.T) {
	if !hasBin("7z") {
		t.Skip("7z not installed")
	}
	// create a real 7z archive
	src := filepath.Join(t.TempDir(), "test.7z")
	inDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inDir, "one.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(inDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inDir, "sub", "two.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec7z("a", src, filepath.Join(inDir, "one.txt"), filepath.Join(inDir, "sub", "two.txt")); err != nil {
		t.Fatalf("7z create failed: %v %s", err, out)
	}

	dest := t.TempDir()
	e := &Extractor{}
	if err := e.Extract(src, dest, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "one.txt"))
	if err != nil || string(data) != "one" {
		t.Errorf("one.txt missing/wrong: %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(dest, "two.txt"))
	if string(data) != "two" {
		t.Errorf("two.txt wrong: %q", data)
	}
}

func exec7z(args ...string) (string, error) {
	out, err := exec7zImpl(args...)
	return out, err
}
