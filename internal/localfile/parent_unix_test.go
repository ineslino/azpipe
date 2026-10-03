//go:build !windows

package localfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDoesNotChangeCallerDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(filepath.Join(dir, "journal.json"), []byte("{}")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatal("caller directory permissions changed")
	}
}
