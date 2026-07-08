package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// install_prompt.go — `install` 대화형 입력. 웹의 설치 안내 모달이 요구하는 3가지 값
// (gateway / git.user / git.token) 을 사람이 JSON 을 직접 열어 채우지 않아도 되게 한다.
//
// 비대화형(TTY 아님 — CI/스크립트/파이프)일 때는 절대 프롬프트를 띄우지 않고 즉시
// 넘어간다(기존 --gateway 플래그만 쓰는 자동화 흐름과 100% 호환).
// 이미 값이 있는 항목(재설치/업데이트)은 다시 묻지 않는다 — Enter 만 치면 기존 값 유지.

// isInteractiveStdin — stdin 이 실제 터미널(사람이 입력 중)인지 판별. 외부 의존성
// 없이 표준 os.ModeCharDevice 로 판별 — Windows 콘솔/Unix TTY 모두 동작.
func isInteractiveStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// promptInstallAnswers — 대화형 터미널일 때만 비어있는 값을 되묻는다. 기존 값이 있으면
// 그 값을 기본값으로 보여주고, Enter 만 치면 그대로 유지한다. 반환값은 최종적으로
// 설정에 반영할 값(변경 없으면 기존 값 그대로).
func promptInstallAnswers(existingGateway, existingGitUser, existingGitToken string) (gateway, gitUser, gitToken string) {
	gateway, gitUser, gitToken = existingGateway, existingGitUser, existingGitToken
	if !isInteractiveStdin() {
		return
	}

	fmt.Println("\n대화형 설치 — 값을 입력하고 Enter (비워두면 기존 값 유지, 나중에 launcher.json 에서 채워도 됩니다)")
	reader := bufio.NewReader(os.Stdin)

	if v := promptLine(reader, "게이트웨이 주소", existingGateway, "예: https://gojira-host:44300"); v != "" {
		gateway = v
	}
	if v := promptLine(reader, "Gitea 사용자명 (git.user)", existingGitUser, "폐쇄망 인증용, 관리자에게 문의"); v != "" {
		gitUser = v
	}
	if v := promptLine(reader, "Gitea 개인 액세스 토큰 (git.token)", maskToken(existingGitToken), "화면에 그대로 표시됩니다 — 주변 확인"); v != "" {
		// 마스킹된 기존 값을 그대로 Enter 로 넘긴 경우와 구분 — 사용자가 실제로 새 값을
		// 입력했을 때만 교체한다. maskToken 결과와 동일하면(=Enter 로 그대로 둔 것) 무시.
		if v != maskToken(existingGitToken) {
			gitToken = v
		}
	}
	return
}

// promptLine — 라벨 + (있으면) 힌트/기존값을 보여주고 한 줄 입력을 받는다. 빈 입력이면 "".
func promptLine(reader *bufio.Reader, label, existing, hint string) string {
	suffix := ""
	if existing != "" {
		suffix = fmt.Sprintf(" [현재: %s]", existing)
	} else if hint != "" {
		suffix = fmt.Sprintf(" (%s)", hint)
	}
	fmt.Printf("%s%s: ", label, suffix)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// maskToken — 토큰 표시용 마스킹(앞 6자 + ...). 로그/프롬프트 노출 최소화.
func maskToken(t string) string {
	if t == "" {
		return ""
	}
	if len(t) <= 6 {
		return t[:1] + "..."
	}
	return t[:6] + "..."
}
