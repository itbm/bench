//go:build !unix

package config

// checkPrivate does nothing where there are no Unix owners and
// permissions to check.
func checkPrivate(string) error { return nil }
