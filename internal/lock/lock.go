// Package lock implements the per-project file lock at
// `~/.klanky/locks/<owner>/<repo>/<slug>.lock`. One lock per project; a runner
// against project A does not block one against project B.
package lock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Path returns the lock file path for a (klanky-state-root, owner, repo, slug)
// tuple. stateRoot is typically `~/.klanky`. owner and repo are lowercased to
// match the directory layout.
func Path(stateRoot, owner, repo, slug string) string {
	return filepath.Join(stateRoot, "locks", lc(owner), lc(repo), slug+".lock")
}

func lc(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// Lock represents a held klanky runner lock. Call Release on graceful
// shutdown (typically via defer at the top of a run).
type Lock struct {
	path string
}

type content struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

// Acquire attempts to create the lock file. On success returns a Lock; the
// caller must call Release. If the file exists:
//   - alive PID  → returns an error refusing to start
//   - dead PID   → silent takeover (overwrites with our PID)
//   - corrupt    → silent takeover (treat as dead)
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir lock dir: %w", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			body := content{PID: os.Getpid(), StartedAt: time.Now().UTC().Format(time.RFC3339)}
			data, _ := json.Marshal(body)
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

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("read existing lock %s: %w", path, rerr)
		}
		var existing content
		if jerr := json.Unmarshal(data, &existing); jerr != nil {
			if rmerr := os.Remove(path); rmerr != nil {
				return nil, fmt.Errorf("remove corrupt lock %s: %w", path, rmerr)
			}
			fmt.Fprintf(os.Stderr, "klanky: stale lock at %s (corrupt); recovering.\n", path)
			continue
		}
		if existing.PID > 0 && pidAlive(existing.PID) {
			return nil, fmt.Errorf("another klanky run is in progress for this project (pid %d, started %s). Exit it first, or wait.",
				existing.PID, existing.StartedAt)
		}
		if rmerr := os.Remove(path); rmerr != nil {
			return nil, fmt.Errorf("remove stale lock %s: %w", path, rmerr)
		}
		fmt.Fprintf(os.Stderr, "klanky: stale lock from pid %d (started %s); recovering.\n",
			existing.PID, existing.StartedAt)
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

// pidAlive returns true if the given PID is currently alive. Implemented via
// signal-0, which performs error checking without delivering a signal.
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
