//go:build !unix

package main

// lockDir does nothing where there's no flock(2). Only the macOS commands,
// which these builds don't run, act on parked.json.
func lockDir(string, func()) (unlock func(), err error) { return func() {}, nil }
