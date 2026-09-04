package organize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSidecarFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sidecarOrganizer(lib string) *Organizer {
	return &Organizer{
		Paths: Paths{
			Movies:    filepath.Join(lib, "movies"),
			Series:    filepath.Join(lib, "series"),
			Anime:     filepath.Join(lib, "anime"),
			Music:     filepath.Join(lib, "music"),
			Documents: filepath.Join(lib, "documents"),
			Archives:  filepath.Join(lib, "archives"),
			Others:    filepath.Join(lib, "others"),
			Torrents:  filepath.Join(lib, "torrents"),
		},
	}
}

func TestIsSidecar(t *testing.T) {
	for _, n := range []string{"a.srt", "b.sub", "c.ssa", "d.ass", "e.vtt", "f.nfo", "g.jpg", "h.png", "i.txt"} {
		if !IsSidecar(n) {
			t.Errorf("expected sidecar: %s", n)
		}
	}
	for _, n := range []string{"a.mkv", "b.mp4", "c.zip", "d.rar", "e.mp3", "f.pdf", "g.torrent"} {
		if IsSidecar(n) {
			t.Errorf("unexpected sidecar: %s", n)
		}
	}
}

// Movie torrent dir: video + subs + artwork + notes stay together.
func TestOrganizeDirectoryKeepsSidecarsWithMovie(t *testing.T) {
	lib := t.TempDir()
	src := t.TempDir()
	writeSidecarFile(t, filepath.Join(src, "Example.Movie.2024.1080p.WEBRip.mp4"), "video")
	writeSidecarFile(t, filepath.Join(src, "Example.Movie.2024.1080p.WEBRip.srt"), "sub")
	writeSidecarFile(t, filepath.Join(src, "Example.Movie.2024.1080p.WEBRip.SDH.eng.srt"), "sub2")
	writeSidecarFile(t, filepath.Join(src, "cover.jpg"), "art")
	writeSidecarFile(t, filepath.Join(src, "readme.txt"), "notes")

	res, err := sidecarOrganizer(lib).OrganizeDirectory(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 5 {
		t.Fatalf("expected 5 moved, got %d (%v)", len(res.Moved), res.Moved)
	}
	// every moved file must sit in the same directory (next to the movie)
	dir := filepath.Dir(res.Moved[0])
	for _, m := range res.Moved[1:] {
		if filepath.Dir(m) != dir {
			t.Errorf("sidecar split: %s not next to %s", m, res.Moved[0])
		}
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected source dir removed, stat err: %v", err)
	}
}

// Season pack dir: each episode keeps its own subtitles.
func TestOrganizeDirectorySidecarsFollowOwnEpisode(t *testing.T) {
	lib := t.TempDir()
	if err := makeDirs(filepath.Join(lib, "series"), "Some Show"); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	writeSidecarFile(t, filepath.Join(src, "Some.Show.S02E04.1080p.mkv"), "ep4")
	writeSidecarFile(t, filepath.Join(src, "Some.Show.S02E04.srt"), "sub4")
	writeSidecarFile(t, filepath.Join(src, "Some.Show.S02E05.1080p.mkv"), "ep5")
	writeSidecarFile(t, filepath.Join(src, "Some.Show.S02E05.srt"), "sub5")

	res, err := sidecarOrganizer(lib).OrganizeDirectory(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 4 {
		t.Fatalf("expected 4 moved, got %d (%v)", len(res.Moved), res.Moved)
	}
	byDir := map[string][]string{}
	for _, m := range res.Moved {
		byDir[filepath.Dir(m)] = append(byDir[filepath.Dir(m)], filepath.Base(m))
	}
	for d, names := range byDir {
		t.Logf("%s: %v", d, names)
	}
	// E04 video and its sub must share a directory, same for E05
	loc := map[string]string{}
	for _, m := range res.Moved {
		loc[filepath.Base(m)] = filepath.Dir(m)
	}
	var ep4, sub4, ep5, sub5 string
	for base, d := range loc {
		switch {
		case strings.Contains(base, "E04") && filepath.Ext(base) == ".mkv":
			ep4 = d
		case strings.Contains(base, "E04") && filepath.Ext(base) == ".srt":
			sub4 = d
		case strings.Contains(base, "E05") && filepath.Ext(base) == ".mkv":
			ep5 = d
		case strings.Contains(base, "E05") && filepath.Ext(base) == ".srt":
			sub5 = d
		}
	}
	if ep4 == "" || ep4 != sub4 {
		t.Errorf("E04 video/sub split: %q vs %q", ep4, sub4)
	}
	if ep5 == "" || ep5 != sub5 {
		t.Errorf("E05 video/sub split: %q vs %q", ep5, sub5)
	}
}

func TestCleanMovieFolderName(t *testing.T) {
	cases := map[string]string{
		"Example Movie (2024) [1080p] [WEBRip] [5.1] [DEMO]":     "Example Movie (2024)",
		"Example.Movie.2024.1080p.WEBRip.x264.AAC5.1-DEMO.mp4": "Example Movie (2024)",
		"Example Movie (2024)":                  "Example Movie (2024)",
		"Sample.Film.2023.1080p.WEB-DL.mkv": "Sample Film (2023)",
	}
	for in, want := range cases {
		if got := CleanMovieFolderName(in); got != want {
			t.Errorf("CleanMovieFolderName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := CleanMovieFolderName("2012"); got != "" {
		t.Errorf("expected empty for year-only name, got %q", got)
	}
}

// Non-episodic torrent dir: everything lands together in one movie folder.
func TestOrganizeDirectoryGroupsRelease(t *testing.T) {
	lib := t.TempDir()
	src := filepath.Join(t.TempDir(), "Example.Movie.2024.1080p.WEBRip")
	writeSidecarFile(t, filepath.Join(src, "Example.Movie.2024.1080p.WEBRip.mp4"), "video")
	writeSidecarFile(t, filepath.Join(src, "Example.Movie.2024.1080p.WEBRip.srt"), "sub")
	writeSidecarFile(t, filepath.Join(src, "cover.jpg"), "art")

	res, err := sidecarOrganizer(lib).OrganizeDirectory(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 3 {
		t.Fatalf("expected 3 moved, got %d (%v)", len(res.Moved), res.Moved)
	}
	wantDir := filepath.Join(lib, "movies", "Example Movie (2024)")
	for _, m := range res.Moved {
		if filepath.Dir(m) != wantDir {
			t.Errorf("expected everything in %s, got %s", wantDir, m)
		}
	}
	if res.Category != CatMovies {
		t.Errorf("expected category movies, got %q", res.Category)
	}
}

// Single-file torrent movie: dedicated folder, not the category root.
func TestOrganizeTorrentFileGroupsMovie(t *testing.T) {
	lib := t.TempDir()
	src := t.TempDir()
	f := filepath.Join(src, "Sample.Film.2023.1080p.WEB-DL.mkv")
	writeSidecarFile(t, f, "video")

	res, err := sidecarOrganizer(lib).OrganizeTorrentFile(f, "Sample Film (2023) [1080p] [WEB-DL]", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 1 {
		t.Fatalf("expected 1 moved, got %v", res.Moved)
	}
	want := filepath.Join(lib, "movies", "Sample Film (2023)", "Sample.Film.2023.1080p.WEB-DL.mkv")
	if res.Moved[0] != want {
		t.Errorf("got %q, want %q", res.Moved[0], want)
	}
}

// Single-file torrent episode: keeps series placement, no movie folder.
func TestOrganizeTorrentFileKeepsSeriesPlacement(t *testing.T) {
	lib := t.TempDir()
	if err := makeDirs(filepath.Join(lib, "series"), "Some Show"); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	f := filepath.Join(src, "Some.Show.S02E04.1080p.mkv")
	writeSidecarFile(t, f, "ep")

	res, err := sidecarOrganizer(lib).OrganizeTorrentFile(f, "Some.Show.S02E04.1080p", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 1 {
		t.Fatalf("expected 1 moved, got %v", res.Moved)
	}
	want := filepath.Join(lib, "series", "Some Show", "Season 2", "Some.Show.S02E04.mkv")
	if res.Moved[0] != want {
		t.Errorf("got %q, want %q", res.Moved[0], want)
	}
}
