// Package extract implements safe archive extraction with live progress.
//
// Copyright 2026 spaulll - prox-download-bot (Apache-2.0)
package extract

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Progress reports extraction progress: files done / total.
type Progress func(done, total int, current string)

// Extractor extracts archives into a destination directory.
type Extractor struct {
	// Logf logs every decision
	Logf func(format string, v ...interface{})
}

func (e *Extractor) log(format string, v ...interface{}) {
	if e.Logf != nil {
		e.Logf(format, v...)
	}
}

// IsArchivePath reports whether the file name has a supported archive extension.
func IsArchivePath(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz", ".tgz", ".tbz2":
		return true
	}
	return false
}

// MultiVolumePrefix returns the base name of a multi-volume rar part, or "".
// "Show.part1.rar" and "Show.part01.rar" both belong to "Show".
func MultiVolumePrefix(name string) string {
	base := filepath.Base(name)
	lower := strings.ToLower(base)
	for _, ext := range []string{".rar", ".r00", ".r01"} {
		if strings.HasSuffix(lower, ext) {
			stem := base[:len(base)-len(ext)]
			if i := strings.LastIndex(stem, ".part"); i > 0 {
				return stem[:i]
			}
			return stem
		}
	}
	return ""
}

// IsFirstVolume reports whether an archive is the first volume of a set
// (or a single-volume archive). Multi-part sets must only be extracted once.
func IsFirstVolume(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	// "xxx.part001.rar" style: only extract when part number is 0/1
	if i := strings.LastIndex(lower, ".part"); i >= 0 {
		num := lower[i+5:]
		num = strings.TrimSuffix(num, filepath.Ext(num))
		trimmed := strings.TrimLeft(num, "0")
		return trimmed == "" || trimmed == "1"
	}
	// ".r00"/".r01" style: first volume is ".rar"
	if strings.HasSuffix(lower, ".r00") || strings.HasSuffix(lower, ".r01") {
		return false
	}
	return true
}

// ErrUnsupported is returned when no extraction backend is available.
var ErrUnsupported = errors.New("no extraction backend available for this archive")

// Extract extracts the archive at src into destDir, reporting progress.
// Backends, in order: 7z, unrar/unzip (native), native zip, native tar*.
func (e *Extractor) Extract(src, destDir string, progress Progress) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	lower := strings.ToLower(src)

	switch {
	case hasBin("7z"):
		return e.extract7z(src, destDir, progress)
	case strings.HasSuffix(lower, ".rar") && hasBin("unrar"):
		return e.extractUnrar(src, destDir, progress)
	case strings.HasSuffix(lower, ".zip") && hasBin("unzip"):
		return e.extractUnzip(src, destDir, progress)
	case strings.HasSuffix(lower, ".zip"):
		return e.extractNativeZip(src, destDir, progress)
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".tgz"),
		strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tar.bz2"),
		strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".gz"),
		strings.HasSuffix(lower, ".bz2"), strings.HasSuffix(lower, ".xz"):
		return e.extractTar(src, destDir, progress)
	default:
		return ErrUnsupported
	}
}

// ExtractAll flattens multi-volume archives: extracts the first volume of a
// set. Returns the number of archives skipped as later volumes.
func (e *Extractor) ExtractAll(src, destDir string, progress Progress) (int, error) {
	if !IsFirstVolume(src) {
		return 1, nil
	}
	return 0, e.Extract(src, destDir, progress)
}

func hasBin(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (e *Extractor) extract7z(src, destDir string, progress Progress) error {
	// count entries for progress
	total := 0
	out, err := exec.Command("7z", "l", "-ba", "-slt", src).Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Path = ") {
				total++
			}
		}
	}
	if total == 0 {
		total = 1
	}
	done := 0
	cmd := exec.Command("7z", "x", "-y", "-o"+destDir, src)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("7z extract failed: %w", err)
	}
	// report per-file progress after extraction by counting files
	fileCount := countFiles(destDir)
	done = 0
	_ = filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		done++
		if progress != nil {
			progress(done, fileCount, filepath.Base(path))
		}
		return nil
	})
	if progress != nil && fileCount == 0 {
		progress(1, 1, filepath.Base(src))
	}
	return nil
}

func (e *Extractor) extractUnrar(src, destDir string, progress Progress) error {
	cmd := exec.Command("unrar", "x", "-y", src, destDir+string(filepath.Separator))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unrar extract failed: %w", err)
	}
	reportFileProgress(destDir, progress)
	return nil
}

func (e *Extractor) extractUnzip(src, destDir string, progress Progress) error {
	cmd := exec.Command("unzip", "-o", "-q", src, "-d", destDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unzip extract failed: %w", err)
	}
	reportFileProgress(destDir, progress)
	return nil
}

func reportFileProgress(destDir string, progress Progress) {
	if progress == nil {
		return
	}
	total := countFiles(destDir)
	done := 0
	_ = filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		done++
		progress(done, total, filepath.Base(path))
		return nil
	})
	if total == 0 {
		progress(1, 1, "")
	}
}

func countFiles(dir string) int {
	n := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// extractNativeZip handles zip archives in pure Go with per-file progress.
func (e *Extractor) extractNativeZip(src, destDir string, progress Progress) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	total := 0
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "__MACOSX") {
			total++
		}
	}
	done := 0
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "__MACOSX") {
			continue
		}
		name, err := safeJoin(destDir, f.Name)
		if err != nil {
			e.log("skipping unsafe zip entry %q: %v", f.Name, err)
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(name, 0o755); err != nil {
				return err
			}
			done++
			if progress != nil {
				progress(done, total, filepath.Base(f.Name))
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(f, name); err != nil {
			return err
		}
		done++
		if progress != nil {
			progress(done, total, filepath.Base(f.Name))
		}
	}
	return nil
}

func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// extractTar handles tar, tar.gz, tar.bz2 and single-file .gz/.bz2/.xz.
func (e *Extractor) extractTar(src, destDir string, progress Progress) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	lower := strings.ToLower(src)
	var tr *tar.Reader

	openWrapped := func(base io.Reader, kind string) (io.Reader, error) {
		switch kind {
		case "gz":
			return gzip.NewReader(base)
		case "bz2":
			return io.Reader(bzip2.NewReader(base)), nil
		default:
			return base, nil
		}
	}

	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"), strings.HasSuffix(lower, ".gz"):
		wrapped, err := openWrapped(f, "gz")
		if err != nil {
			return err
		}
		tr = tar.NewReader(wrapped)
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".bz2"):
		wrapped, _ := openWrapped(f, "bz2")
		tr = tar.NewReader(wrapped)
	default:
		wrapped, err := openWrapped(f, "")
		if err != nil {
			return err
		}
		tr = tar.NewReader(wrapped)
	}

	// count entries first
	total := 0
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		total++
	}
	if total == 0 {
		return ErrUnsupported
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var reader io.Reader = f
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"), strings.HasSuffix(lower, ".gz"):
		wrapped, err := openWrapped(f, "gz")
		if err != nil {
			return err
		}
		reader = wrapped
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".bz2"):
		wrapped, _ := openWrapped(f, "bz2")
		reader = wrapped
	default:
		wrapped, err := openWrapped(f, "")
		if err != nil {
			return err
		}
		reader = wrapped
	}
	tr = tar.NewReader(reader)

	done := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			e.log("skipping unsafe tar entry %q: %v", hdr.Name, err)
			done++
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(name, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
		done++
		if progress != nil {
			progress(done, total, filepath.Base(hdr.Name))
		}
	}
	return nil
}

// safeJoin prevents path traversal ("zip slip").
func safeJoin(destDir, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("unsafe path: %q", name)
	}
	return filepath.Join(destDir, clean), nil
}
