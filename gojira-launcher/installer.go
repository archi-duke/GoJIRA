package main

// Installer — 서비스 패키지 설치 (설계서 §10.4, §10.5).
//
// 카탈로그에서 패키지 위치·sha256 을 읽어 내려받고, 해시를 검증한 뒤 버전별 디렉터리에
// 풀어 놓는다. 기존 버전을 덮어쓰지 않는 이유는 Windows 에서 실행 중인 exe 가 잠겨
// 교체가 실패하기 때문이다(§10.4) — 새 버전을 따로 깔고 current 포인터만 바꾼다.
//
//	~/.gojira/services/<name>/<version>/…   버전별 설치
//	~/.gojira/services/<name>/current       현재 버전 (텍스트 파일)

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// platformKey — 카탈로그 packages 의 키 (예: windows-amd64).
func platformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

// installService — 카탈로그 항목을 내려받아 설치하고 service.json 을 돌려준다.
// progress 가 nil 이 아니면 단계별 메시지를 보낸다(웹 SSE 중계용).
func (s *Server) installService(cs *CatalogService, progress func(string)) (*ServiceSpec, error) {
	step := func(msg string) {
		log.Printf("설치[%s]: %s", cs.Name, msg)
		if progress != nil {
			progress(msg)
		}
	}

	pkg, ok := cs.Packages[platformKey()]
	if !ok {
		return nil, fmt.Errorf("이 플랫폼(%s)용 패키지가 없습니다", platformKey())
	}
	if ok, missing := checkRequirements(cs.Requires); !ok {
		return nil, fmt.Errorf("요건 불충족: %v", missing)
	}

	dir, err := gojiraDir()
	if err != nil {
		return nil, err
	}
	versionDir := filepath.Join(dir, "services", cs.Name, cs.Version)
	if _, err := os.Stat(filepath.Join(versionDir, "service.json")); err == nil {
		step("이미 설치됨 — 건너뜀")
		return readServiceSpec(versionDir)
	}

	// 다운로드 — 카탈로그의 file 은 Platform-API 경로 기준이다(§10.6 패키지 저장소).
	base := strings.TrimRight(s.cfg.Backend.PlatformAPI, "/") + "/launcher/services"
	url := fmt.Sprintf("%s/%s/%s/%s", base, cs.Name, cs.Version, pkg.File)
	step("다운로드: " + pkg.File)

	tmp, err := os.CreateTemp("", "gojira-svc-*.zip")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	sum, err := downloadTo(tmp, url)
	_ = tmp.Close()
	if err != nil {
		return nil, fmt.Errorf("다운로드 실패: %w", err)
	}
	if !strings.EqualFold(sum, pkg.SHA256) {
		return nil, fmt.Errorf("sha256 불일치 — 기대 %s, 실제 %s", pkg.SHA256, sum)
	}
	step("sha256 검증 통과")

	// 임시 디렉터리에 풀고, 온전한 경우에만 제자리로 옮긴다(부분 설치 방지).
	staging := versionDir + ".staging"
	_ = os.RemoveAll(staging)
	if err := unzipTo(tmpPath, staging); err != nil {
		_ = os.RemoveAll(staging)
		return nil, fmt.Errorf("압축 해제 실패: %w", err)
	}
	spec, err := readServiceSpec(staging)
	if err != nil {
		_ = os.RemoveAll(staging)
		return nil, err
	}
	// minLauncher(§9.1) — 이 런처로는 못 쓰는 서비스를 깔아두면 호출 때마다 실패한다.
	if err := checkMinLauncher(spec.MinLauncher); err != nil {
		_ = os.RemoveAll(staging)
		return nil, err
	}
	if spec.Name != cs.Name || spec.Version != cs.Version {
		_ = os.RemoveAll(staging)
		return nil, fmt.Errorf("패키지 내용이 카탈로그와 다릅니다 — service.json=%s@%s, 카탈로그=%s@%s",
			spec.Name, spec.Version, cs.Name, cs.Version)
	}
	_ = os.RemoveAll(versionDir)
	if err := os.Rename(staging, versionDir); err != nil {
		_ = os.RemoveAll(staging)
		return nil, fmt.Errorf("설치 위치로 이동 실패: %w", err)
	}

	// current 포인터 — 실패 시 롤백할 수 있도록 이전 값을 보관한다(§10.5).
	prev := readCurrentVersion(dir, cs.Name)
	if err := writeCurrentVersion(dir, cs.Name, cs.Version); err != nil {
		return nil, fmt.Errorf("current 갱신 실패: %w", err)
	}
	if prev != "" && prev != cs.Version {
		step(fmt.Sprintf("current %s → %s (직전 버전 보관)", prev, cs.Version))
		pruneOldVersions(dir, cs.Name, cs.Version, prev)
	}
	step("설치 완료")
	return spec, nil
}

// downloadTo — url 을 w 로 받으면서 sha256 을 계산한다(메모리 적재 없이).
func downloadTo(w io.Writer, url string) (string, error) {
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// unzipTo — zip 을 dest 로 푼다. zip slip(`../` 로 디렉터리 탈출)을 막는다.
func unzipTo(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		target := filepath.Join(dest, f.Name) //nolint:gosec // 바로 아래에서 탈출 검사
		abs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if abs != destAbs && !strings.HasPrefix(abs, destAbs+string(os.PathSeparator)) {
			return fmt.Errorf("패키지에 디렉터리를 벗어나는 경로가 있습니다: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, 512<<20))
		_ = rc.Close()
		_ = out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func readServiceSpec(dir string) (*ServiceSpec, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "service.json"))
	if err != nil {
		return nil, fmt.Errorf("service.json 없음: %w", err)
	}
	var spec ServiceSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("service.json 파싱 실패: %w", err)
	}
	if spec.Name == "" || spec.Version == "" || spec.Entry == "" {
		return nil, fmt.Errorf("service.json 에 name·version·entry 가 모두 있어야 합니다")
	}
	return &spec, nil
}

func readCurrentVersion(gojira, name string) string {
	b, err := os.ReadFile(filepath.Join(gojira, "services", name, "current"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeCurrentVersion(gojira, name, version string) error {
	p := filepath.Join(gojira, "services", name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p, "current"), []byte(version+"\n"), 0o644)
}

// pruneOldVersions — 직전 1개만 남기고 정리한다(§10.5 "직전 1개 버전 보관").
func pruneOldVersions(gojira, name, keep, keepPrev string) {
	base := filepath.Join(gojira, "services", name)
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == keep || e.Name() == keepPrev {
			continue
		}
		_ = os.RemoveAll(filepath.Join(base, e.Name()))
	}
}

// installedServices — 설치된 서비스들의 service.json 을 읽어온다(current 기준).
func installedServices() []ServiceSpec {
	dir, err := gojiraDir()
	if err != nil {
		return nil
	}
	base := filepath.Join(dir, "services")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []ServiceSpec
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cur := readCurrentVersion(dir, e.Name())
		if cur == "" {
			continue
		}
		spec, err := readServiceSpec(filepath.Join(base, e.Name(), cur))
		if err != nil {
			log.Printf("설치된 서비스 %s 읽기 실패: %v", e.Name(), err)
			continue
		}
		out = append(out, *spec)
	}
	return out
}


// checkMinLauncher — 서비스가 요구하는 최소 런처 버전을 만족하는지(§9.1).
// 빈 값이면 제약 없음. 형식이 이상하면 막지 않는다 — 설치를 못 하게 하는 것보다
// 느슨하게 통과시키고 로그를 남기는 쪽이 운영에 안전하다.
func checkMinLauncher(min string) error {
	min = strings.TrimSpace(min)
	if min == "" {
		return nil
	}
	if isNewer(min, version) {
		return fmt.Errorf("런처 버전이 낮습니다 — 이 서비스는 v%s 이상을 요구하지만 현재 v%s 입니다", min, version)
	}
	return nil
}
