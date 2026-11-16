package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// Indy's password login requires a Cloudflare Turnstile token, so the CLI logs in through a browser or a pasted cookie.
func cmdLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	cookie := fs.String("cookie", "", "skip the browser: paste the __Http-sid cookie value from app.indy.fr")
	timeout := fs.Duration("timeout", 5*time.Minute, "how long to wait for the browser login")
	parse(fs, args)

	s := &Session{SID: *cookie}
	if s.SID == "" {
		if err := browserLogin(s, *timeout); err != nil {
			return err
		}
	}
	c := newClient(s)
	me, err := c.JSON("GET", "/organisations/me", nil, nil)
	if err != nil {
		return fmt.Errorf("session check failed: %w", err)
	}
	s.OrgID = str(me, "organisation", "firebase", "uid")
	s.Email = str(me, "organisation", "signup_funnel", "email")
	if err := s.save(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Logged in as %s (session saved to %s)\n", s.Email, sessionPath())
	return nil
}

func cmdLogout(args []string) error {
	if s, err := loadSession(); err == nil {
		newClient(s).JSON("POST", "/auth/sign-out", nil, nil)
	}
	if err := os.Remove(sessionPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Fprintln(os.Stderr, "Logged out")
	return nil
}

func cmdWhoami(c *Client, args []string) (any, error) {
	me, err := c.JSON("GET", "/organisations/me", nil, nil)
	if err != nil {
		return nil, err
	}
	sf := func(k string) string { return str(me, "organisation", "signup_funnel", k) }
	return map[string]any{
		"email":        sf("email"),
		"company":      sf("company_name"),
		"siret":        sf("siret"),
		"fiscalRegime": sf("fiscal_regime"),
		"orgId":        str(me, "organisation", "firebase", "uid"),
		"fiscalPeriod": dig(me, "organisation", "current_fiscal_period"),
	}, nil
}

func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any, keys ...string) string {
	s, _ := dig(v, keys...).(string)
	return s
}
