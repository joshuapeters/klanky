package lock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPath_Lowercases(t *testing.T) {
	got := Path("/root", "JoshuaPeters", "Klanky", "auth")
	want := "/root/locks/joshuapeters/klanky/auth.lock"
	if got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestAcquire_FreshAndRelease(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.lock")

	lk, err := Acquire(p)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var body content
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if body.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", body.PID, os.Getpid())
	}

	if err := lk.Release(); err != nil {
		t.Errorf("Release: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("lock file still present after Release")
	}
}

func TestAcquire_RefusesIfHeldByLivePID(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.lock")
	body, _ := json.Marshal(content{PID: os.Getpid(), StartedAt: "2026-05-02T00:00:00Z"})
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Acquire(p)
	if err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("expected refusal, got %v", err)
	}
}

func TestAcquire_TakesOverDeadPID(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.lock")
	body, _ := json.Marshal(content{PID: 1, StartedAt: "2026-05-02T00:00:00Z"})
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	// PID 1 is alive on every Unix system, so use a definitely-dead high PID.
	body, _ = json.Marshal(content{PID: 999999999, StartedAt: "2026-05-02T00:00:00Z"})
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	lk, err := Acquire(p)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lk.Release()
	data, _ := os.ReadFile(p)
	var got content
	_ = json.Unmarshal(data, &got)
	if got.PID != os.Getpid() {
		t.Errorf("expected lock takeover by current pid, got %d", got.PID)
	}
}

func TestAcquire_TakesOverCorruptLock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.lock")
	if err := os.WriteFile(p, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	lk, err := Acquire(p)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lk.Release()
}
