package lock

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
)

// Lock represents a held klanky runner lock. Call Release on graceful shutdown
// (typically via defer at the top of a run).
type Lock struct {
	path string
}

type lockContent struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

// AcquireLock attempts to create the lock file at path. On success it returns
// a Lock; the caller must call Release. If the file already exists:
//   - alive PID  → returns an error refusing to start
//   - dead PID   → silent takeover (overwrites with our PID), returns Lock
//   - corrupt    → silent takeover (treat as dead), returns Lock
func AcquireLock(path string) (*Lock, error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			content := lockContent{
				PID:       os.Getpid(),
				StartedAt: time.Now().UTC().Format(time.RFC3339),
			}
			data, _ := json.Marshal(content)
			if _, werr := f.Write(data); werr != nil {
				f.Close()
				os.Remove(path)
				return nil, fmt.Errorf("write lock %s: %w", path, werr)
			}
			f.Close()
			return &Lock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create lock %s: %w", path, err)
		}

		// File exists. Inspect it.
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("read existing lock %s: %w", path, rerr)
		}
		var existing lockContent
		if jerr := json.Unmarshal(data, &existing); jerr != nil {
			// Corrupt — take over.
			if rmerr := os.Remove(path); rmerr != nil {
				return nil, fmt.Errorf("remove corrupt lock %s: %w", path, rmerr)
			}
			fmt.Fprintf(os.Stderr, "klanky: stale lock at %s (corrupt); recovering.\n", path)
			continue
		}

		if existing.PID > 0 && pidAlive(existing.PID) {
			return nil, fmt.Errorf(
				"another klanky run is in progress for this feature (pid %d, started %s). Exit it first, or wait.",
				existing.PID, existing.StartedAt)
		}

		// Dead PID → take over.
		if rmerr := os.Remove(path); rmerr != nil {
			return nil, fmt.Errorf("remove stale lock %s: %w", path, rmerr)
		}
		fmt.Fprintf(os.Stderr, "klanky: stale lock from pid %d (started %s); recovering.\n", existing.PID, existing.StartedAt)
	}
	return nil, fmt.Errorf("could not acquire lock at %s after retry", path)
}

// Release deletes the lock file. Safe to call multiple times.
func (l *Lock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	l.path = ""
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// pidAlive returns true if the process with the given PID is currently alive.
// Implemented via signal-0, which delivers no signal but performs error checking.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but we don't own it — still alive.
	if err == syscall.EPERM {
		return true
	}
	return false
}
