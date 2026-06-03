package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v.json")
	if err := WriteAtomicJSON(path, map[string]int{"v": 1}); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomicJSON(path, map[string]int{"v": 2}); err != nil {
		t.Fatal(err)
	}
	var out map[string]int
	if err := ReadJSON(path, &out); err != nil {
		t.Fatal(err)
	}
	if out["v"] != 2 {
		t.Fatalf("overwrite failed: %v", out)
	}
}

func TestReadJSON_Missing(t *testing.T) {
	if err := ReadJSON(filepath.Join(t.TempDir(), "nope.json"), &map[string]any{}); err == nil {
		t.Fatal("expected error reading missing file")
	}
}

func TestReadJSON_BadJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(p, []byte("{not json"), 0o644)
	if err := ReadJSON(p, &map[string]any{}); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestRelease_NilSafe(t *testing.T) {
	var l *FileLock
	if err := l.Release(); err != nil {
		t.Fatalf("nil Release should be safe: %v", err)
	}
}

func TestWriteAtomic_ParentIsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// MkdirAll under a regular file must fail.
	if err := WriteAtomicJSON(filepath.Join(file, "child.json"), map[string]int{"a": 1}); err == nil {
		t.Fatal("expected error when parent path is a file")
	}
}

func TestAcquireLock_BadParent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(filepath.Join(file, "x.lock")); err == nil {
		t.Fatal("expected error opening lock under a file parent")
	}
	if _, _, err := TryLock(filepath.Join(file, "y.lock")); err == nil {
		t.Fatal("expected error from TryLock under a file parent")
	}
}
