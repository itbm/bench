//go:build linux

package term

import (
	"os"
	"strconv"
	"strings"
)

// processName returns the name of process pid, as the kernel records it.
func processName(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(b))
	return name, name != ""
}

// processDir returns the current folder of process pid.
func processDir(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	dir, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	if err != nil || !strings.HasPrefix(dir, "/") {
		return "", false
	}
	return dir, true
}
