package main

import "testing"

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
