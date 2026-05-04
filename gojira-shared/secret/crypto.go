// Package secret 은 GoJIRA 의 모든 서비스가 공유하는 secret 암복호화 단일 소스다.
//
// AES-256-GCM, "ENC:" prefix 가 있는 base64 ciphertext 를 평문으로 복호화한다.
// prefix 가 없으면 그대로 통과시켜 점진 도입을 가능하게 한다.
//
// 마스터 키는 GOJIRA_ENC_KEY 환경변수에서 매 호출 시 읽는다 — 서비스의 .env
// 로딩이 끝난 뒤에도 정상 반영되도록 init 시점에 캐시하지 않는다.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

const defaultKey = "G0J1R4-S3cR3t-K3y!2026@SAI-Corp!"

func key() []byte {
	if v := os.Getenv("GOJIRA_ENC_KEY"); v != "" {
		return []byte(v)
	}
	return []byte(defaultKey)
}

// Encrypt 는 평문을 AES-256-GCM 으로 암호화하고 "ENC:" prefix 가 붙은 base64 를 반환한다.
func Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(key())
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

// Decrypt 는 "ENC:" prefix 가 있는 base64 ciphertext 를 평문으로 복호화한다.
// prefix 가 없으면 입력을 그대로 반환한다 (평문 호환).
func Decrypt(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "ENC:") {
		return encoded, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "ENC:"))
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}
	block, err := aes.NewCipher(key())
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
