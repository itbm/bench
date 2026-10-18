//go:build linux

package vscode

import (
	"os"
	"strconv"
	"strings"
)

// pfExiting is PF_EXITING from <linux/sched.h>: the process is ending. It's
// set before the process's files close.
const pfExiting = 0x4

// lookUpProcess returns what the kernel says about process pid: when it
// started (in clock ticks since boot, after the boot's ID) and whether it
// has ended or is ending. It returns false if there's no such process.
func lookUpProcess(pid int) (process, bool) {
	if pid <= 0 {
		return process{}, false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return process{}, false
	}
	// The command name, in parentheses, can hold spaces and parentheses;
	// the fields after the last ")" start with the state (field 3), so the
	// flags (field 9) are the 7th and the start time (field 22) the 20th.
	i := strings.LastIndexByte(string(stat), ')')
	if i < 0 {
		return process{}, false
	}
	fields := strings.Fields(string(stat[i+1:]))
	if len(fields) < 20 {
		return process{}, false
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return process{}, false
	}
	flags, _ := strconv.ParseUint(fields[6], 10, 64)
	return process{
		started: strings.TrimSpace(string(boot)) + "/" + fields[19],
		ended:   fields[0] == "Z" || fields[0] == "X" || flags&pfExiting != 0,
	}, true
}
