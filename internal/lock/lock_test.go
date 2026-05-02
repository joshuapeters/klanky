package lock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestAcquireLock_FreshCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		PID       int    `json:"pid"`
		StartedAt string `json:"started_at"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatal(err)
	}
	if content.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", content.PID, os.Getpid())
	}
	if content.StartedAt == "" {
		t.Error("StartedAt empty")
	}
}

func TestAcquireLock_RefusesWhenAlive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	// Write a lock file claiming the current process owns it (definitely alive).
	content := []byte(`{"pid": ` + strconv.Itoa(os.Getpid()) + `, "started_at": "2026-04-26T10:00:00Z"}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := AcquireLock(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "in progress") {
		t.Errorf("error should mention 'in progress': %v", err)
	}
}

func TestAcquireLock_TakesOverDeadPID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	// PID 1 is init/launchd — definitely not us, but definitely alive on a real
	// system. We need a dead PID. Find one by scanning a high range; or trust
	// that PID 999999 is unlikely to exist.
	dead := findDeadPID(t)
	content := []byte(`{"pid": ` + strconv.Itoa(dead) + `, "started_at": "2026-04-26T10:00:00Z"}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("expected silent takeover, got error: %v", err)
	}
	defer lock.Release()

	// Verify the lock was overwritten with our PID.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), strconv.Itoa(os.Getpid())) {
		t.Errorf("lock file should contain our PID; got: %s", data)
	}
}

func TestAcquireLock_TakesOverCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("expected takeover on corrupt file, got: %v", err)
	}
	defer lock.Release()
}

func TestRelease_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner-7.lock")

	lock, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("lock file should be removed; got err=%v", err)
	}
}

// findDeadPID returns a PID that is not currently alive on the system.
// Scans backward from a high number looking for one where kill(pid, 0) fails.
func findDeadPID(t *testing.T) int {
	t.Helper()
	for pid := 99999; pid > 1000; pid-- {
		if err := syscall.Kill(pid, 0); err != nil {
			return pid
		}
	}
	t.Fatal("could not find a dead PID for test")
	return 0
}
