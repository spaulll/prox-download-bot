package organize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOrganizeIntegration verifies the full flow on a real temp library:
// a single episode file lands in Series/<Show>/Season N with a clean name.
func TestOrganizeIntegration(t *testing.T) {
	lib := t.TempDir()
	dl := t.TempDir()

	for _, d := range []string{"movies", "series", "anime", "music", "documents", "archives", "others"} {
		if err := os.MkdirAll(filepath.Join(lib, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// existing show folder
	showDir := filepath.Join(lib, "series", "Some Show", "Season 2")
	if err := os.MkdirAll(showDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// downloaded release file
	src := filepath.Join(dl, "Some.Show.S02E04.1080p.x264.Hindi.Korean.English.Msubs.RG.mkv")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := &Organizer{
		Paths: Paths{
			Movies: filepath.Join(lib, "movies"), Series: filepath.Join(lib, "series"),
			Anime: filepath.Join(lib, "anime"), Music: filepath.Join(lib, "music"),
			Documents: filepath.Join(lib, "documents"), Archives: filepath.Join(lib, "archives"),
			Others: filepath.Join(lib, "others"),
		},
		Logf: func(f string, v ...interface{}) { t.Logf(f, v...) },
	}

	res, err := o.OrganizeFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 1 {
		t.Fatalf("expected 1 move, got %v", res.Moved)
	}
	want := filepath.Join(showDir, "Some.Show.S02E04.mkv")
	if res.Moved[0] != want {
		t.Errorf("moved to %q, want %q", res.Moved[0], want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("file missing at destination: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source still exists")
	}
}

// TestOrganizeSeasonPackIntegration verifies a season pack routing.
func TestOrganizeSeasonPackIntegration(t *testing.T) {
	lib := t.TempDir()
	dl := t.TempDir()
	for _, d := range []string{"movies", "series", "anime", "music", "documents", "archives", "others"} {
		if err := os.MkdirAll(filepath.Join(lib, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// extracted episodes (as if unpacked from an archive)
	episodes := []string{
		"Mousetrap.S01E01.480p.x264.Hindi.mkv",
		"Mousetrap.S01E02.480p.x264.Hindi.mkv",
		"Mousetrap.S01E03.480p.x264.Hindi.mkv",
	}
	for i, e := range episodes {
		p := filepath.Join(dl, e)
		if err := os.WriteFile(p, []byte(strings.Repeat("x", i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		episodes[i] = p
	}

	o := &Organizer{
		Paths: Paths{
			Movies: filepath.Join(lib, "movies"), Series: filepath.Join(lib, "series"),
			Anime: filepath.Join(lib, "anime"), Music: filepath.Join(lib, "music"),
			Documents: filepath.Join(lib, "documents"), Archives: filepath.Join(lib, "archives"),
			Others: filepath.Join(lib, "others"),
		},
		Logf: func(f string, v ...interface{}) { t.Logf(f, v...) },
	}

	res, err := o.OrganizeSeasonPack(SeasonPackInput{
		SourceName:   "Mousetrap.S01.480p.x264.Hindi.Korean.English.Msubs.RG.zip",
		EpisodeFiles: episodes,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 3 {
		t.Fatalf("expected 3 moves, got %v", res.Moved)
	}
	base := filepath.Join(lib, "series", "Mousetrap", "Season 1")
	for i, m := range res.Moved {
		want := filepath.Join(base, sprintf("Mousetrap.S01E0%d.mkv", i+1))
		if m != want {
			t.Errorf("moved to %q, want %q", m, want)
		}
	}
	if res.SizeBytes != 6 {
		t.Errorf("size = %d, want 6", res.SizeBytes)
	}
}

// TestOrganizeNonEpisode verifies movies stay out of series (safety net).
func TestOrganizeNonEpisode(t *testing.T) {
	lib := t.TempDir()
	dl := t.TempDir()
	for _, d := range []string{"movies", "series", "anime", "music", "documents", "archives", "others"} {
		if err := os.MkdirAll(filepath.Join(lib, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(dl, "The.Legend.of.Hei.2019.1080p.BluRay.x264-WiKi.mkv")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := &Organizer{
		Paths: Paths{
			Movies: filepath.Join(lib, "movies"), Series: filepath.Join(lib, "series"),
			Anime: filepath.Join(lib, "anime"), Music: filepath.Join(lib, "music"),
			Documents: filepath.Join(lib, "documents"), Archives: filepath.Join(lib, "archives"),
			Others: filepath.Join(lib, "others"),
		},
	}
	res, err := o.OrganizeFile(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(lib, "movies", "The.Legend.of.Hei.2019.1080p.BluRay.x264-WiKi.mkv")
	if len(res.Moved) != 1 || res.Moved[0] != want {
		t.Errorf("expected movie in movies/, got %v", res.Moved)
	}
}

func sprintf(format string, v ...interface{}) string {
	return fmtSprintf(format, v...)
}
