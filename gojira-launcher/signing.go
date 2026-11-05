package main

// 공급망 서명 검증 (설계서 §8 "공급망", §10.6, §10.7 — 이슈 #116).
//
// 배경: 현행 자기 업데이트는 sha256 과 바이너리를 같은 서버에서 받는다 — 전송 손상은 막아도
// 배포 경로가 오염되면 막지 못한다(무결성 확인, 출처 미확인). 서명 매니페스트로 출처까지 검증한다.
//
// 키는 2종이며 이 파일은 그중 루트 키(런처 매니페스트)만 다룬다. 카탈로그 키는 1단계에서 추가한다.
// 키별 공개키는 2슬롯(current, next) — next 를 미리 배포한 뒤 서명 키를 교체하고, 다음 릴리스에서
// 구 키를 제거한다(§10.6 "키 교체").
//
// 이행은 2단계(§10.7):
//
//	A. 경고 모드 — 서명이 없거나 불일치해도 경고만 남기고 기존 sha256 경로로 진행 (≤0.1.38 구제)
//	B. 필수 모드 — 검증 실패 시 교체 거부
//
// 현재 단계는 아래 signatureMode 로 전환한다.

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// signatureMode — "warn"(A단계) | "require"(B단계).
// A 가 충분히 보급된 뒤(Platform-API 접속 로그의 버전 분포로 판단) "require" 로 올린다.
const signatureMode = "warn"

// rootPublicKeys — 런처 매니페스트 검증용 루트 공개키 (base64, ed25519).
// current 가 운용 키, next 는 교체 예정 키. `gojira-sign keygen` 산출물의 PUBLIC 을 넣는다.
// 비어 있으면 검증을 수행할 수 없다 — warn 모드에서는 경고 후 통과, require 모드에서는 거부.
var rootPublicKeys = []string{
	"", // current — TODO: 루트 키 생성 후 채울 것 (launcher-release environment secret 과 쌍)
	"", // next    — 키 교체 시 사용
}

// ReleaseManifest — 루트 키로 서명되는 런처 릴리스 매니페스트 원문.
// Platform-API `GET /api/v2/launcher/release-manifest` 가 파일 그대로 서빙하고,
// 서명은 같은 경로의 `.sig`(base64) 로 제공한다. 서명 대상은 파일 바이트 그대로다.
type ReleaseManifest struct {
	Version   string                  `json:"version"`
	Platforms map[string]PlatformInfo `json:"platforms"`
	IssuedAt  string                  `json:"issuedAt"`
	Commit    string                  `json:"commit,omitempty"`
	RunID     string                  `json:"runId,omitempty"`
}

// verifyManifestSignature — 런처 매니페스트를 루트 공개키로 검증한다.
// 실제 검증은 카탈로그와 공유하는 verifyWithKeys(catalog.go)가 수행한다 — 키 슬롯만 다르다.
func verifyManifestSignature(raw []byte, sigB64 string) (bool, string) {
	ok, reason := verifyWithKeys(rootPublicKeys, raw, sigB64)
	if !ok && reason == "공개키 미설정" {
		return false, "루트 공개키 미설정"
	}
	return ok, reason
}

// acceptManifest — 서명 검증 + 재전송/다운그레이드 방지를 적용해 매니페스트 수용 여부를 정한다.
// lastIssuedAt 은 마지막으로 수용한 매니페스트의 issuedAt(없으면 빈 문자열).
//
// 반환 ok=false 면 호출자는 업데이트를 중단한다. warn 모드에서는 서명 문제만으로는
// ok=false 가 되지 않는다(경고 로그만) — 다만 issuedAt 역행은 모드와 무관하게 거부한다.
func acceptManifest(raw []byte, sigB64 string, m *ReleaseManifest, lastIssuedAt string) (bool, error) {
	ok, reason := verifyManifestSignature(raw, sigB64)
	if !ok {
		if signatureMode == "require" {
			return false, fmt.Errorf("매니페스트 서명 검증 실패(%s) — 필수 모드이므로 업데이트 중단", reason)
		}
		log.Printf("⚠️  매니페스트 서명 검증 실패(%s) — 경고 모드라 sha256 검증으로 진행합니다", reason)
	}

	// 재전송·다운그레이드 방지 — 마지막으로 수용한 것보다 오래된 매니페스트 거부(§10.6).
	if lastIssuedAt != "" && m.IssuedAt != "" {
		prev, err1 := time.Parse(time.RFC3339, lastIssuedAt)
		cur, err2 := time.Parse(time.RFC3339, m.IssuedAt)
		if err1 == nil && err2 == nil && cur.Before(prev) {
			return false, fmt.Errorf("매니페스트 issuedAt 역행(%s < %s) — 거부", m.IssuedAt, lastIssuedAt)
		}
	}
	return true, nil
}

// parseReleaseManifest — 원문 바이트를 매니페스트로 해석.
func parseReleaseManifest(raw []byte) (*ReleaseManifest, error) {
	var m ReleaseManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("매니페스트 파싱 실패: %w", err)
	}
	if m.Version == "" {
		return nil, fmt.Errorf("매니페스트에 version 없음")
	}
	return &m, nil
}
