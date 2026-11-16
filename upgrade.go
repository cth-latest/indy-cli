package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Set by the release build: -ldflags "-X main.version=v1.2.3".
var version = "dev"

const repo = "cth-latest/indy-cli"

func cmdUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	check := fs.Bool("check", false, "only report whether a newer version exists")
	parse(fs, args)

	b, err := httpGet("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return fmt.Errorf("could not check the latest release: %w", err)
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(b, &rel); err != nil || rel.Tag == "" {
		return fmt.Errorf("unexpected answer from GitHub releases: %.200s", b)
	}
	if rel.Tag == version {
		fmt.Fprintf(os.Stderr, "indy %s is up to date\n", version)
		return nil
	}
	fmt.Fprintf(os.Stderr, "indy %s is available (you have %s)\n", rel.Tag, version)
	if *check {
		return nil
	}

	name := "indy-" + runtime.GOOS + "-" + runtime.GOARCH
	base := "https://github.com/" + repo + "/releases/download/" + rel.Tag + "/"
	bin, err := httpGet(base + name)
	if err != nil {
		return fmt.Errorf("could not download %s: %w", name, err)
	}
	sums, err := httpGet(base + "checksums.txt")
	if err != nil {
		return fmt.Errorf("could not download the checksums: %w", err)
	}
	if !checksumOK(sums, name, bin) {
		return fmt.Errorf("checksum mismatch for %s: nothing was installed", name)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	// Write next to the binary, then rename: the swap is atomic and works while indy is running.
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf("could not write %s (try with sudo, or reinstall with the install script): %w", tmp, err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return err
	}
	fmt.Fprintf(os.Stderr, "Upgraded %s to %s\n", exe, rel.Tag)
	return nil
}

// checksumOK looks for the sha256sum line "<hash>  <name>" in sums.
func checksumOK(sums []byte, name string, bin []byte) bool {
	want := fmt.Sprintf("%x  %s", sha256.Sum256(bin), name)
	for _, l := range strings.Split(string(sums), "\n") {
		if strings.TrimSpace(l) == want {
			return true
		}
	}
	return false
}

func httpGet(u string) ([]byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, u)
	}
	return io.ReadAll(resp.Body)
}
