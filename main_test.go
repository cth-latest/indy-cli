package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestFilterBuild(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var f txFilter
	f.register(fs)
	pos := parse(fs, []string{"--from", "2026-01-01", "x", "--vat-type", "fr,eu", "--vat-rate", "2000,550", "--min-cents", "0", "--direction", "expense", "--has-receipts", "no"})
	if len(pos) != 1 || pos[0] != "x" {
		t.Fatalf("positionals: %v", pos)
	}
	m, err := f.build()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(m)
	want := `{"amountInCentsRange":{"above":0,"displayIncomeExpense":"expense"},"hasReceipts":"no","period":{"startDate":"2026-01-01"},"vatRate":[2000,550],"vatType":["fr","autoliquidation-fr","eu"]}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if empty, _ := (&txFilter{minCents: -1, maxCents: -1}).build(); len(empty) != 0 {
		t.Fatalf("empty filter not empty: %v", empty)
	}
	if _, err := (&txFilter{vatRates: "20%", minCents: -1, maxCents: -1}).build(); err == nil {
		t.Fatal("expected error for non-integer rate")
	}
}

func TestNewVat(t *testing.T) {
	if v, _ := newVat("toBeDefined", -1); v["rate"] != 0 {
		t.Fatal(v)
	}
	if v, _ := newVat("eu", 2000); v["type"] != "eu" || v["rate"] != 2000 {
		t.Fatal(v)
	}
	if _, err := newVat("fr", -1); err == nil {
		t.Fatal("missing rate should fail")
	}
	if _, err := newVat("bogus", 0); err == nil {
		t.Fatal("unknown type should fail")
	}
}

func TestAbsorb(t *testing.T) {
	s := &Session{SID: "old", DeviceID: "dev"}
	if s.absorb([]*http.Cookie{{Name: "other", Value: "x"}, {Name: "__Http-sid", Value: "old"}, {Name: "__Http-sid", Value: "", MaxAge: -1}}) || s.SID != "old" {
		t.Fatal("unchanged or deleted cookie must not change the session")
	}
	if !s.absorb([]*http.Cookie{{Name: "__Http-sid", Value: "new"}}) || s.SID != "new" || s.DeviceID != "dev" {
		t.Fatalf("rotated cookie not taken: %+v", s)
	}
}

func TestFileType(t *testing.T) {
	dir := t.TempDir()
	noExt := filepath.Join(dir, "scan")
	os.WriteFile(noExt, []byte("%PDF-1.7\n"), 0o600)
	for file, want := range map[string]string{"a.PDF": "application/pdf", "b.png": "image/png", noExt: "application/pdf"} {
		if got := fileType(file); got != want {
			t.Errorf("fileType(%q) = %q, want %q", file, got, want)
		}
	}
}
