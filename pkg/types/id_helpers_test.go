package types

import "testing"

func TestParseID(t *testing.T) {
	if _, err := ParseID(""); err == nil {
		t.Fatalf("expected error for empty UUID")
	}
	id := NewID()
	s := id.String()
	got, err := ParseID(s)
	if err != nil {
		t.Fatalf("parse valid uuid: %v", err)
	}
	if got != id {
		t.Fatalf("round trip mismatch: %s != %s", got, id)
	}
}
