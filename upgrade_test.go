package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestChecksumOK(t *testing.T) {
	// sha256("hello")
	sums := []byte("deadbeef  indy-linux-amd64\n2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824  indy-darwin-arm64\n")
	if !checksumOK(sums, "indy-darwin-arm64", []byte("hello")) {
		t.Fatal("valid checksum rejected")
	}
	if checksumOK(sums, "indy-linux-amd64", []byte("hello")) || checksumOK(sums, "indy-darwin-arm64", []byte("tampered")) {
		t.Fatal("bad checksum accepted")
	}
}

func TestUpdateNotice(t *testing.T) {
	t.Setenv("INDY_SESSION_FILE", filepath.Join(t.TempDir(), "session.json"))
	t.Setenv("INDY_NO_UPDATE_CHECK", "")
	version = "v1.0.0"
	defer func() { version = "dev" }()
	calls := 0
	fetch := func() (string, error) { calls++; return "v1.1.0", nil }
	if n := updateNotice(fetch); n == "" || calls != 1 {
		t.Fatalf("first run: notice=%q calls=%d", n, calls)
	}
	// Within 15 minutes the cache answers, even if GitHub is unreachable.
	if n := updateNotice(func() (string, error) { calls++; return "", errors.New("offline") }); n == "" || calls != 1 {
		t.Fatalf("cached run: notice=%q calls=%d", n, calls)
	}
	version = "v1.1.0"
	if n := updateNotice(fetch); n != "" {
		t.Fatalf("up to date but got %q", n)
	}
}
