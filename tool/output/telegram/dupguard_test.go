package telegram

import (
	"os"
	"path/filepath"
	"testing"

	"DownloadBot/internal/users"
)

func makeDupLib(t *testing.T) (movies, series, anime string) {
	t.Helper()
	lib := t.TempDir()
	movies, series, anime = filepath.Join(lib, "movies"), filepath.Join(lib, "series"), filepath.Join(lib, "anime")
	// series/Neagley/Season 1 with 2 episodes
	ep1 := filepath.Join(series, "Neagley", "Season 1")
	if err := os.MkdirAll(ep1, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Neagley.S01E01.mkv", "Neagley.S01E02.mkv"} {
		if err := os.WriteFile(filepath.Join(ep1, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// movies/Some Movie (2024) with 1 file
	mv := filepath.Join(movies, "Some Movie (2024)")
	if err := os.MkdirAll(mv, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mv, "Some Movie (2024).mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return movies, series, anime
}

func TestFindLibraryDuplicate(t *testing.T) {
	movies, series, anime := makeDupLib(t)

	hit := findLibraryDuplicateIn("Neagley.S01.1080P.MoviesMod.zone.zip", movies, series, anime)
	if hit == nil || hit.Kind != "season" {
		t.Fatalf("season pack hit = %+v, want season", hit)
	}

	hit = findLibraryDuplicateIn("Neagley.S01E01.1080p.x264.mkv", movies, series, anime)
	if hit == nil || hit.Kind != "episode" {
		t.Fatalf("present episode hit = %+v, want episode", hit)
	}

	if hit := findLibraryDuplicateIn("Neagley.S01E09.1080p.x264.mkv", movies, series, anime); hit != nil {
		t.Fatalf("missing episode hit = %+v, want nil (new episode)", hit)
	}

	if hit := findLibraryDuplicateIn("Unknown.Show.S01.1080p.GRP.zip", movies, series, anime); hit != nil {
		t.Fatalf("unknown show hit = %+v, want nil", hit)
	}

	hit = findLibraryDuplicateIn("Some.Movie.2024.1080p.WEB.mkv", movies, series, anime)
	if hit == nil || hit.Kind != "movie" {
		t.Fatalf("movie hit = %+v, want movie", hit)
	}

	if hit := findLibraryDuplicateIn("readme.txt", movies, series, anime); hit != nil {
		t.Fatalf("non-media hit = %+v, want nil", hit)
	}

	if hit := findLibraryDuplicateIn("magnet", movies, series, anime); hit != nil {
		t.Fatalf("magnet hit = %+v, want nil", hit)
	}
}

func TestFindActiveDownload(t *testing.T) {
	taskStore = users.NewTaskStore(t.TempDir() + "/tasks.json")
	taskStore.Add(users.Task{GID: "g1", UserID: 100, Link: "http://x/file.zip", Engine: "aria2", Status: "downloading"})
	taskStore.Add(users.Task{GID: "g2", UserID: 100, Link: "http://y/other.zip", Engine: "aria2", Status: "completed"})

	if dup := findActiveDownload("http://x/file.zip"); dup == nil || dup.GID != "g1" {
		t.Fatalf("active lookup = %+v, want g1", dup)
	}
	if dup := findActiveDownload("http://y/other.zip"); dup != nil {
		t.Fatalf("completed lookup = %+v, want nil", dup)
	}
	if dup := findActiveDownload("http://z/new.zip"); dup != nil {
		t.Fatalf("unknown lookup = %+v, want nil", dup)
	}
}

func TestDupPendingRoundTrip(t *testing.T) {
	key := rememberDupPending("http://x/file.zip", 100, 200, "bob")
	p := takeDupPending(key)
	if p == nil || p.Link != "http://x/file.zip" || p.UserID != 100 {
		t.Fatalf("round trip = %+v, want entry", p)
	}
	if again := takeDupPending(key); again != nil {
		t.Fatalf("second take = %+v, want nil (consumed)", again)
	}
	if missing := takeDupPending("dup-nope"); missing != nil {
		t.Fatalf("unknown take = %+v, want nil", missing)
	}
}
