// Package frontend embeds Bench's UI, so that the app and the browser preview
// serve it from the binary. The UI itself is plain HTML, CSS, and ES modules
// in this folder, with no build step (docs/architecture.md).
package frontend

import (
	"embed"
	"io/fs"
)

// files holds the UI. Directories are embedded without their dot and
// underscore files; this Go file isn't embedded.
//
//go:embed index.html acknowledgements.html styles.css js vendor
var files embed.FS

// FS is the UI, with index.html at its root.
var FS fs.FS = files
