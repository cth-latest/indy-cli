package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const baseURL = "https://app.indy.fr"

// Mimic the web app; the API sits behind Cloudflare.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/157.0.0.0 Safari/537.36"

type Session struct {
	SID      string `json:"sid"`
	DeviceID string `json:"deviceId,omitempty"`
	OrgID    string `json:"orgId,omitempty"`
	Email    string `json:"email,omitempty"`
}

func sessionPath() string {
	if p := os.Getenv("INDY_SESSION_FILE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "indy", "session.json")
}

func loadSession() (*Session, error) {
	s := &Session{}
	if b, err := os.ReadFile(sessionPath()); err == nil {
		if err := json.Unmarshal(b, s); err != nil {
			return nil, fmt.Errorf("corrupt session file %s: %w", sessionPath(), err)
		}
	}
	if v := os.Getenv("INDY_SESSION"); v != "" {
		s.SID = v
	}
	if v := os.Getenv("INDY_ORG_ID"); v != "" {
		s.OrgID = v
	}
	if s.SID == "" {
		return nil, fmt.Errorf("not logged in: run `indy login` (or set INDY_SESSION)")
	}
	return s, nil
}

func (s *Session) save() error {
	p := sessionPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

// absorb takes over session cookies set by a response and reports whether anything changed.
func (s *Session) absorb(cookies []*http.Cookie) bool {
	changed := false
	for _, ck := range cookies {
		dst := map[string]*string{"__Http-sid": &s.SID, "device_id": &s.DeviceID}[ck.Name]
		// MaxAge < 0 is a deletion (sign-out), not a new value.
		if dst != nil && ck.Value != "" && ck.MaxAge >= 0 && ck.Value != *dst {
			*dst, changed = ck.Value, true
		}
	}
	return changed
}

type Client struct {
	s    *Session
	http *http.Client
}

func newClient(s *Session) *Client {
	return &Client{s: s, http: &http.Client{Timeout: 120 * time.Second}}
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body) }

// do sends a request to /api<path>. body may be nil, an io.Reader (sent as-is with ctype), or any JSON-marshalable value.
func (c *Client) do(method, path string, query url.Values, body any, ctype string) (*http.Response, error) {
	u := baseURL + "/api" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case io.Reader:
		r = b
	default:
		j, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		r, ctype = bytes.NewReader(j), "application/json"
	}
	req, err := http.NewRequest(method, u, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", userAgent)
	req.Header.Set("origin", baseURL)
	req.Header.Set("x-client-app-type", "web")
	if ctype != "" {
		req.Header.Set("content-type", ctype)
	}
	if c.s != nil {
		if c.s.OrgID != "" {
			req.Header.Set("x-active-organisation-id", c.s.OrgID)
		}
		if c.s.SID != "" {
			req.AddCookie(&http.Cookie{Name: "__Http-sid", Value: c.s.SID})
		}
		if c.s.DeviceID != "" {
			req.AddCookie(&http.Cookie{Name: "device_id", Value: c.s.DeviceID})
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	// Indy may replace the session cookie on any response; keeping the old one would log the CLI out later.
	if c.s != nil && c.s.absorb(resp.Cookies()) && os.Getenv("INDY_SESSION") == "" {
		c.s.save()
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &APIError{resp.StatusCode, strings.TrimSpace(string(b))}
	}
	return resp, nil
}

// JSON performs a request and decodes the JSON response (nil for empty bodies).
func (c *Client) JSON(method, path string, query url.Values, body any) (any, error) {
	resp, err := c.do(method, path, query, body, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b), nil
	}
	return v, nil
}

// fileType is the MIME type of a file, from its extension or else from its first bytes.
func fileType(file string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(file))); t != "" {
		return t
	}
	b, _ := os.ReadFile(file)
	return http.DetectContentType(b)
}

func (c *Client) Upload(path string, query url.Values, file string) (any, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	// Indy rejects files sent as application/octet-stream (the CreateFormFile default); the web app sends the real type.
	part, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filepath.Base(file)})},
		"Content-Type":        {fileType(file)},
	})
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	mw.Close()
	resp, err := c.do("POST", path, query, &buf, mw.FormDataContentType())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v any
	return v, json.NewDecoder(resp.Body).Decode(&v)
}

// Download writes the raw response body to out ("-" = stdout).
func (c *Client) Download(path string, query url.Values, out string) error {
	resp, err := c.do("GET", path, query, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	w := io.Writer(os.Stdout)
	if out != "-" {
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	_, err = io.Copy(w, resp.Body)
	return err
}
