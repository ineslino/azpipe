package localfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteAtomicReplacesAndProtects(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Protect(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	for _, data := range []string{"first", "replacement"} {
		if err := WriteAtomic(path, []byte(data)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != data {
			t.Fatalf("read=%q, error=%v", got, err)
		}
		for _, protected := range []string{dir, path} {
			if err := Check(protected); err != nil {
				t.Fatalf("%s: %v", filepath.Base(protected), err)
			}
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 1 {
			t.Fatalf("temporary files remain: %v, %v", files, err)
		}
	}
}

func TestWriteAtomicRejectsDirectoryAndKeepsContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keep")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(path, "original")
	if err := os.WriteFile(child, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("replacement")); err == nil {
		t.Fatal("directory accepted as destination")
	}
	got, err := os.ReadFile(child)
	if err != nil || string(got) != "original" {
		t.Fatalf("original changed: %q, %v", got, err)
	}
}

func TestPrivateFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "original")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Windows host does not permit symlink creation")
		}
		t.Fatal(err)
	}
	if err := Protect(link); err == nil {
		t.Fatal("Protect accepted a symlink")
	}
	if err := WriteAtomic(link, []byte("replacement")); err == nil {
		t.Fatal("WriteAtomic accepted a symlink")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Fatalf("symlink target changed: %q, %v", got, err)
	}
}
