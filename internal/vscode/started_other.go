//go:build !darwin && !linux

package vscode

// lookUpProcess returns false: Bench can't find out here when a process
// started, so StopRecorded never signals one.
func lookUpProcess(int) (process, bool) { return process{}, false }
