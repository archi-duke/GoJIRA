package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// runSubcommand — 단일 exe 가 데몬 외에 install/uninstall/version/update-finalize 를 처리한다.
// 반환값 shouldExit=true 면 호출부(main)가 즉시 종료해야 한다.
// update-finalize 는 정리만 하고 false 를 반환 → 그대로 데몬으로 진행.
func runSubcommand(args []string) (shouldExit bool) {
	if len(args) == 0 {
		return false // 인자 없음 → 데몬
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Printf("gojira-launcher v%s\n", version)
		return true
	case "install":
		if err := doInstall(); err != nil {
			fmt.Fprintf(os.Stderr, "설치 실패: %v\n", err)
			os.Exit(1)
		}
		return true
	case "uninstall":
		if err := doUninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "제거 실패: %v\n", err)
			os.Exit(1)
		}
		return true
	case "update-finalize":
		// 자동 업데이트 직후 새 exe 가 자기 자신을 이 인자로 재기동 — 구 exe(.old) 정리 후 데몬 계속.
		if len(args) >= 2 {
			finalizeUpdate(args[1])
		}
		return false
	case "run", "daemon":
		return false // 명시적 데몬 실행
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 명령: %s\n", args[0])
		printUsage()
		return true
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `gojira-launcher — GoJIRA 로컬 소환 데몬

사용법:
  gojira-launcher                 데몬 실행 (기본)
  gojira-launcher install         설치(자기복사 + 스킬배포 + 설정파일 + 자동시작 등록) 후 기동
  gojira-launcher uninstall       자동시작 해제 + 설치 정리
  gojira-launcher version         버전 출력
`)
}

// doInstall — 단일 exe 로부터 전체 설치를 수행한다:
//  1. 자기 자신을 설치 디렉터리로 복사(이미 그 위치면 스킵)
//  2. launcher.json placeholder + 설치 토큰 생성(LoadConfig)
//  3. 임베드 스킬을 skillsSourceDir 로 추출
//  4. OS 자동시작 등록(Windows: HKCU Run 키)
//  5. 데몬 기동(detached)
func doInstall() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("실행 파일 경로 확인 실패: %w", err)
	}
	self, _ = filepath.EvalSymlinks(self)

	dir := installDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("설치 디렉터리 생성 실패(%s): %w", dir, err)
	}
	target := filepath.Join(dir, exeName())

	if !sameFile(self, target) {
		if err := copyFile(self, target, 0o755); err != nil {
			return fmt.Errorf("exe 복사 실패(%s → %s): %w", self, target, err)
		}
		fmt.Printf("✓ exe 설치: %s\n", target)
	} else {
		fmt.Printf("✓ exe 이미 설치 위치에서 실행 중: %s\n", target)
	}

	// 설정/토큰 — placeholder 생성 트리거.
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("설정 초기화 실패: %w", err)
	}
	fmt.Printf("✓ 설정/토큰 준비: ~/.gojira/%s, ~/.gojira/%s\n", configFileName, tokenFileName)

	// 스킬 추출.
	if err := ExtractSkills(cfg.SkillsSourceDir, version, true); err != nil {
		log.Printf("스킬 추출 실패(비치명적): %v", err)
	} else if hasEmbeddedSkills() {
		fmt.Printf("✓ 스킬 배포: %s/.claude\n", cfg.SkillsSourceDir)
	}

	// 자동시작 등록.
	if err := registerAutostart(target); err != nil {
		log.Printf("자동시작 등록 실패(비치명적): %v", err)
	} else {
		fmt.Printf("✓ 자동시작 등록 완료\n")
	}

	// 데몬 기동(detached).
	if err := startDetached(target); err != nil {
		log.Printf("데몬 자동 기동 실패(비치명적 — 수동 실행 가능): %v", err)
	} else {
		fmt.Printf("✓ 데몬 기동: http://127.0.0.1:%d\n", cfg.Port)
	}

	fmt.Printf("\n설치 완료. 폐쇄망에서는 ~/.gojira/%s 의 git.user/git.token 을 채우고\n런처를 재시작(또는 재로그인)하세요.\n", configFileName)
	return nil
}

// doUninstall — 자동시작 해제 + 설치된 exe 정리. 사용자 설정(launcher.json)/스킬은 보존.
func doUninstall() error {
	if err := unregisterAutostart(); err != nil {
		log.Printf("자동시작 해제 실패(비치명적): %v", err)
	} else {
		fmt.Printf("✓ 자동시작 해제\n")
	}
	target := filepath.Join(installDir(), exeName())
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		// 실행 중이면 삭제 불가 — 다음 부팅 정리 안내.
		fmt.Printf("! 설치 exe 제거 보류(실행 중일 수 있음): %s\n", target)
	} else {
		fmt.Printf("✓ 설치 exe 제거: %s\n", target)
	}
	fmt.Printf("\n설정(~/.gojira/%s)과 스킬은 보존했습니다. 완전 삭제하려면 ~/.gojira/skills 와 설정 파일을 직접 지우세요.\n", configFileName)
	return nil
}

// copyFile — src → dst 복사(권한 mode). dst 가 실행 중이면 실패할 수 있음(호출부에서 처리).
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// sameFile — 두 경로가 같은 파일을 가리키는지(설치 위치에서 직접 실행된 경우 자기복사 스킵).
func sameFile(a, b string) bool {
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(ai, bi)
}
