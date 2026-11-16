package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"
)

// Opens a real Chrome window, so it only runs on demand: INDY_BROWSER_TEST=1 go test -run BrowserCookies
func TestBrowserCookies(t *testing.T) {
	if os.Getenv("INDY_BROWSER_TEST") == "" {
		t.Skip("set INDY_BROWSER_TEST=1 to run")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "device_id", Value: "abc", Path: "/api", HttpOnly: true})
	}))
	defer srv.Close()
	cmd, ws, exited, err := launchChrome(t.TempDir(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Signal(syscall.SIGTERM); <-exited }()
	for range 20 {
		time.Sleep(500 * time.Millisecond)
		cookies, err := browserCookies(ws)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cookies {
			if c.Name == "device_id" && c.Value == "abc" {
				return
			}
		}
	}
	t.Fatal("cookie not found")
}
