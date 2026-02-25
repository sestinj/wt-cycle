package lock

import (
	"crypto/md5"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultTimeout = 30 * time.Second
	pollInterval   = 500 * time.Millisecond
)

// Lock represents a filesystem-based lock using mkdir atomicity.
type Lock struct {
	dir string
}

// New creates a lock for the given repo root.
func New(repoRoot string) *Lock {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(repoRoot)))
	return &Lock{dir: fmt.Sprintf("/tmp/wt-cycle-lock-%s", hash)}
}

// Acquire attempts to acquire the lock, blocking up to timeout.
func (l *Lock) Acquire(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := os.Mkdir(l.dir, 0755); err == nil {
			// Write our PID
			_ = os.WriteFile(l.dir+"/pid", []byte(strconv.Itoa(os.Getpid())), 0644)
			return nil
		}

		// Check for stale lock
		if l.isStale() {
			os.RemoveAll(l.dir)
			continue
		}

		if time.Now().After(deadline) {
			// Timeout — break the lock
			os.RemoveAll(l.dir)
			continue
		}

		time.Sleep(pollInterval)
	}
}

// Release releases the lock.
func (l *Lock) Release() {
	os.RemoveAll(l.dir)
}

// isStale checks if the lock holder PID is still alive.
func (l *Lock) isStale() bool {
	data, err := os.ReadFile(l.dir + "/pid")
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return true // corrupt PID file
	}
	// Check if process exists
	proc, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	// On Unix, signal 0 checks existence without sending a signal
	err = proc.Signal(syscall.Signal(0))
	return err != nil
}
