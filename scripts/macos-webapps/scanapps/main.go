// Command scanapps prints, as JSON, the installed apps that Bench finds, with
// the same code the app uses for its add dialog. scripts/macos-webapps/run.sh
// runs it on a GitHub macOS runner to check that Bench finds the web apps
// that browsers make.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/itbm/bench/internal/apps"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "scanapps:", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(apps.Scan(apps.Dirs(home))); err != nil {
		fmt.Fprintln(os.Stderr, "scanapps:", err)
		os.Exit(1)
	}
}
