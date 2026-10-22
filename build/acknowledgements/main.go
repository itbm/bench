// Command acknowledgements writes the page that Bench's Acknowledgements
// window shows: the open source software that Bench.app includes, each with
// its copyright notices and licence.
//
// Run it from the repository root, after changing go.mod, the Go toolchain,
// or frontend/vendor:
//
//	make acknowledgements
//
// which runs
//
//	go run ./build/acknowledgements -o frontend/acknowledgements.html
//
// It asks go list which packages the macOS app's build links, with the
// architectures and tags that the Makefile builds Bench.app with, and takes
// each component's texts from the module cache, the Go toolchain, the
// vendored files, or texts/. Every third-party module that the build links
// must be in components, so a new dependency stops it until someone has
// checked the dependency's licence and added it. Its output is the same on
// every run. Its tests fail when the committed page is out of date.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	out := flag.String("o", filepath.Join("frontend", "acknowledgements.html"), "the page to write")
	flag.Parse()
	if err := run(".", *out); err != nil {
		fmt.Fprintln(os.Stderr, "acknowledgements:", err)
		os.Exit(1)
	}
}

// run writes the page for the repository at root to out, unless it's
// already up to date.
func run(root, out string) error {
	page, err := generate(root)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(out); err == nil && bytes.Equal(old, page) {
		fmt.Println(out, "is up to date")
		return nil
	}
	if err := os.WriteFile(out, page, 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

// generate returns the page for the repository at root.
func generate(root string) ([]byte, error) {
	src, err := load(root)
	if err != nil {
		return nil, err
	}
	entries, err := build(src)
	if err != nil {
		return nil, err
	}
	return render(entries)
}
