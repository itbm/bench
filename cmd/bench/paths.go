package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/itbm/bench/internal/config"
)

// previewFolder is the browser preview's support folder, inside Bench's,
// so that the preview's demo never touches the app's config.toml or
// state.json.
const previewFolder = "Preview"

// configFlag adds the --config flag.
func configFlag(fs *flag.FlagSet) *string {
	return fs.String("config", "", "the config file to read (default: config.toml in Bench's support folder)")
}

// dirFlag adds the --dir flag.
func dirFlag(fs *flag.FlagSet, usage string) *string {
	return fs.String("dir", "", usage)
}

// configPath returns the config file to read: flagValue, or config.toml in
// the support folder.
func configPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, config.ConfigFile), nil
}

// supportDir returns flagValue, or Bench's support folder. A folder given
// with --dir is checked as $BENCH_HOME is, since it may be somewhere shared.
func supportDir(flagValue string) (string, error) {
	if flagValue != "" {
		if err := config.CheckDir(flagValue); err != nil {
			return "", err
		}
		return flagValue, nil
	}
	return config.Dir()
}

// previewDir returns the browser preview's support folder: flagValue, or
// $BENCH_HOME if it's set, or the Preview folder in Bench's support folder.
func previewDir(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if home := os.Getenv("BENCH_HOME"); home != "" {
		return home, nil
	}
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, previewFolder), nil
}
