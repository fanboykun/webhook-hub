package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const (
	EncryptionKeyEnv = "GATEWAY_ENCRYPTION_KEY"
	nonceSize        = 12
)

type Cipher struct {
	key []byte
}

func New(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	cloned := make([]byte, len(key))
	copy(cloned, key)
	return &Cipher{key: cloned}, nil
}

func NewFromString(raw string) (*Cipher, error) {
	key, err := ParseKey(raw)
	if err != nil {
		return nil, err
	}
	return New(key)
}

func ParseKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("encryption key is required")
	}

	if decoded, err := hex.DecodeString(raw); err == nil {
		if len(decoded) != 32 {
			return nil, fmt.Errorf("encryption key hex value must decode to 32 bytes")
		}
		return decoded, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("encryption key must be valid hex or base64: %w", err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("encryption key base64 value must decode to 32 bytes")
	}
	return decoded, nil
}

func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	out := make([]byte, 0, len(nonce)+len(plaintext)+gcm.Overhead())
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plaintext, nil)
	return out, nil
}

func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := ciphertext[:nonceSize]
	payload := ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}
	return plaintext, nil
}
