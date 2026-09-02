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
	"regexp"
	"strconv"
	"strings"
)

// Info describes extraction progress at a point in time.
type Info struct {
	Done       int     // entries processed
	Total      int     // total entries (0 when unknown)
	Percent    float64 // 0..100 (byte-accurate when BytesTotal is set)
	BytesDone  int64
	BytesTotal int64
	Current    string // file name being extracted (optional)
}

// Progress reports extraction progress.
type Progress func(i Info)

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
	case strings.HasSuffix(lower, ".zip"):
		// native first: byte-accurate live progress (7z hides it in redraws)
		if err := e.extractNativeZip(src, destDir, progress); err == nil {
			return nil
		} else if !hasBin("7z") {
			return err
		} else {
			e.log("native zip failed (%v), retrying with 7z", err)
		}
		return e.extract7z(src, destDir, progress)
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".tgz"),
		strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tar.bz2"),
		strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".gz"),
		strings.HasSuffix(lower, ".bz2"), strings.HasSuffix(lower, ".xz"):
		if err := e.extractTar(src, destDir, progress); err == nil {
			return nil
		} else if !hasBin("7z") {
			return err
		} else {
			e.log("native tar failed (%v), retrying with 7z", err)
		}
		return e.extract7z(src, destDir, progress)
	case hasBin("7z"):
		return e.extract7z(src, destDir, progress)
	case strings.HasSuffix(lower, ".rar") && hasBin("unrar"):
		return e.extractUnrar(src, destDir, progress)
	case strings.HasSuffix(lower, ".zip") && hasBin("unzip"):
		return e.extractUnzip(src, destDir, progress)
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
	bytesTotal := archiveUncompressedSize(src)

	// -bso0 silences normal output, -bsp1 routes the progress meter to stdout.
	// The meter redraws in place with backspaces; parse the visible characters
	// like a terminal would to get live percentages.
	cmd := exec.Command("7z", "x", "-y", "-bso0", "-bsp1", "-o"+destDir, src)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("7z extract failed: %w", err)
	}

	pctRe := regexp.MustCompile(`(\d{1,3})\s*%`)
	var vis []byte // visible text between redraw control chars
	report := func() {
		if progress == nil {
			return
		}
		if m := pctRe.FindSubmatch(vis); m != nil {
			pct, _ := strconv.ParseFloat(string(m[1]), 64)
			progress(Info{
				Total:      100,
				Done:       int(pct),
				Percent:    pct,
				BytesTotal: bytesTotal,
				BytesDone:  int64(pct / 100.0 * float64(bytesTotal)),
			})
		}
	}
	buf := make([]byte, 32*1024)
	for {
		n, rerr := stdout.Read(buf)
		for _, c := range buf[:n] {
			switch c {
			case '\b':
				if len(vis) > 0 {
					vis = vis[:len(vis)-1]
				}
			case '\r', '\n':
				vis = vis[:0]
			default:
				// cap so a chatty stream can never grow unbounded
				if len(vis) < 512 {
					vis = append(vis, c)
				}
			}
		}
		if n > 0 {
			report()
		}
		if rerr != nil {
			break
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("7z extract failed: %w", err)
	}
	if progress != nil {
		progress(Info{Total: 100, Done: 100, Percent: 100,
			BytesTotal: bytesTotal, BytesDone: bytesTotal})
	}
	return nil
}

// archiveUncompressedSize sums the uncompressed sizes of all entries via the
// 7z listing. Returns 0 when unavailable.
func archiveUncompressedSize(src string) int64 {
	out, err := exec.Command("7z", "l", "-ba", "-slt", src).Output()
	if err != nil {
		return 0
	}
	var total int64
	for _, line := range strings.Split(string(out), "\n") {
		if s, ok := strings.CutPrefix(line, "Size = "); ok {
			if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				total += v
			}
		}
	}
	return total
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

// reportFileProgress walks already-extracted files; a fallback for backends
// without native live progress (rarely hit - 7z/native cover most archives).
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
		progress(Info{Done: done, Total: total,
			Percent: float64(done) / float64(total) * 100, Current: filepath.Base(path)})
		return nil
	})
	if total == 0 {
		progress(Info{Done: 1, Total: 1, Percent: 100})
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

// extractNativeZip handles zip archives in pure Go with byte-accurate
// per-file progress.
func (e *Extractor) extractNativeZip(src, destDir string, progress Progress) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	total := 0
	var bytesTotal int64
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "__MACOSX") {
			continue
		}
		total++
		bytesTotal += int64(f.UncompressedSize64)
	}
	done := 0
	var bytesDone int64
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
				progress(Info{Done: done, Total: total,
					Percent: bytePercent(bytesDone, bytesTotal),
					BytesDone: bytesDone, BytesTotal: bytesTotal,
					Current: filepath.Base(f.Name)})
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		var fileDone int64
		if err := extractZipFile(f, name, func(n int64) {
			// live intra-file progress (huge files update continuously)
			fileDone += n
			if progress != nil {
				progress(Info{Done: done, Total: total,
					Percent: bytePercent(bytesDone+fileDone, bytesTotal),
					BytesDone: bytesDone + fileDone, BytesTotal: bytesTotal,
					Current: filepath.Base(f.Name)})
			}
		}); err != nil {
			return err
		}
		done++
		bytesDone += int64(f.UncompressedSize64)
		if progress != nil {
			progress(Info{Done: done, Total: total,
				Percent: bytePercent(bytesDone, bytesTotal),
				BytesDone: bytesDone, BytesTotal: bytesTotal,
				Current: filepath.Base(f.Name)})
		}
	}
	return nil
}

func bytePercent(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(done) / float64(total) * 100
}

// progressReader counts bytes passing through and calls onRead per chunk.
type progressReader struct {
	r      io.Reader
	onRead func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 && p.onRead != nil {
		p.onRead(int64(n))
	}
	return n, err
}

func extractZipFile(f *zip.File, dest string, onRead func(int64)) error {
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
	var r io.Reader = rc
	if onRead != nil {
		r = &progressReader{r: rc, onRead: onRead}
	}
	_, err = io.Copy(out, r)
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
	var bytesTotal int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		total++
		bytesTotal += hdr.Size
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
	var bytesDone int64
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
			var fileDone int64
			var src io.Reader = tr
			if progress != nil {
				src = &progressReader{r: tr, onRead: func(n int64) {
					fileDone += n
					progress(Info{Done: done, Total: total,
						Percent: bytePercent(bytesDone+fileDone, bytesTotal),
						BytesDone: bytesDone + fileDone, BytesTotal: bytesTotal,
						Current: filepath.Base(hdr.Name)})
				}}
			}
			_, copyErr := io.Copy(out, src)
			out.Close()
			if copyErr != nil {
				return copyErr
			}
		}
		done++
		bytesDone += hdr.Size
		if progress != nil {
			progress(Info{Done: done, Total: total,
				Percent: bytePercent(bytesDone, bytesTotal),
				BytesDone: bytesDone, BytesTotal: bytesTotal,
				Current: filepath.Base(hdr.Name)})
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
