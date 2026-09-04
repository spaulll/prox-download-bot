//go:build !windows

package telegram

import "syscall"

// hasFreeSpace reports whether path's filesystem has more than need bytes
// available. Returns true when the check cannot be performed (fail-open).
func hasFreeSpace(path string, need int64) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return true
	}
	avail := int64(st.Bavail) * int64(st.Bsize)
	return avail > need
}
