// Package localfile protects local settings, profiles and run journals.
package localfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes a protected temporary file and replaces the destination.
// It does not change permissions on an existing parent directory.
// Rename is atomic on Unix; Go does not guarantee atomic replacement on Windows.
func WriteAtomic(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("destino privado deve ser um ficheiro regular")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".azpipe-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = Protect(f.Name()); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
