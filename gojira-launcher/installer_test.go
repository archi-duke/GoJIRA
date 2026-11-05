package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// zip slip — 패키지가 설치 디렉터리를 벗어나는 경로를 담고 있으면 거부해야 한다.
func TestUnzipRejectsPathEscape(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../escaped.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	_ = f.Close()

	err := unzipTo(zipPath, filepath.Join(dir, "dest"))
	if err == nil || !strings.Contains(err.Error(), "벗어나는") {
		t.Fatalf("경로 탈출을 막지 못했다: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); err == nil {
		t.Fatal("디렉터리 밖에 파일이 생성됐다")
	}
}

func TestUnzipNormal(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ok.zip")
	f, _ := os.Create(zipPath)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("sub/a.txt")
	_, _ = w.Write([]byte("hello"))
	_ = zw.Close()
	_ = f.Close()

	dest := filepath.Join(dir, "dest")
	if err := unzipTo(zipPath, dest); err != nil {
		t.Fatalf("해제 실패: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "sub", "a.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatalf("내용 불일치: %q %v", b, err)
	}
}

func TestAcceptCatalog(t *testing.T) {
	saved := catalogPublicKeys
	defer func() { catalogPublicKeys = saved }()
	catalogPublicKeys = []string{"", ""} // 키 미설정 — warn 모드에서는 통과해야 한다

	if signatureMode != "warn" {
		t.Skipf("현재 모드 %q — warn 전제", signatureMode)
	}

	reset := func() {
		catalogCache.Lock()
		catalogCache.issuedAt = ""
		catalogCache.Unlock()
	}

	t.Run("무서명이어도 경고 모드에서는 통과", func(t *testing.T) {
		reset()
		cat := &Catalog{IssuedAt: time.Now().UTC().Format(time.RFC3339)}
		if err := acceptCatalog([]byte("{}"), "", cat); err != nil {
			t.Fatalf("막혔다: %v", err)
		}
	})

	t.Run("만료된 카탈로그는 거부", func(t *testing.T) {
		reset()
		cat := &Catalog{
			IssuedAt:  time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
			ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		}
		if err := acceptCatalog([]byte("{}"), "", cat); err == nil {
			t.Fatal("만료 카탈로그가 통과했다")
		}
	})

	t.Run("issuedAt 역행은 거부", func(t *testing.T) {
		reset()
		now := time.Now().UTC()
		catalogCache.Lock()
		catalogCache.issuedAt = now.Format(time.RFC3339)
		catalogCache.Unlock()
		cat := &Catalog{IssuedAt: now.Add(-24 * time.Hour).Format(time.RFC3339)}
		if err := acceptCatalog([]byte("{}"), "", cat); err == nil {
			t.Fatal("역행 카탈로그가 통과했다")
		}
	})
}

// 설치 전체 흐름 — 패키지를 서빙하는 가짜 Platform-API 로 다운로드·검증·배치를 확인한다.
func TestInstallService(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// 패키지 zip 구성: service.json + 더미 실행 파일
	spec := ServiceSpec{
		Name: "demo-svc", Version: "1.0.0", Entry: "demo",
		Run: RunSpec{Mode: "on-demand", IdleTimeoutMin: 5},
	}
	specJSON, _ := json.Marshal(spec)

	buf := new(strings.Builder)
	_ = buf
	zipPath := filepath.Join(t.TempDir(), "pkg.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	w1, _ := zw.Create("service.json")
	_, _ = w1.Write(specJSON)
	w2, _ := zw.Create("demo")
	_, _ = w2.Write([]byte("#!/bin/sh\necho demo\n"))
	_ = zw.Close()
	_ = zf.Close()

	raw, _ := os.ReadFile(zipPath)
	sum := sha256.Sum256(raw)
	shaHex := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pkg.zip") {
			_, _ = w.Write(raw)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := &Server{cfg: &Config{Backend: Backend{PlatformAPI: srv.URL + "/api/v2"}}}
	cs := &CatalogService{
		Name: "demo-svc", Version: "1.0.0", Policy: "optional",
		Packages: map[string]CatalogPackage{platformKey(): {File: "pkg.zip", SHA256: shaHex}},
	}

	got, err := s.installService(cs, nil)
	if err != nil {
		t.Fatalf("설치 실패: %v", err)
	}
	if got.Name != "demo-svc" || got.Version != "1.0.0" {
		t.Fatalf("spec 불일치: %+v", got)
	}
	// 버전별 디렉터리 + current 포인터
	if _, err := os.Stat(filepath.Join(home, ".gojira", "services", "demo-svc", "1.0.0", "service.json")); err != nil {
		t.Fatalf("설치 위치에 service.json 없음: %v", err)
	}
	if cur := readCurrentVersion(filepath.Join(home, ".gojira"), "demo-svc"); cur != "1.0.0" {
		t.Fatalf("current=%q", cur)
	}
	// 재설치는 no-op
	if _, err := s.installService(cs, nil); err != nil {
		t.Fatalf("재설치가 실패했다: %v", err)
	}

	t.Run("sha256 불일치는 거부", func(t *testing.T) {
		bad := *cs
		bad.Version = "2.0.0"
		bad.Packages = map[string]CatalogPackage{platformKey(): {File: "pkg.zip", SHA256: strings.Repeat("0", 64)}}
		if _, err := s.installService(&bad, nil); err == nil || !strings.Contains(err.Error(), "sha256") {
			t.Fatalf("해시 불일치를 막지 못했다: %v", err)
		}
	})

	t.Run("카탈로그와 패키지 내용이 다르면 거부", func(t *testing.T) {
		mismatch := *cs
		mismatch.Version = "3.0.0" // 패키지 안 service.json 은 1.0.0
		mismatch.Packages = map[string]CatalogPackage{platformKey(): {File: "pkg.zip", SHA256: shaHex}}
		_, err := s.installService(&mismatch, nil)
		if err == nil || !strings.Contains(err.Error(), "카탈로그와 다릅니다") {
			t.Fatalf("불일치를 막지 못했다: %v", err)
		}
	})

	t.Run("플랫폼 패키지 없으면 거부", func(t *testing.T) {
		np := *cs
		np.Version = "4.0.0"
		np.Packages = map[string]CatalogPackage{"nonexistent-arch": {File: "x.zip", SHA256: shaHex}}
		if _, err := s.installService(&np, nil); err == nil {
			t.Fatal("플랫폼 미지원을 막지 못했다")
		}
	})

	_ = fmt.Sprint()
}

func TestCheckMinLauncher(t *testing.T) {
	t.Run("빈 값은 제약 없음", func(t *testing.T) {
		if err := checkMinLauncher(""); err != nil {
			t.Fatalf("제약 없어야 하는데 막혔다: %v", err)
		}
	})
	t.Run("현재 버전과 같으면 통과", func(t *testing.T) {
		if err := checkMinLauncher(version); err != nil {
			t.Fatalf("동일 버전이 막혔다: %v", err)
		}
	})
	t.Run("현재보다 높으면 거부", func(t *testing.T) {
		err := checkMinLauncher("99.0.0")
		if err == nil || !strings.Contains(err.Error(), "런처 버전이 낮습니다") {
			t.Fatalf("상위 버전 요구를 막지 못했다: %v", err)
		}
	})
	t.Run("현재보다 낮으면 통과", func(t *testing.T) {
		if err := checkMinLauncher("0.0.1"); err != nil {
			t.Fatalf("하위 버전 요구가 막혔다: %v", err)
		}
	})
}
