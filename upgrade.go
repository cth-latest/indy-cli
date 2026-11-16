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
	"time"
)

// Set by the release build: -ldflags "-X main.version=v1.2.3".
var version = "dev"

const repo = "cth-latest/indy-cli"

func cmdUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	check := fs.Bool("check", false, "only report whether a newer version exists")
	parse(fs, args)

	tag, err := latestTag(30 * time.Second)
	if err != nil {
		return fmt.Errorf("could not check the latest release: %w", err)
	}
	rel := struct{ Tag string }{tag}
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
	bin, err := httpGet(base+name, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("could not download %s: %w", name, err)
	}
	sums, err := httpGet(base+"checksums.txt", 30*time.Second)
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

func latestTag(timeout time.Duration) (string, error) {
	b, err := httpGet("https://api.github.com/repos/"+repo+"/releases/latest", timeout)
	if err != nil {
		return "", err
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(b, &rel); err != nil || rel.Tag == "" {
		return "", fmt.Errorf("unexpected answer from GitHub releases: %.200s", b)
	}
	return rel.Tag, nil
}

// updateNotice returns a one-line hint when a newer release exists. The latest tag is cached in a file whose
// modification time is the last check, so GitHub is asked at most once every 15 minutes.
func updateNotice(fetch func() (string, error)) string {
	if version == "dev" || os.Getenv("INDY_NO_UPDATE_CHECK") != "" {
		return ""
	}
	p := filepath.Join(filepath.Dir(sessionPath()), "latest-version")
	cached, _ := os.ReadFile(p)
	tag := strings.TrimSpace(string(cached))
	if st, err := os.Stat(p); err != nil || time.Since(st.ModTime()) > 15*time.Minute {
		if t, err := fetch(); err == nil {
			tag = t
		}
		// Written even when the check failed, so being offline does not slow down every command.
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(tag), 0o600)
	}
	if tag == "" || tag == version {
		return ""
	}
	return fmt.Sprintf("indy %s is available (you have %s): run `indy upgrade`", tag, version)
}

func httpGet(u string, timeout time.Duration) ([]byte, error) {
	resp, err := (&http.Client{Timeout: timeout}).Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, u)
	}
	return io.ReadAll(resp.Body)
}
