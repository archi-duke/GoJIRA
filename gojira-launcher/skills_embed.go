package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// embeddedAssets — 빌드 시 deploy/scripts/build-launcher.sh 가 레포 루트 `.claude` 를
// `assets/.claude` 로 복사한 뒤 컴파일하면, GoJIRA 스킬 트리가 exe 안에 통째로 박힌다.
// → 폐쇄망에 exe 단일 파일만 전달하면 스킬까지 함께 배포된다.
//
// `all:` 프리픽스는 `.`/`_` 로 시작하는 파일(.claude 하위)도 포함시킨다.
// README.txt 가 항상 존재하므로 스킬을 복사하지 않고 `go build` 해도 컴파일은 성공한다
// (이 경우 추출 단계가 .claude 부재를 감지해 스킵).
//
//go:embed all:assets
var embeddedAssets embed.FS

// skillsVersionMarker — 추출 대상 디렉터리에 기록하는 버전 마커 파일명.
// 마커 내용이 현재 launcher 버전과 같으면 재추출을 건너뛴다(매 기동 비용 회피).
const skillsVersionMarker = ".skills-version"

// hasEmbeddedSkills — exe 에 .claude/skills 가 임베드돼 있는지.
func hasEmbeddedSkills() bool {
	if _, err := fs.Stat(embeddedAssets, "assets/.claude"); err != nil {
		return false
	}
	return true
}

// ExtractSkills — 임베드된 assets/ 의 에이전트 자산(.claude/.opencode/opencode.json)을
// dest(예: ~/.gojira/skills) 아래로 추출한다. 이후 ensureAgentAssets 가 이들을 각 clone 에 연결한다.
//
// 멱등성: dest/.skills-version 마커가 ver 와 같으면 노옵(재추출 스킵).
// force=true 면 마커 무시하고 항상 재추출(업데이트 직후 등).
//
// 부분 추출/구버전 잔여를 막기 위해 추출 전 provision 엔트리를 통째로 지우고 다시 쓴다
// (이 디렉터리는 launcher 가 소유 — 사용자 데이터 아님).
func ExtractSkills(dest, ver string, force bool) error {
	if !hasEmbeddedSkills() {
		log.Printf("에이전트 자산 추출 스킵 — exe 에 임베드된 자산 없음 (개발 빌드). skillsSourceDir=%s", dest)
		return nil
	}

	markerPath := filepath.Join(dest, skillsVersionMarker)
	if !force {
		if data, err := os.ReadFile(markerPath); err == nil && string(data) == ver {
			log.Printf("에이전트 자산 최신(v%s) — 추출 스킵: %s", ver, dest)
			return nil
		}
	}

	// 기존 provision 자산 제거 후 재추출.
	for _, e := range agentAssetEntries {
		if err := os.RemoveAll(filepath.Join(dest, e)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	count := 0
	err := fs.WalkDir(embeddedAssets, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "assets" {
			return nil
		}
		rel, rerr := filepath.Rel("assets", p) // assets/.claude/... → .claude/...
		if rerr != nil {
			return rerr
		}
		if rel == "README.txt" { // embed placeholder — 추출 대상 아님.
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := embeddedAssets.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		// .sh 스크립트는 실행권한 부여(스킬이 _scripts 의 doc-save.sh 등을 호출).
		mode := os.FileMode(0o644)
		if filepath.Ext(target) == ".sh" {
			mode = 0o755
		}
		if werr := os.WriteFile(target, data, mode); werr != nil {
			return werr
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(markerPath, []byte(ver), 0o644); err != nil {
		return err
	}
	log.Printf("에이전트 자산 추출 완료 — %d개 파일 → %s (v%s)", count, dest, ver)
	return nil
}
