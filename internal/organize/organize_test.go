package organize

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"Some.Show.S02E04.1080p.x264.Hindi.Korean.English.Msubs.RG.mkv": "some show",
		"Attack.on.Titan.S04E05.1080p.BluRay.x265.mkv":                                   "attack on titan",
		"The.Legend.of.Hei.2019.1080p.BluRay.x264-WiKi.mkv":                              "the legend of hei",
		"One.Piece.1000.1080p.mkv":                                                       "one piece",
		"Mousetrap.S01.480p.x264.Hindi.Korean.English.Msubs.RG":               "mousetrap",
		"Show.Name.2020.WEB-DL.mkv":                                                      "show name",
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFolderName(t *testing.T) {
	cases := map[string]string{
		"Show: Subtitle":     "Show - Subtitle",
		`A*B?C"D<E>F|G`:      "ABCDEFG",
		"trailing...dots...": "trailing...dots",
		"  spaced   out  ":   "spaced out",
		"path/slash\\test":   "pathslashtest",
		"Anime: The Rise: X": "Anime - The Rise - X",
	}
	for in, want := range cases {
		if got := SanitizeFolderName(in); got != want {
			t.Errorf("SanitizeFolderName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"Some.Show.S02E04.mkv": "Some.Show.S02E04.mkv",
		"video:name?.mkv":            "videoname.mkv",
		"a<b>c|d.mkv":                "abcd.mkv",
	}
	for in, want := range cases {
		if got := SanitizeFileName(in); got != want {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsEpisode(t *testing.T) {
	trueCases := []string{
		"Show.S01E01.mkv", "Show.s1e1.mkv", "Show.1x01.mkv", "show s01 e02.mkv",
		"Show.S01.E01.mkv", "Show.E05.mkv", "Show - 1x02 - title.mkv",
	}
	falseCases := []string{
		"The.Legend.of.Hei.2019.1080p.mkv", "Movie.2020.mkv", "Inception.mkv",
		"Blade.Runner.2049.mkv", "1917.2019.mkv",
	}
	for _, c := range trueCases {
		if !IsEpisode(c) {
			t.Errorf("IsEpisode(%q) = false, want true", c)
		}
	}
	for _, c := range falseCases {
		if IsEpisode(c) {
			t.Errorf("IsEpisode(%q) = true, want false", c)
		}
	}
}

func TestExtractSeason(t *testing.T) {
	cases := map[string]int{
		"Show.S04E06.mkv":      4,
		"Show.s4e6.mkv":        4,
		"Show.4x06.mkv":        4,
		"Show.s04.e06.mkv":     4,
		"Show.S01E01.720p.mkv": 1,
		"Mousetrap.S01.480p":   1,
		"Show.S12E03.mkv":      12,
		"The.Matrix.1999.mkv":  0,
		"Random.Show.mkv":      0,
	}
	for in, want := range cases {
		if got := ExtractSeason(in); got != want {
			t.Errorf("ExtractSeason(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestExtractEpisode(t *testing.T) {
	cases := map[string]int{
		"Show.S04E06.mkv": 6,
		"Show.4x06.mkv":   6,
		"Show.E05.mkv":    5,
		"Show.S01E12.mkv": 12,
		"No.Episode.mkv":  0,
	}
	for in, want := range cases {
		if got := ExtractEpisode(in); got != want {
			t.Errorf("ExtractEpisode(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestCleanEpisodeFileName(t *testing.T) {
	cases := map[string]string{
		"Some.Show.S02E04.1080p.x264.Hindi.Korean.English.Msubs.RG.mkv": "Some.Show.S02E04.mkv",
		"Mousetrap.S01E01.480p.x264.mkv":                                                 "Mousetrap.S01E01.mkv",
		"Show.1x05.1080p.mkv":                                                            "Show.1x05.mkv",
		"Attack.on.Titan.S04E05.1080p.BluRay.x265.mkv":                                   "Attack.on.Titan.S04E05.mkv",
		"Show.S02E04.mkv":                                                                "Show.S02E04.mkv",
	}
	for in, want := range cases {
		if got := CleanEpisodeFileName(in); got != want {
			t.Errorf("CleanEpisodeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanSeasonPackName(t *testing.T) {
	cases := map[string]string{
		"Mousetrap.S01.480p.x264.Hindi.Korean.English.Msubs.RG": "Mousetrap",
		"Some.Show.S02":        "Some Show",
		"Show.Name.S01E01-E05.1080p": "Show Name",
		"One Piece S05 1080p BluRay": "One Piece",
	}
	for in, want := range cases {
		if got := CleanSeasonPackName(in); got != want {
			t.Errorf("CleanSeasonPackName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitleCase(t *testing.T) {
	cases := map[string]string{
		"Some Show":   "Some Show",
		"the legend of hei": "The Legend Of Hei",
		"mousetrap":         "Mousetrap",
	}
	for in, want := range cases {
		if got := TitleCase(in); got != want {
			t.Errorf("TitleCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTree(t *testing.T) {
	paths := []string{
		"/lib/series/Mousetrap/Season 1/Mousetrap.S01E01.mkv",
		"/lib/series/Mousetrap/Season 1/Mousetrap.S01E02.mkv",
		"/lib/series/Mousetrap/Season 1/Mousetrap.S01E03.mkv",
	}
	roots := map[string]string{"Series": "/lib/series", "Movies": "/lib/movies"}
	got := Tree(paths, roots)
	want := "Series\n└── Mousetrap\n    └── Season 1\n        ├── Mousetrap.S01E01.mkv\n        ├── Mousetrap.S01E02.mkv\n        └── Mousetrap.S01E03.mkv"
	if got != want {
		t.Errorf("Tree mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFolderMatcher(t *testing.T) {
	root := t.TempDir()
	if err := makeDirs(root, "Some Show", "Game Of Silence", "Mousetrap"); err != nil {
		t.Fatal(err)
	}
	m := FolderMatcher{Root: root}

	if got := m.Find("Some Show"); filepath_Base(got) != "Some Show" {
		t.Errorf("exact match failed: %q", got)
	}
	// token overlap: "Some Show s02" should match "Some Show"
	if got := m.Find("Some Show s02"); filepath_Base(got) != "Some Show" {
		t.Errorf("token match failed: %q", got)
	}
	// no confident match
	if got := m.Find("totally unknown show"); got != "" {
		t.Errorf("expected no match, got %q", got)
	}
}

func filepath_Base(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

func makeDirs(root string, names ...string) error {
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			return err
		}
	}
	return nil
}
