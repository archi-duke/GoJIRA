package config

// crypto.go: 공용 secret 모듈로 위임 (gojira-shared/secret 가 단일 소스).
// 호출자는 기존처럼 config.Encrypt / config.Decrypt 를 그대로 사용한다.

import "gojira.local/shared/secret"

func Encrypt(plaintext string) (string, error) { return secret.Encrypt(plaintext) }
func Decrypt(encoded string) (string, error)   { return secret.Decrypt(encoded) }
