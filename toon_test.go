package main

import (
	"encoding/json"
	"testing"

	toon "github.com/toon-format/toon-go"
)

// The API result is decoded JSON (map[string]any / []any): check that shape encodes to tabular TOON.
func TestToon(t *testing.T) {
	var v any
	json.Unmarshal([]byte(`{"transactions":[{"id":"a","amount":-1200},{"id":"b","amount":50}],"total":2}`), &v)
	b, err := toon.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	want := "total: 2\ntransactions[2]{amount,id}:\n  -1200,a\n  50,b"
	if string(b) != want {
		t.Fatalf("got:\n%s", b)
	}
}
