package security

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "errors"
    "io"
)

// Envelope provides simple AES-GCM based envelope encryption.
// The master key should be 32 bytes (AES-256) provided via environment or secret.
// This utility is deliberately small and dependency-free.

func Encrypt(masterKey []byte, plaintext []byte, aad []byte) (string, error) {
    if len(masterKey) != 32 {
        return "", errors.New("master key must be 32 bytes (AES-256)")
    }
    block, err := aes.NewCipher(masterKey)
    if err != nil { return "", err }
    gcm, err := cipher.NewGCM(block)
    if err != nil { return "", err }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
    ct := gcm.Seal(nonce, nonce, plaintext, aad)
    return base64.StdEncoding.EncodeToString(ct), nil
}

func Decrypt(masterKey []byte, b64 string, aad []byte) ([]byte, error) {
    if len(masterKey) != 32 {
        return nil, errors.New("master key must be 32 bytes (AES-256)")
    }
    raw, err := base64.StdEncoding.DecodeString(b64)
    if err != nil { return nil, err }
    block, err := aes.NewCipher(masterKey)
    if err != nil { return nil, err }
    gcm, err := cipher.NewGCM(block)
    if err != nil { return nil, err }
    if len(raw) < gcm.NonceSize() { return nil, errors.New("ciphertext too short") }
    nonce := raw[:gcm.NonceSize()]
    data := raw[gcm.NonceSize():]
    pt, err := gcm.Open(nil, nonce, data, aad)
    if err != nil { return nil, err }
    return pt, nil
}

