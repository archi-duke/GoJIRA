// Package envload 는 GoJIRA Go 서비스의 .env 로딩 인프라.
//
// 기존 config.go 의 loadEnvFile(".env") 는 현재 작업 디렉토리(cwd) 기준 상대 경로를
// 열어 systemd / Windows Service / SCCM 배포 등 cwd 가 binary 위치와 다른 환경에서
// 조용히 실패했다. 본 패키지는 다음 순서로 .env 후보 경로를 탐색해 첫 성공을 채택한다:
//
//	1. {SERVICE}_ENV_FILE 환경변수 (예: PLATFORM_ENV_FILE) 가 가리키는 절대/상대 경로
//	2. 현재 작업 디렉토리 (./.env)
//	3. binary 실행파일 디렉토리 (<exeDir>/.env)
//	4. binary 부모 디렉토리 (<exeDir>/../.env — 개발 환경에서 cmd 하위 binary 대비)
//
// 모든 시도와 결과를 log 로 출력 — silent 실패 금지. 시스템 환경변수가 이미 설정된
// 키는 덮어쓰지 않는다 (시스템 우선). UTF-8 BOM, CRLF, "value" 양끝 따옴표를 트림.
package envload

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Load 는 overrideEnvVar (예: "PLATFORM_ENV_FILE") 가 지정되어 있으면 가장 먼저 그
// 경로를 시도하고, 이후 cwd / binary 디렉토리 / 그 부모 디렉토리 순으로 .env 파일을
// 탐색해 첫 성공을 채택한다. 못 찾으면 경고 로그만 출력 (fail-fast 는 호출자의
// requireEnv 에 위임).
func Load(overrideEnvVar string) {
	candidates := buildCandidates(overrideEnvVar)
	tried := make([]string, 0, len(candidates))

	for _, p := range candidates {
		absPath, _ := filepath.Abs(p)
		tried = append(tried, absPath)
		count, ok := loadIfExists(p)
		if !ok {
			continue
		}
		log.Printf("[envload] loaded %d entries from %s", count, absPath)
		return
	}

	log.Printf("[envload] .env not found — tried: %v. 시스템 환경변수만 사용합니다.", tried)
}

func buildCandidates(overrideEnvVar string) []string {
	var out []string
	if overrideEnvVar != "" {
		if p := strings.TrimSpace(os.Getenv(overrideEnvVar)); p != "" {
			out = append(out, p)
		}
	}
	out = append(out, ".env")
	if exePath, err := os.Executable(); err == nil {
		// macOS / Linux 의 symlink (특히 /var/run/com.apple.security.cryptexd/... 같은
		// 경로) 를 그대로 두면 binary 옆의 실제 .env 를 찾지 못하므로 EvalSymlinks 시도.
		if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
			exePath = resolved
		}
		exeDir := filepath.Dir(exePath)
		out = append(out, filepath.Join(exeDir, ".env"))
		out = append(out, filepath.Join(exeDir, "..", ".env"))
	}
	return out
}

func loadIfExists(path string) (int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// 큰 비밀키도 한 줄에 들어갈 수 있도록 버퍼 확장 (기본 64KiB)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	count := 0
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			// UTF-8 BOM (U+FEFF, 0xEF 0xBB 0xBF) 제거. 윈도우즈 메모장 저장 시 흔히 붙음.
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// 양 끝이 짝지어진 따옴표라면 제거 — `KEY="value"` 또는 `KEY='value'` 호환
		if n := len(val); n >= 2 {
			if (val[0] == '"' && val[n-1] == '"') || (val[0] == '\'' && val[n-1] == '\'') {
				val = val[1 : n-1]
			}
		}
		if key == "" {
			continue
		}
		// 시스템 환경변수가 이미 설정되어 있으면 덮어쓰지 않음 (시스템 우선)
		if os.Getenv(key) != "" {
			continue
		}
		os.Setenv(key, val)
		count++
	}
	return count, true
}
