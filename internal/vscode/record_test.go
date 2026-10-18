//go:build unix

package vscode

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestStartRecordsTheServer(t *testing.T) {
	f := newFake(t, bodyServer)
	path := filepath.Join(t.TempDir(), "vscode.json")
	s := startFake(t, Options{Code: f.path, Record: path})

	rec, err := readRecord(path)
	if err != nil {
		t.Fatalf("no record after Start: %v", err)
	}
	if pid := f.pid(t, f.pidFile); rec.PID != pid {
		t.Errorf("record names process %d, want the one Start ran, %d", rec.PID, pid)
	}
	if want := f.args(t)[6]; rec.TokenFile != want || rec.TempDir != filepath.Dir(want) {
		t.Errorf("record names token %s in %s, want %s in its own folder", rec.TokenFile, rec.TempDir, want)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if rec.Started == "" {
			t.Error("record has no start time")
		}
	}
	checkMode(t, path, 0o600)

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("record still there after Stop: %v", err)
	}
}

// TestStopRecordedStopsAnOrphanedServer stands in for a Bench that crashed:
// its server is still running, so the watchdog stops it from its record.
func TestStopRecordedStopsAnOrphanedServer(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Bench can't tell here when a process started")
	}
	f := newFake(t, bodyServer)
	path := filepath.Join(t.TempDir(), "vscode.json")
	s := startFake(t, Options{Code: f.path, Record: path})
	child := f.pid(t, f.child)
	token := f.args(t)[6]
	// The Bench that started it has gone.
	rec, err := readRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Owner != os.Getpid() || rec.OwnerStarted == "" {
		t.Errorf("record names Bench as %d (started %q), want this process", rec.Owner, rec.OwnerStarted)
	}
	rec.OwnerStarted = "gone"
	writeTestRecord(t, path, rec)

	if err := StopRecorded(path); err != nil {
		t.Fatalf("StopRecorded() = %v", err)
	}
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the server is still running after StopRecorded")
	}
	if runtime.GOOS == "linux" {
		waitFor(t, "the server's child to end", func() bool { return !processAlive(child) })
	}
	for _, p := range []string{token, filepath.Dir(token), path} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there after StopRecorded: %v", p, err)
		}
	}
	if err := StopRecorded(path); err != nil {
		t.Errorf("StopRecorded() with no record = %v, want nil", err)
	}
}

// TestStopRecordedLeavesARunningBenchsServer checks that a server whose
// Bench is still running is left alone: bench arrange's watchdog, say, while
// the app runs.
func TestStopRecordedLeavesARunningBenchsServer(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Bench can't tell here when a process started")
	}
	f := newFake(t, bodyServer)
	path := filepath.Join(t.TempDir(), "vscode.json")
	s := startFake(t, Options{Code: f.path, Record: path})
	if err := StopRecorded(path); err != nil {
		t.Fatalf("StopRecorded() = %v", err)
	}
	select {
	case <-s.Done():
		t.Fatal("StopRecorded stopped the server of a Bench that's still running")
	case <-time.After(100 * time.Millisecond):
	}
	for _, p := range []string{f.args(t)[6], path} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("StopRecorded removed %s, which a running Bench needs: %v", p, err)
		}
	}
}

// writeTestRecord writes a record.
func writeTestRecord(t *testing.T, path string, rec record) {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestStopRecordedSparesOtherProcesses checks that a record whose process
// has gone, so that its ID may now be another process's, signals nothing,
// and that a damaged record removes nothing it doesn't name as Start would.
func TestStopRecordedSparesOtherProcesses(t *testing.T) {
	sleeper := exec.Command("sleep", "60")
	sleeper.SysProcAttr = ownProcessGroup()
	if err := sleeper.Start(); err != nil {
		t.Skip("can't run sleep:", err)
	}
	t.Cleanup(func() {
		signalGroup(sleeper.Process.Pid, true)
		_ = sleeper.Wait()
	})
	pid := sleeper.Process.Pid

	dir := t.TempDir()
	other := filepath.Join(dir, "notes.txt")
	otherDir := filepath.Join(dir, "projects")
	for _, err := range []error{os.WriteFile(other, nil, 0o600), os.Mkdir(otherDir, 0o700)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		rec  record
	}{
		{"another start time", record{PID: pid, Started: "1.000001", TokenFile: other, TempDir: otherDir}},
		{"no start time", record{PID: pid, TokenFile: other, TempDir: otherDir}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vscode.json")
			writeTestRecord(t, path, tt.rec)
			if err := StopRecorded(path); err != nil {
				t.Errorf("StopRecorded() = %v", err)
			}
			time.Sleep(50 * time.Millisecond)
			if err := syscall.Kill(pid, 0); err != nil {
				t.Errorf("StopRecorded signalled process %d, which isn't the server: %v", pid, err)
			}
			for _, p := range []string{other, otherDir} {
				if _, err := os.Stat(p); err != nil {
					t.Errorf("StopRecorded removed %s, which isn't a token: %v", p, err)
				}
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the record is still there: %v", err)
			}
		})
	}
}

// TestStopRecordedRemovesOnlyATokenFolder checks that a damaged record
// can't remove a folder that's only named like the one Start makes for the
// token, such as a project, or anything else in a folder that is.
func TestStopRecordedRemovesOnlyATokenFolder(t *testing.T) {
	project := filepath.Join(t.TempDir(), tempDirPrefix+"extension")
	tokens, err := os.MkdirTemp("", tempDirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tokens) })
	keep := []string{filepath.Join(project, "main.go"), filepath.Join(tokens, "notes.txt")}
	for _, err := range []error{os.Mkdir(project, 0o700), os.WriteFile(keep[0], nil, 0o600), os.WriteFile(keep[1], nil, 0o600)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		rec  record
	}{
		{"not in the temporary folder", record{TokenFile: filepath.Join(project, tokenPrefix+"1"), TempDir: project}},
		{"not the token's folder", record{TokenFile: filepath.Join(t.TempDir(), tokenPrefix+"1"), TempDir: tokens}},
		{"holding other files", record{TokenFile: filepath.Join(tokens, tokenPrefix+"1"), TempDir: tokens}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vscode.json")
			writeTestRecord(t, path, tt.rec)
			_ = StopRecorded(path)
			for _, p := range keep {
				if _, err := os.Stat(p); err != nil {
					t.Errorf("StopRecorded removed %s, which isn't a token: %v", p, err)
				}
			}
		})
	}
}

func TestStopRecordedWithADamagedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vscode.json")
	if err := os.WriteFile(path, []byte("{pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := StopRecorded(path); err == nil {
		t.Error("StopRecorded() = nil for a damaged record, want an error")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the damaged record is still there: %v", err)
	}
}

func TestProcessStarted(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Bench can't tell here when a process started")
	}
	first, ok := processStarted(os.Getpid())
	if !ok || first == "" {
		t.Fatalf("processStarted(this process) = %q, %v", first, ok)
	}
	if again, _ := processStarted(os.Getpid()); again != first {
		t.Errorf("processStarted changed from %q to %q", first, again)
	}
	sleeper := exec.Command("sleep", "60")
	if err := sleeper.Start(); err != nil {
		t.Skip("can't run sleep:", err)
	}
	pid := sleeper.Process.Pid
	if other, ok := processStarted(pid); !ok || other == first {
		t.Errorf("processStarted(%d) = %q, %v; want another start time than this process's, %q", pid, other, ok, first)
	}
	if p, _ := lookUpProcess(pid); p.ended {
		t.Errorf("lookUpProcess(%d) says a running process has ended", pid)
	}
	_ = sleeper.Process.Kill()
	// Until it's collected, it's a zombie.
	deadline := time.Now().Add(5 * time.Second)
	for p, ok := lookUpProcess(pid); !ok || !p.ended; p, ok = lookUpProcess(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("lookUpProcess(%d) = %+v, %v after it ended; want a zombie", pid, p, ok)
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = sleeper.Wait()
	if got, ok := processStarted(pid); ok {
		t.Errorf("processStarted(%d) after it ended = %q, want false", pid, got)
	}
	if _, ok := processStarted(0); ok {
		t.Error("processStarted(0) = true")
	}
}
