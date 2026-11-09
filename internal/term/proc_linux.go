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

// processParent returns the parent of process pid: the process that
// started it, while that runs.
func processParent(pid int) (int, bool) {
	if pid <= 0 {
		return 0, false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	// The line is "pid (name) state parent ...". The name can hold spaces
	// and brackets, so the fields that follow it start after the last ")".
	stat := string(b)
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0, false
	}
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	return ppid, err == nil && ppid > 0
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
