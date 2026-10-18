package darwin

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestE2EInstallRetries checks that a failed Chromium download leaves
// nothing that make takes for a finished install, so that the next
// `make e2e` tries again. It runs the Makefile's rule with stand-ins for
// npm and npx.
func TestE2EInstallRetries(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make isn't installed")
	}
	makefile, err := filepath.Abs(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	// The target that `make e2e` installs Playwright with.
	m := regexp.MustCompile(`(?m)^e2e: (\S+)`).FindStringSubmatch(readText(t, makefile))
	if m == nil {
		t.Fatal("found no e2e rule in the Makefile")
	}
	target := m[1]
	dir := t.TempDir()
	tools := filepath.Join(dir, "tools")
	work := filepath.Join(dir, "work")
	calls := filepath.Join(dir, "npx-calls")
	for path, text := range map[string]string{
		// npm ci makes node_modules afresh, as the real one does.
		filepath.Join(tools, "npm"): "#!/bin/sh\nrm -rf node_modules && mkdir node_modules\n",
		// npx records each call, then fails or succeeds as the test says.
		filepath.Join(tools, "npx"):                     "#!/bin/sh\necho \"$*\" >>\"$NPX_CALLS\"\nexit \"$NPX_STATUS\"\n",
		filepath.Join(work, "e2e", "package.json"):      "{}\n",
		filepath.Join(work, "e2e", "package-lock.json"): "{}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// When `make test` runs this test, make's own variables are in the
	// environment, and the make below would take its flags from them: with
	// -i, say, it would ignore the failed download.
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "MAKEFLAGS", "MFLAGS", "GNUMAKEFLAGS", "MAKELEVEL", "MAKEFILES":
		default:
			env = append(env, kv)
		}
	}

	// install runs the rule, and returns how many times npx has run so far.
	install := func(npxStatus string, wantOK bool) int {
		t.Helper()
		cmd := exec.Command("make", "--no-print-directory", "-f", makefile, "VERSION=", target)
		cmd.Dir = work
		cmd.Env = append(env,
			"PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"),
			"NPX_CALLS="+calls,
			"NPX_STATUS="+npxStatus,
		)
		out, err := cmd.CombinedOutput()
		if ok := err == nil; ok != wantOK {
			t.Fatalf("make succeeded = %v, want %v:\n%s", ok, wantOK, out)
		}
		data, _ := os.ReadFile(calls)
		return strings.Count(string(data), "playwright install chromium")
	}

	if n := install("1", false); n != 1 {
		t.Fatalf("after a failed download, npx ran %d times, want 1", n)
	}
	if n := install("0", true); n != 2 {
		t.Fatalf("after the failed download, the next run ran npx %d times in all, want 2: make took the failed install for a finished one", n)
	}
	if n := install("0", true); n != 2 {
		t.Errorf("after a finished install, the next run ran npx %d times in all, want 2: the install should be up to date", n)
	}
}
