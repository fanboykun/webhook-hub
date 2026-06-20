package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestParseKeyAcceptsHexAndBase64(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	hexKey := hex.EncodeToString(key)
	parsedHex, err := ParseKey(hexKey)
	if err != nil {
		t.Fatalf("ParseKey(hex) error = %v", err)
	}
	if string(parsedHex) != string(key) {
		t.Fatalf("ParseKey(hex) mismatch")
	}

	base64Key := base64.StdEncoding.EncodeToString(key)
	parsedB64, err := ParseKey(base64Key)
	if err != nil {
		t.Fatalf("ParseKey(base64) error = %v", err)
	}
	if string(parsedB64) != string(key) {
		t.Fatalf("ParseKey(base64) mismatch")
	}
}

func TestParseKeyRejectsInvalidLength(t *testing.T) {
	if _, err := ParseKey("abcd"); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	c, err := New(key)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	ciphertext, err := c.Encrypt([]byte("hello"))
	if err != nil {
		t.Fatalf("Encrypt error = %v", err)
	}
	if string(ciphertext) == "hello" {
		t.Fatal("ciphertext should not equal plaintext")
	}

	plaintext, err := c.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt error = %v", err)
	}
	if string(plaintext) != "hello" {
		t.Fatalf("plaintext = %q", plaintext)
	}
}
