package telegram

import (
	i18nLoc "DownloadBot/i18n"
	"DownloadBot/internal/config"
	"DownloadBot/tool/typeTrans"
	logger "DownloadBot/tool/zap"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// probeClient issues HEAD requests for add-time size checks.
var probeClient = &http.Client{Timeout: 10 * time.Second}

// probeLinkSize returns the advertised size of an http(s) link via HEAD.
// ok=false when the size is unknown (not http(s), HEAD unsupported, missing
// Content-Length, any error) — callers fail open and allow the download.
func probeLinkSize(link string) (size int64, ok bool) {
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return 0, false
	}
	req, err := http.NewRequest(http.MethodHead, link, nil)
	if err != nil {
		return 0, false
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, false
	}
	cl := strings.TrimSpace(resp.Header.Get("Content-Length"))
	if cl == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(cl, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// diskGuardError returns a user-facing refusal when link provably does not
// fit the download filesystem (size known via HEAD and free space smaller).
// Returns "" when the download may proceed (unknown size or fits).
func diskGuardError(link string) string {
	return diskGuardErrorFor(link, config.GetDownloadFolder())
}

// diskGuardErrorFor is diskGuardError with an explicit folder (test seam).
func diskGuardErrorFor(link, downloadFolder string) string {
	size, ok := probeLinkSize(link)
	if !ok {
		return ""
	}
	dl := downloadFolder
	if hasFreeSpace(dl, size) {
		return ""
	}
	free, freeOK := freeSpaceBytes(dl)
	freeStr := "unknown"
	if freeOK {
		freeStr = typeTrans.Byte2Readable(float64(free))
	}
	logger.Error("disk guard refused %q: need %s, free %s", link, typeTrans.Byte2Readable(float64(size)), freeStr)
	return fmt.Sprintf(i18nLoc.LocText("diskGuardNoSpace"),
		shortDisplayName(link), typeTrans.Byte2Readable(float64(size)), freeStr)
}
