// Package tokencrypto provides AES-256-GCM authenticated encryption for
// short-lived OAuth access tokens stored at rest in the database.
//
// Key derivation: the raw TOKEN_ENCRYPTION_KEY env value is SHA-256 hashed
// so any non-empty string of arbitrary length produces a valid 32-byte key.
// Use a securely-generated random string in production (e.g. `openssl rand -hex 32`).
package tokencrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Encryptor holds a derived AES-256 key and performs GCM encrypt/decrypt.
type Encryptor struct {
	key [32]byte
}

// New creates an Encryptor from a raw key string.
// Returns an error if the key is empty.
func New(rawKey string) (*Encryptor, error) {
	if rawKey == "" {
		return nil, errors.New("TOKEN_ENCRYPTION_KEY must not be empty")
	}
	return &Encryptor{key: sha256.Sum256([]byte(rawKey))}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64url-encoded
// ciphertext of the form: nonce || ciphertext || tag.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt. Returns the original plaintext or an error if
// the ciphertext is corrupt or was tampered with.
func (e *Encryptor) Decrypt(encoded string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypting token: %w", err)
	}
	return string(plaintext), nil
}
