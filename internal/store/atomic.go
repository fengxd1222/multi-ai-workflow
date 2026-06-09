package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes data to path via tmp-in-same-dir + fsync + rename + dir
// fsync, so a crash never exposes a partially-written file (rev3 §4, §9).
// POSIX rename(2) within one filesystem is atomic.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename succeeds.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return FsyncDir(dir)
}

// WriteAtomicJSON marshals v (indented) and writes it atomically.
func WriteAtomicJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return WriteAtomic(path, data, 0o644)
}

// ReadJSON reads and unmarshals a JSON file into v.
func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// FsyncDir flushes a directory's metadata (the entries linking newly-created
// files into it) so a crash cannot lose a file that was just created+fsync'd.
// Directory fsync is unsupported on some platforms (e.g. Windows); such errors
// are treated as best-effort no-ops.
func FsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return nil //nolint:nilerr // dir fsync best-effort
	}
	return nil
}
