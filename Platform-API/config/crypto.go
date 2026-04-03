package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// 암호화 키 (32 bytes = AES-256). 환경변수로 오버라이드 가능.
var encryptionKey = envOrDefault("GOJIRA_ENC_KEY", "G0J1R4-S3cR3t-K3y!2026@SAI-Corp!")

// Encrypt encrypts plaintext using AES-256-GCM and returns "ENC:" prefixed base64 string.
func Encrypt(plaintext string) (string, error) {
	key := []byte(encryptionKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "ENC:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts an "ENC:" prefixed base64 string using AES-256-GCM.
func Decrypt(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "ENC:") {
		return encoded, nil // 평문이면 그대로 반환
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "ENC:"))
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	key := []byte(encryptionKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}

	return string(plaintext), nil
}
