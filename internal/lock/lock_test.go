package lock

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	l := New("/test/repo/" + t.Name())
	defer l.Release()

	if err := l.Acquire(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	// Verify lock dir exists
	if _, err := os.Stat(l.dir); os.IsNotExist(err) {
		t.Fatal("lock dir should exist after acquire")
	}

	l.Release()

	// Verify lock dir removed
	if _, err := os.Stat(l.dir); !os.IsNotExist(err) {
		t.Fatal("lock dir should not exist after release")
	}
}

func TestContention(t *testing.T) {
	l1 := New("/test/contention/" + t.Name())
	l2 := New("/test/contention/" + t.Name())

	if err := l1.Acquire(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	// l2 should block until l1 releases
	var wg sync.WaitGroup
	wg.Add(1)
	acquired := make(chan struct{})
	go func() {
		defer wg.Done()
		if err := l2.Acquire(5 * time.Second); err != nil {
			t.Errorf("l2 acquire failed: %v", err)
			return
		}
		close(acquired)
	}()

	// Give goroutine time to block
	time.Sleep(100 * time.Millisecond)

	select {
	case <-acquired:
		t.Fatal("l2 should not have acquired yet")
	default:
		// Good — still blocking
	}

	l1.Release()
	wg.Wait()

	select {
	case <-acquired:
		// Good — acquired after release
	default:
		t.Fatal("l2 should have acquired after l1 released")
	}

	l2.Release()
}

func TestStaleLock(t *testing.T) {
	l := New("/test/stale/" + t.Name())
	defer l.Release()

	// Manually create a stale lock with a dead PID
	os.MkdirAll(l.dir, 0755)
	os.WriteFile(filepath.Join(l.dir, "pid"), []byte(strconv.Itoa(999999)), 0644)

	// Should break the stale lock and acquire
	if err := l.Acquire(2 * time.Second); err != nil {
		t.Fatal(err)
	}
}
