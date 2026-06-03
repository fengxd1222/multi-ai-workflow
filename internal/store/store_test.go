package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteAtomicJSON_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "job.json")
	in := map[string]any{"job_id": "J-1", "rev": float64(3)}

	if err := WriteAtomicJSON(path, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out map[string]any
	if err := ReadJSON(path, &out); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out["job_id"] != "J-1" || out["rev"] != float64(3) {
		t.Fatalf("roundtrip mismatch: %v", out)
	}
}

func TestWriteAtomic_NoTempLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	if err := WriteAtomicJSON(path, map[string]int{"a": 1}); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "x.json" {
			t.Fatalf("unexpected leftover file: %s", e.Name())
		}
	}
}

// TestLock_Serializes verifies flock provides mutual exclusion: N goroutines
// each read-increment-write a counter under the lock; no updates are lost.
func TestLock_Serializes(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "state.lock")
	counterPath := filepath.Join(dir, "counter.json")
	if err := WriteAtomicJSON(counterPath, map[string]int{"n": 0}); err != nil {
		t.Fatal(err)
	}

	const goroutines, perG = 8, 25
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				lk, err := AcquireLock(lockPath)
				if err != nil {
					t.Errorf("lock: %v", err)
					return
				}
				var c map[string]int
				_ = ReadJSON(counterPath, &c)
				c["n"]++
				_ = WriteAtomicJSON(counterPath, c)
				lk.Release()
			}
		}()
	}
	wg.Wait()

	var final map[string]int
	if err := ReadJSON(counterPath, &final); err != nil {
		t.Fatal(err)
	}
	if want := goroutines * perG; final["n"] != want {
		t.Fatalf("lost updates: got %d want %d", final["n"], want)
	}
}

func TestTryLock_BlocksWhileHeld(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "recover.lock")

	held, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := TryLock(lockPath); err != nil || ok {
		t.Fatalf("expected TryLock to fail while held; ok=%v err=%v", ok, err)
	}
	held.Release()

	lk, ok, err := TryLock(lockPath)
	if err != nil || !ok {
		t.Fatalf("expected TryLock to succeed after release; ok=%v err=%v", ok, err)
	}
	lk.Release()
}
