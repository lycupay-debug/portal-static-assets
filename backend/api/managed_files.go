package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func validateManagedFilename(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/"+string(rune(92))) {
		return errors.New("managed filename must be a single path component")
	}
	return nil
}

// openManagedFile scopes the open operation to the configured directory. os.Root
// prevents symlink traversal from escaping that directory.
func openManagedFile(dir, name string) (*os.File, error) {
	if err := validateManagedFilename(name); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(name)
}

func readManagedFile(dir, name string) ([]byte, error) {
	f, err := openManagedFile(dir, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func writeManagedFile(dir, name string, data []byte, mode fs.FileMode) error {
	if err := validateManagedFilename(name); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	written, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		_ = root.Remove(name)
		return writeErr
	}
	if closeErr != nil {
		_ = root.Remove(name)
		return closeErr
	}
	return nil
}

func removeManagedFile(dir, name string) error {
	if err := validateManagedFilename(name); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(name)
}
