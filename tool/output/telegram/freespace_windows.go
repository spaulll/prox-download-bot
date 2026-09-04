//go:build windows

package telegram

// hasFreeSpace is fail-open on Windows: the Statfs syscall used on
// unix platforms does not exist here, so skip the pre-flight check.
func hasFreeSpace(path string, need int64) bool {
	return true
}
