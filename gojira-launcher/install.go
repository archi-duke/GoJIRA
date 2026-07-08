package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		if err := doInstall(parseGatewayFlag(args[1:])); err != nil {
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
  gojira-launcher                        데몬 실행 (기본)
  gojira-launcher install                설치(자기복사 + 스킬배포 + 설정파일 + 자동시작 등록) 후 기동
  gojira-launcher install --gateway URL  설치 + launcher.json 의 gateway 자동 기록
                                         (backend URL/webOrigins 자동 파생). 예: --gateway https://gojira-host:44300
  gojira-launcher uninstall              자동시작 해제 + 설치 정리
  gojira-launcher version                버전 출력
`)
}

// parseGatewayFlag — install 인자에서 `--gateway <url>` (또는 `--gateway=<url>`) 을 뽑는다.
// 없으면 "". 웹이 install 명령을 만들 때 자기 origin 을 넣어주면 손편집이 사라진다.
func parseGatewayFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--gateway" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--gateway="); ok {
			return v
		}
	}
	return ""
}

// doInstall — 단일 exe 로부터 전체 설치를 수행한다:
//  1. 자기 자신을 설치 디렉터리로 복사(이미 그 위치면 스킵)
//  2. launcher.json placeholder + 설치 토큰 생성(LoadConfig)
//  3. 임베드 스킬을 skillsSourceDir 로 추출
//  4. OS 자동시작 등록(Windows: HKCU Run 키)
//  5. 데몬 기동(detached)
func doInstall(gateway string) error {
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

	// 기존 데몬 종료 — copyFile 보다 먼저 해야 한다. target 이 이미 실행 중이면(재설치/
	// --gateway 재실행) 그 exe 파일이 잠겨 있어 copyFile 이 "text file busy" 로 실패한다
	// (update.go 의 자동 업데이트가 .old/.new staged swap 을 쓰는 것과 같은 이유).
	// 아직 LoadConfig 를 부르지 않은 시점이라, 기존 설정 파일(있으면)에서 port/token 만
	// 가볍게 읽어 종료 요청한다. 파일이 없거나(최초 설치) 읽기 실패해도 무해 — 그 경우
	// 애초에 떠 있는 데몬이 없으므로 shutdown 요청은 그냥 도달 실패로 끝난다.
	if gdir, derr := gojiraDir(); derr == nil {
		port, token := peekExistingPortToken(gdir)
		stopRunningDaemon(port, token)
	}

	if !sameFile(self, target) {
		if err := copyFile(self, target, 0o755); err != nil {
			return fmt.Errorf("exe 복사 실패(%s → %s): %w", self, target, err)
		}
		fmt.Printf("✓ exe 설치: %s\n", target)
	} else {
		fmt.Printf("✓ exe 이미 설치 위치에서 실행 중: %s\n", target)
	}

	// gateway 가 넘어오면 launcher.json 에 자동 기록 — backend URL/webOrigins 는
	// 런처가 파생하므로 이후 손편집 불요(자동화). LoadConfig 이전에 기록해야 파생에 반영됨.
	if gateway != "" {
		if dir, derr := gojiraDir(); derr == nil {
			if perr := persistGateway(dir, gateway); perr != nil {
				log.Printf("gateway 기록 실패(비치명적): %v", perr)
			} else {
				fmt.Printf("✓ gateway 기록: %s (backend URL/webOrigins 자동 파생)\n", gateway)
			}
		}
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

	// 의존성(jq, python) 확보 — 스킬이 curl/jq/python3 로 동작하므로 필수.
	// 다운로드 실패 등으로 확보 못 하면 설치를 여기서 중단한다(엄격).
	binDir, err := gojiraBinDir()
	if err != nil {
		return fmt.Errorf("bin 디렉터리 경로 확인 실패: %w", err)
	}
	if err := EnsureDependencies(binDir); err != nil {
		return fmt.Errorf("필수 의존성 확보 실패 — 설치를 중단합니다: %w", err)
	}
	fmt.Printf("✓ 의존성 디렉터리: %s\n", binDir)

	// 자동시작 등록.
	if err := registerAutostart(target); err != nil {
		log.Printf("자동시작 등록 실패(비치명적): %v", err)
	} else {
		fmt.Printf("✓ 자동시작 등록 완료\n")
	}

	// 데몬 기동(detached). (기존 데몬은 위에서 이미 종료 요청했다 — 설정은 기동 시
	// 1회만 읽으므로, 내리지 않으면 방금 기록한 gateway/backend 값이 절대 반영되지
	// 않는다. 사람이 "재시작하라" 안내를 놓치는 게 실제로 반복 발생한 실패 원인이었다.)
	if err := startDetached(target); err != nil {
		log.Printf("데몬 자동 기동 실패(비치명적 — 수동 실행 가능): %v", err)
	} else {
		fmt.Printf("✓ 데몬 기동: http://127.0.0.1:%d\n", cfg.Port)
	}

	fmt.Printf("\n설치 완료. 폐쇄망에서는 ~/.gojira/%s 의 git.user/git.token 을 채우고\ninstall 을 다시 실행하세요(기존 데몬을 자동으로 내리고 새 설정으로 재기동합니다).\n", configFileName)
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

// persistGateway — launcher.json 의 `gateway` 필드만 갱신해 저장한다. 다른 필드
// (git 크리덴셜 등)는 map round-trip 으로 보존. 파일이 없으면 placeholder 를 먼저 만든다.
func persistGateway(dir, gateway string) error {
	path := filepath.Join(dir, configFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if werr := writePlaceholderConfig(dir, path); werr != nil {
			return werr
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("%s 파싱 실패: %w", path, err)
	}
	if strings.TrimSpace(gateway) == m["gateway"] {
		return nil // 변경 없음
	}
	m["gateway"] = strings.TrimSpace(gateway)
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o600)
}

// peekExistingPortToken — LoadConfig 를 부르기 전, 기존 launcher.json(있으면)에서
// port/token 만 가볍게 읽는다. 파일 파싱에 실패하거나 없으면 기본 port + 빈 token 반환
// (그 경우 stopRunningDaemon 은 그냥 조용히 실패한다 — 최초 설치라 떠 있는 데몬이 없음).
func peekExistingPortToken(dir string) (int, string) {
	path := filepath.Join(dir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultPort, ""
	}
	var m struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return defaultPort, ""
	}
	if m.Port == 0 {
		m.Port = defaultPort
	}
	return m.Port, m.Token
}

// stopRunningDaemon — 같은 포트에 이미 떠 있는 구 데몬을 POST /shutdown 으로 정상
// 종료시킨다. 도달 불가(실행 중 아님)면 조용히 무시 — install 첫 실행 시엔 항상 이 경로.
// 토큰이 안 맞으면(설정 파일이 아예 새로 생성된 경우 등) 서버가 401 을 주는데, 그 경우도
// 그냥 무시하고 계속 진행한다(포트 충돌은 startDetached 쪽에서 자연히 드러남).
func stopRunningDaemon(port int, token string) {
	url := fmt.Sprintf("http://127.0.0.1:%d/shutdown", port)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return
	}
	if token != "" {
		req.Header.Set(tokenHeader, token)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return // 실행 중인 인스턴스 없음(또는 도달 불가) — 정상 케이스
	}
	resp.Body.Close()
	fmt.Println("✓ 기존 데몬 종료 요청")
	time.Sleep(500 * time.Millisecond) // 포트 해제 대기
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
