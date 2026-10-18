//go:build darwin && cgo && !server

package mac

/*
#include <stdlib.h>

// mainloop_darwin.m
int benchRunMainLoop(void);
void benchStopMainLoop(void);
char *benchTemporaryDirectory(void);
*/
import "C"

import (
	"os"
	"unsafe"
)

// RunMainLoop runs the main thread's run loop until stop is closed. The app
// has NSApplication's run loop, but `bench arrange`, which has no window,
// needs this: macOS updates NSWorkspace's list of running apps, posts its
// notifications about apps, and delivers Accessibility notifications, only
// while the main thread runs its run loop. Work sent to the main queue
// before stop is closed still runs.
//
// Call it from the goroutine that runs main, which Wails locks to the main
// thread. Anywhere else, it only waits for stop.
func RunMainLoop(stop <-chan struct{}) {
	go func() {
		<-stop
		C.benchStopMainLoop()
	}()
	if C.benchRunMainLoop() == 0 {
		<-stop
	}
}

// TemporaryDirectory returns NSTemporaryDirectory(), which ends with a
// slash: where Wails keeps Bench's single-instance lock.
func TemporaryDirectory() string {
	p := C.benchTemporaryDirectory()
	if p == nil {
		return os.TempDir() + "/"
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}
