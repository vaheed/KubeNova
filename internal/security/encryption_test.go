package security

import (
	"crypto/rand"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	aad := []byte("tenant:alice")
	ct, err := Encrypt(key, []byte("secret-data"), aad)
	if err != nil {
		t.Fatalf("encrypt error: %v", err)
	}
	pt, err := Decrypt(key, ct, aad)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if string(pt) != "secret-data" {
		t.Fatalf("roundtrip mismatch: %s", string(pt))
	}
}
