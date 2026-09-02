package organize

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// TestArchivePipeline verifies: archive -> extract -> organize -> season pack
func TestArchivePipeline(t *testing.T) {
	lib := t.TempDir()
	dl := t.TempDir()
	for _, d := range []string{"movies", "series", "anime", "music", "documents", "archives", "others"} {
		if err := os.MkdirAll(filepath.Join(lib, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// build a season-pack zip
	zipPath := filepath.Join(dl, "Mousetrap.S01.480p.x264.Hindi.Korean.English.Msubs.RG.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for i := 1; i <= 5; i++ {
		name := sprintf("Mousetrap.S01E0%d.480p.x264.Hindi.mkv", i)
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// extract
	ex := NewTestExtractor()
	stage := filepath.Join(t.TempDir(), "stage")
	if err := ex.Extract(zipPath, stage, nil); err != nil {
		t.Fatal(err)
	}

	// inventory
	var episodes []string
	err = filepath.Walk(stage, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if IsVideo(filepath.Base(path)) && IsEpisode(filepath.Base(path)) {
			episodes = append(episodes, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 5 {
		t.Fatalf("expected 5 episodes, got %d", len(episodes))
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
		SourceName:   filepath.Base(zipPath),
		EpisodeFiles: episodes,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 5 {
		t.Fatalf("expected 5 moves, got %v", res.Moved)
	}
	// final structure: series/Mousetrap/Season 1/Mousetrap.S01E0N.mkv
	seasonDir := filepath.Join(lib, "series", "Mousetrap", "Season 1")
	for i := 1; i <= 5; i++ {
		want := filepath.Join(seasonDir, sprintf("Mousetrap.S01E0%d.mkv", i))
		if _, err := os.Stat(want); err != nil {
			t.Errorf("missing %s", want)
		}
	}
}
