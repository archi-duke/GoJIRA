package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"
)

// 서명 검증 — 올바른 키/서명은 통과, 변조·타 키·빈 서명은 실패해야 한다.
func TestVerifyManifestSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	other, _, _ := ed25519.GenerateKey(nil)
	raw := []byte(`{"version":"0.2.0","issuedAt":"2026-11-05T00:00:00Z"}`)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))

	saved := rootPublicKeys
	defer func() { rootPublicKeys = saved }()

	t.Run("정상 서명", func(t *testing.T) {
		rootPublicKeys = []string{base64.StdEncoding.EncodeToString(pub), ""}
		if ok, why := verifyManifestSignature(raw, sig); !ok {
			t.Fatalf("통과해야 하는데 실패: %s", why)
		}
	})

	t.Run("next 슬롯으로도 통과", func(t *testing.T) {
		rootPublicKeys = []string{base64.StdEncoding.EncodeToString(other), base64.StdEncoding.EncodeToString(pub)}
		if ok, why := verifyManifestSignature(raw, sig); !ok {
			t.Fatalf("next 슬롯 검증 실패: %s", why)
		}
	})

	t.Run("원문 변조", func(t *testing.T) {
		rootPublicKeys = []string{base64.StdEncoding.EncodeToString(pub), ""}
		if ok, _ := verifyManifestSignature([]byte(`{"version":"9.9.9"}`), sig); ok {
			t.Fatal("변조된 원문이 통과했다")
		}
	})

	t.Run("다른 키", func(t *testing.T) {
		rootPublicKeys = []string{base64.StdEncoding.EncodeToString(other), ""}
		if ok, _ := verifyManifestSignature(raw, sig); ok {
			t.Fatal("다른 키로 통과했다")
		}
	})

	t.Run("서명 없음 / 키 미설정", func(t *testing.T) {
		rootPublicKeys = []string{base64.StdEncoding.EncodeToString(pub), ""}
		if ok, why := verifyManifestSignature(raw, ""); ok || why != "서명 없음" {
			t.Fatalf("빈 서명 처리 오류: ok=%v why=%s", ok, why)
		}
		rootPublicKeys = []string{"", ""}
		if ok, why := verifyManifestSignature(raw, sig); ok || why != "루트 공개키 미설정" {
			t.Fatalf("키 미설정 처리 오류: ok=%v why=%s", ok, why)
		}
	})
}

// 경고 모드(A)에서는 서명이 없어도 통과하되, issuedAt 역행은 모드와 무관하게 거부한다.
func TestAcceptManifest(t *testing.T) {
	saved := rootPublicKeys
	defer func() { rootPublicKeys = saved }()
	rootPublicKeys = []string{"", ""}

	now := time.Now().UTC().Format(time.RFC3339)
	past := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	raw := []byte(`{"version":"0.2.0"}`)

	if signatureMode != "warn" {
		t.Skipf("현재 모드가 %q — 이 테스트는 warn 전제", signatureMode)
	}

	t.Run("무서명이어도 통과(경고 모드)", func(t *testing.T) {
		ok, err := acceptManifest(raw, "", &ReleaseManifest{Version: "0.2.0", IssuedAt: now}, "")
		if !ok || err != nil {
			t.Fatalf("경고 모드에서 막혔다: ok=%v err=%v", ok, err)
		}
	})

	t.Run("issuedAt 역행은 거부", func(t *testing.T) {
		ok, err := acceptManifest(raw, "", &ReleaseManifest{Version: "0.2.0", IssuedAt: past}, now)
		if ok || err == nil {
			t.Fatal("역행 매니페스트가 수용됐다")
		}
	})

	t.Run("기록 없으면 역행 검사 생략", func(t *testing.T) {
		if ok, err := acceptManifest(raw, "", &ReleaseManifest{Version: "0.2.0", IssuedAt: past}, ""); !ok || err != nil {
			t.Fatalf("최초 수용이 막혔다: ok=%v err=%v", ok, err)
		}
	})
}

// 업데이트 실패 기억 — 서버 매니페스트와 실제 바이너리가 어긋나도
// 클라이언트가 무한 재시도에 빠지지 않아야 한다.
func TestUpdateStateLoopGuard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	t.Run("기록 없으면 아무것도 건너뛰지 않는다", func(t *testing.T) {
		if shouldSkipVersion("9.9.9") {
			t.Fatal("빈 상태에서 건너뛰었다")
		}
	})

	t.Run("교체했는데 버전이 그대로면 그 버전을 실패로 기록", func(t *testing.T) {
		markUpdateAttempt("99.0.0") // 현재 version 보다 높은 목표
		reconcileUpdateState()      // 재기동 시점 — 실제 버전은 그대로다
		if !shouldSkipVersion("99.0.0") {
			t.Fatal("실패한 버전을 건너뛰지 않는다 — 무한 루프에 빠진다")
		}
	})

	t.Run("더 높은 새 버전은 다시 시도한다", func(t *testing.T) {
		if shouldSkipVersion("99.0.1") {
			t.Fatal("다른 버전까지 막혔다")
		}
	})

	t.Run("목표만큼 올라왔으면 기록을 지운다", func(t *testing.T) {
		markUpdateAttempt("0.0.1") // 현재 version 보다 낮은 목표 = 성공 상태
		reconcileUpdateState()
		if shouldSkipVersion("0.0.1") || loadUpdateState().Pending != "" {
			t.Fatalf("성공인데 상태가 남았다: %+v", loadUpdateState())
		}
	})
}
