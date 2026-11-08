//go:build darwin || server

package ui

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/itbm/bench/internal/config"
)

// ForwardKeyFile is the file in the support folder that holds the key with
// which a second Bench, and the bench command's script subcommands (Forward),
// encrypt what they send the running Bench through Wails' single-instance
// lock (SingleInstanceOptions.EncryptionKey). Wails sends it as a distributed
// notification, which any app of the user's can post, and read, even one
// that macOS keeps in a sandbox. With the key, the running Bench drops what
// isn't encrypted with it, and only a program that can read the support
// folder, which a sandboxed app can't, can send it anything
// (docs/architecture.md, decision 77).
const ForwardKeyFile = "forward.key"

// forwardKeySize is the key's size: AES-256's, as Wails uses.
const forwardKeySize = 32

// errBadKey is the error for a key file of the wrong size.
var errBadKey = errors.New("it isn't a key")

// forwardKey returns the key in the support folder dir, making it, readable
// only by the user, if there's none, or if what's there isn't a key. Two
// Benches that start at once agree on one key: each makes its own under
// another name, and only the first to give one the key's name keeps it.
func forwardKey(dir string) ([forwardKeySize]byte, error) {
	var key [forwardKeySize]byte
	if err := config.CheckDir(dir); err != nil {
		return key, err
	}
	path := filepath.Join(dir, ForwardKeyFile)
	key, err := readKey(path)
	bad := errors.Is(err, errBadKey)
	if err == nil || (!bad && !errors.Is(err, fs.ErrNotExist)) {
		return key, err
	}
	if _, err := rand.Read(key[:]); err != nil {
		return key, err
	}
	tmp, err := os.CreateTemp(dir, ForwardKeyFile+".*") // Mode 0600.
	if err != nil {
		return key, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(key[:]); err != nil {
		tmp.Close()
		return key, err
	}
	if err := tmp.Close(); err != nil {
		return key, err
	}
	if bad {
		// A file of the wrong size is no Bench's key, so replacing it
		// takes nothing from a Bench that's running.
		return key, os.Rename(tmp.Name(), path)
	}
	if err := os.Link(tmp.Name(), path); errors.Is(err, fs.ErrExist) {
		return readKey(path)
	} else if err != nil {
		return key, err
	}
	return key, nil
}

// readKey reads the key in the file at path.
func readKey(path string) ([forwardKeySize]byte, error) {
	var key [forwardKeySize]byte
	data, err := os.ReadFile(path)
	if err != nil {
		return key, err
	}
	if len(data) != forwardKeySize {
		return key, fmt.Errorf("reading %s: %w", path, errBadKey)
	}
	copy(key[:], data)
	return key, nil
}

// someKey returns a key that no other process knows: the running Bench's,
// when it can't make or read its key file. Nothing can then send it
// anything, which is safer than taking what anything sends.
func someKey() [forwardKeySize]byte {
	var key [forwardKeySize]byte
	_, _ = rand.Read(key[:])
	return key
}
