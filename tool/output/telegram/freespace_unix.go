//go:build !windows

package telegram

import "syscall"

// hasFreeSpace reports whether path's filesystem has more than need bytes
// available. Returns true when the check cannot be performed (fail-open).
func hasFreeSpace(path string, need int64) bool {
	free, ok := freeSpaceBytes(path)
	if !ok {
		return true
	}
	return free > need
}

// freeSpaceBytes returns the bytes available to unprivileged callers on
// path's filesystem. ok=false when the check cannot be performed.
func freeSpaceBytes(path string) (free int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true
}
