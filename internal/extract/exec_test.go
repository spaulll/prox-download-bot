package extract

import "os/exec"

func exec7zImpl(args ...string) (string, error) {
	out, err := exec.Command("7z", args...).CombinedOutput()
	return string(out), err
}
