package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// browserLogin runs the login in a plain Chrome window: no automation flags and no DevTools client while the user logs
// in, so Turnstile sees an ordinary browser. The HttpOnly session cookie is read over DevTools only once the user says so.
func browserLogin(s *Session, timeout time.Duration) error {
	// A dedicated, persistent profile: the user's main Chrome stays untouched, and Indy's "trusted device" sticks between logins.
	profile := filepath.Join(filepath.Dir(sessionPath()), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	cmd, ws, exited, err := launchChrome(profile, baseURL+"/")
	if err != nil {
		return err
	}
	defer cmd.Process.Signal(syscall.SIGTERM)

	fmt.Fprintln(os.Stderr, "Log in to Indy in the Chrome window that just opened. When you see your Indy dashboard, come back here and press Enter.")
	enter := make(chan struct{})
	go func() {
		bufio.NewReader(os.Stdin).ReadString('\n')
		close(enter)
	}()
	select {
	case <-enter:
	case <-exited:
		return errors.New("Chrome was closed before the login was confirmed: run `indy login` again and press Enter here while the window is still open")
	case <-time.After(timeout):
		return errors.New("timed out waiting for the login to finish")
	}

	cookies, err := browserCookies(ws)
	if err != nil {
		return fmt.Errorf("could not read the browser cookies: %w", err)
	}
	for _, c := range cookies {
		if !strings.HasSuffix(c.Domain, "indy.fr") {
			continue
		}
		switch c.Name {
		case "__Http-sid":
			s.SID = c.Value
		case "device_id":
			s.DeviceID = c.Value
		}
	}
	if s.SID == "" {
		return errors.New("no Indy session found in the browser: finish the login (until the dashboard shows) before pressing Enter")
	}
	return nil
}

// launchChrome starts Chrome on the profile with a DevTools port and returns the browser websocket URL.
// Nothing connects to that port here.
func launchChrome(profile, url string) (*exec.Cmd, string, <-chan error, error) {
	chrome, err := chromePath()
	if err != nil {
		return nil, "", nil, err
	}
	portFile := filepath.Join(profile, "DevToolsActivePort")
	os.Remove(portFile) // stale file from a previous run
	// Chrome sets navigator.webdriver as soon as a DevTools port is open, which Turnstile rejects; the blink flag turns that off.
	cmd := exec.Command(chrome, "--user-data-dir="+profile, "--remote-debugging-port=0", "--disable-blink-features=AutomationControlled", "--no-first-run", "--no-default-browser-check", url)
	if err := cmd.Start(); err != nil {
		return nil, "", nil, fmt.Errorf("could not start Chrome: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		// The file holds the port on line 1 and the browser websocket path on line 2.
		if b, err := os.ReadFile(portFile); err == nil {
			if l := strings.Split(strings.TrimSpace(string(b)), "\n"); len(l) == 2 {
				return cmd, "ws://127.0.0.1:" + l[0] + l[1], exited, nil
			}
		}
		select {
		case <-exited:
			// Chrome hands the URL to an already-running instance of the same profile and exits.
			return nil, "", nil, errors.New("Chrome exited at once: quit the Chrome window left over from a previous `indy login`, then retry")
		default:
		}
	}
	cmd.Process.Signal(syscall.SIGTERM)
	return nil, "", nil, errors.New("Chrome did not open its DevTools port")
}

// browserCookies reads every cookie at browser level, without attaching to any tab.
func browserCookies(ws string) ([]*network.Cookie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := remote.DialContext(ctx, ws)
	if err != nil {
		return nil, err
	}
	b, err := chromedp.NewBrowserTransport(ctx, conn)
	if err != nil {
		return nil, err
	}
	res, err := cdp.Call(ctx, b, storage.GetCookies, storage.GetCookiesParams{})
	return res.Cookies, err
}

func chromePath() (string, error) {
	for _, p := range []string{os.Getenv("INDY_CHROME"), "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium"} {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("Chrome not found: install it, set INDY_CHROME to its binary, or use `indy login --cookie <value>`")
}
