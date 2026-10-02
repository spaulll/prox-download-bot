package telegram

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	i18nLoc "DownloadBot/i18n"
	logger "DownloadBot/tool/zap"
)

func init() {
	// diskGuardError logs refusals and renders i18n text; both globals are
	// only set up by the real binary, so initialize them for tests.
	logger.InitLog("", "", "error")
	i18nLoc.LocLan("en")
}

func TestProbeLinkSize(t *testing.T) {
	withLen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "12345")
	}))
	defer withLen.Close()
	if n, ok := probeLinkSize(withLen.URL + "/file.zip"); !ok || n != 12345 {
		t.Errorf("probe with length = %d,%v want 12345,true", n, ok)
	}

	noLen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer noLen.Close()
	if _, ok := probeLinkSize(noLen.URL + "/file.zip"); ok {
		t.Errorf("probe without length: ok=true, want false (fail-open)")
	}

	if _, ok := probeLinkSize("magnet:?xt=urn:btih:ABCDEF"); ok {
		t.Errorf("probe magnet: ok=true, want false")
	}
}

func TestDiskGuardError(t *testing.T) {
	huge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "9223372036854775807")
	}))
	defer huge.Close()
	msg := diskGuardErrorFor(huge.URL+"/big.zip", t.TempDir())
	if msg == "" {
		t.Fatalf("diskGuardError(huge) = empty, want refusal")
	}
	if !strings.Contains(msg, "Not enough disk space") {
		t.Errorf("refusal missing headline: %q", msg)
	}

	fit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
	}))
	defer fit.Close()
	if msg := diskGuardErrorFor(fit.URL+"/small.zip", t.TempDir()); msg != "" {
		t.Errorf("diskGuardError(tiny) = %q, want empty (fits)", msg)
	}

	if msg := diskGuardError("magnet:?xt=urn:btih:ABCDEF"); msg != "" {
		t.Errorf("diskGuardError(magnet) = %q, want empty (unknown size allows)", msg)
	}
}

func TestDiskGuardRefusalFormat(t *testing.T) {
	huge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(int64(9)<<30))
	}))
	defer huge.Close()
	msg := diskGuardErrorFor(huge.URL+"/Show.S01.1080p.zip", t.TempDir())
	for _, want := range []string{"Show.S01.1080p.zip", "GB", "Free:"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q missing %q", msg, want)
		}
	}
}
