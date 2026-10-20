package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// repoLocks 레포별 mutex (동시 git 작업 직렬화) — Platform-API/github 의 패턴을 단순화.
var (
	repoLocks   = map[string]*sync.Mutex{}
	repoLocksMu sync.Mutex
)

func getRepoLock(key string) *sync.Mutex {
	repoLocksMu.Lock()
	defer repoLocksMu.Unlock()
	if repoLocks[key] == nil {
		repoLocks[key] = &sync.Mutex{}
	}
	return repoLocks[key]
}

// safeName — code/user 는 경로·URL·브랜치명에 그대로 들어가므로 엄격히 검증
// (path traversal / 명령 인젝션 차단). 영숫자 + . _ - 만 허용, 점 시작 금지.
var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validateName(kind, v string) error {
	if v == "" {
		return fmt.Errorf("%s 누락", kind)
	}
	if !safeName.MatchString(v) || strings.Contains(v, "..") {
		return fmt.Errorf("%s 형식이 올바르지 않습니다: %q", kind, v)
	}
	return nil
}

// gitCmd 는 git 을 실행한다. 자격 프롬프트는 비활성(GIT_TERMINAL_PROMPT=0).
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	hideConsole(cmd) // Windows GUI 데몬에서 git.exe 콘솔 창 번쩍임 방지 (비 Windows no-op)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// repoPath — 로컬 clone 경로 (repoRoot/<code>).
func (s *Server) repoPath(code string) string {
	return filepath.Join(s.cfg.RepoRoot, code)
}

// cloneURL — 자격이 있으면 http://<user>:<token>@<host>/... 형태로 임베드한다.
// 폐쇄망 Gitea (익명 차단) 에서 사용자 PC 의 GCM/netrc 설정 없이도 clone/fetch/push 가 자동 인증.
//
// 자격은 요청 사용자(user=knoxId) 본인의 Gitea PAT 를 우선 사용한다(gitcreds.go).
// 조회 실패 시 launcher.json 의 git.user/git.token 으로 폴백.
func (s *Server) cloneURL(repoName, user string) string {
	base := strings.TrimRight(s.cfg.Git.Base, "/")
	org := strings.Trim(s.cfg.Git.Org, "/")
	gitUser, gitToken := s.userGitCreds(user)
	if gitToken != "" && gitUser != "" {
		// 스킴 분리해서 user:token@ 삽입. user/token 의 @ : / 같은 특수문자 url-encode.
		if idx := strings.Index(base, "://"); idx > 0 {
			scheme := base[:idx+3]
			host := base[idx+3:]
			u := url.QueryEscape(gitUser)
			t := url.QueryEscape(gitToken)
			base = fmt.Sprintf("%s%s:%s@%s", scheme, u, t, host)
		}
	}
	return fmt.Sprintf("%s/%s/%s.git", base, org, repoName)
}

// projCode — 프로젝트 문서에서 code 추출 (jiraCodes.main.jiraCode 우선, 없으면 top-level code).
func projCode(m map[string]any) string {
	if jc, ok := m["jiraCodes"].(map[string]any); ok {
		if main, ok := jc["main"].(map[string]any); ok {
			if c, ok := main["jiraCode"].(string); ok {
				return c
			}
		}
	}
	if c, ok := m["code"].(string); ok {
		return c
	}
	return ""
}

func repoNameOf(m map[string]any) string {
	if docs, ok := m["documents"].(map[string]any); ok {
		if rn, ok := docs["repoName"].(string); ok {
			return rn
		}
	}
	return ""
}

// resolveRepoName — 프로젝트 code 를 실제 Gitea repo 이름으로 해석한다.
// code 와 repo 이름이 다를 수 있으므로(예: NBBALL → number-baseball) GoJIRA-API 의
// 프로젝트 documents.repoName 을 조회한다. 백엔드 미설정/조회 실패 시 code 그대로(하위호환).
func (s *Server) resolveRepoName(code, user string) string {
	base := strings.TrimRight(s.cfg.Backend.GojiraAPI, "/")
	if base == "" {
		return code
	}
	req, _ := http.NewRequest(http.MethodGet, base+"/Project", nil)
	req.Header.Set("XBody", fmt.Sprintf(`{"jiraCode":%q}`, code))
	req.Header.Set("X-User-Id", user)
	if t := s.ssoToken(); t != "" { // SSO 모드: 사용자 세션토큰 Bearer
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		log.Printf("resolveRepoName warn (%s): %v — code 사용", code, err)
		return code
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("resolveRepoName warn (%s): HTTP %d — code 사용", code, resp.StatusCode)
		return code
	}
	body, _ := io.ReadAll(resp.Body)

	// 응답은 배열(필터 안 될 수 있음) 또는 단일 객체. code 일치 항목의 repoName 사용.
	var arr []map[string]any
	if json.Unmarshal(body, &arr) == nil && len(arr) > 0 {
		for _, m := range arr {
			if projCode(m) == code {
				if rn := repoNameOf(m); rn != "" {
					return rn
				}
			}
		}
		// 단일 결과면 code 검증 없이도 사용
		if len(arr) == 1 {
			if rn := repoNameOf(arr[0]); rn != "" {
				return rn
			}
		}
	}
	var obj map[string]any
	if json.Unmarshal(body, &obj) == nil {
		if rn := repoNameOf(obj); rn != "" {
			return rn
		}
	}
	log.Printf("resolveRepoName: %s 의 repoName 미발견 — code 사용", code)
	return code
}

// EnsureRepo 는 로컬 clone 을 보장하고 docs/<user> 브랜치를 체크아웃한다.
//   - 서버측 bare+worktree 모델과 달리 사용자 PC 의 일반 작업본 1개 (설계 §4: 브랜치 내부 충돌 0).
//   - 이미 있으면 fetch 후 브랜치 정렬만 수행.
//   - withFetch=false: 네트워크 호출 생략 (읽기 전용 /files 등에서 사용 — 느린 Gitea 환경에서 hang 방지).
func (s *Server) EnsureRepo(code, user string) (string, error) {
	return s.ensureRepoImpl(code, user, true)
}

// EnsureRepoLocal — EnsureRepo 의 no-network 변형. /files 처럼 단순 조회용 핸들러에서 사용.
// 신규 clone 은 어쨌든 1회 네트워크가 필요하므로 첫 호출은 동일 비용. 이후 호출은 fetch 생략.
func (s *Server) EnsureRepoLocal(code, user string) (string, error) {
	return s.ensureRepoImpl(code, user, false)
}

// resolveRepoLocal — 순수 읽기(/file 단건 조회) 전용 초경량 경로 해석.
//
// clone 이 이미 있으면 origin 정렬·identity·branch checkout·asset 링크·exclude 갱신을
// 일절 하지 않고, 전역 repo 락도 잡지 않고 repoPath 만 즉시 반환한다.
// 이 프로비저닝은 사용자가 목록을 띄울 때 호출되는 /files(EnsureRepoLocal)가 이미 끝내 두므로
// 파일을 열 때마다 반복할 필요가 없다. (클릭당 git 서브프로세스 수개 + 락 대기 제거)
//
// clone 이 아직 없는 콜드 케이스(목록 없이 직접 호출)만 EnsureRepoLocal 로 폴백 — 1회 clone.
func (s *Server) resolveRepoLocal(code, user string) (string, error) {
	if err := validateName("code", code); err != nil {
		return "", err
	}
	if err := validateName("user", user); err != nil {
		return "", err
	}
	path := s.repoPath(code)
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return path, nil // 이미 프로비저닝된 clone — 즉시 반환 (락/git 호출 없음).
	}
	// clone 부재 — 1회 full 프로비저닝 필요.
	return s.EnsureRepoLocal(code, user)
}

func (s *Server) ensureRepoImpl(code, user string, withFetch bool) (string, error) {
	if err := validateName("code", code); err != nil {
		return "", err
	}
	if err := validateName("user", user); err != nil {
		return "", err
	}
	if s.cfg.Git.Base == "" {
		return "", fmt.Errorf("git.base 미설정 — launcher.json 에 clone 대상 Gitea 주소를 지정하세요")
	}

	path := s.repoPath(code)

	// 정렬 게이트 — no-fetch 조회 경로는 프로세스 생애 동안 (code,user) 당 1회만 정렬한다.
	// clone 존재 확인(stat)은 값싸므로 유지 — 사용자가 clone 을 지웠으면 게이트 무효화 후 재프로비저닝.
	if !withFetch {
		if u, ok := s.alignedRepos.Load(code); ok && u.(string) == user {
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				return path, nil // 락/git 서브프로세스 0 으로 즉시 반환.
			}
			s.alignedRepos.Delete(code)
		}
	}

	mu := getRepoLock(code)
	mu.Lock()
	defer mu.Unlock()

	branch := "docs/" + user

	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(s.cfg.RepoRoot, 0o755); err != nil {
			return "", fmt.Errorf("repoRoot 생성 실패: %w", err)
		}
		// code ≠ Gitea repo 이름일 수 있으므로 repoName 으로 clone (로컬 디렉터리명은 code).
		repoName := s.resolveRepoName(code, user)
		if _, err := gitCmd(s.cfg.RepoRoot, "clone", s.cloneURL(repoName, user), code); err != nil {
			return "", err
		}
	} else {
		// 기존 clone — git.user/token 변경 또는 빈 origin(이전 익명 clone) 케이스 대비
		// origin URL 을 현재 cloneURL 로 정렬. 변경 없으면 no-op.
		s.ensureOriginURL(path, code, user)
		if withFetch {
			gitCmd(path, "fetch", "origin")
		}
	}

	if err := s.checkoutUserBranch(path, branch, withFetch); err != nil {
		return "", err
	}
	s.ensureAgentAssets(path)
	// /sync 의 git add -A 가 provision 링크(.claude/.opencode/opencode.json)를 staging 하지 않도록 local-only ignore.
	for _, entry := range agentAssetEntries {
		s.ensureExcluded(path, entry)
	}
	s.ensureGitIdentity(path, user)
	s.alignedRepos.Store(code, user) // 정렬 완료 — 이후 no-fetch 조회는 fast-path.
	return path, nil
}

// ensureExcluded — .git/info/exclude (local-only gitignore) 에 entry 가 없으면 추가.
// .gitignore 와 달리 트래킹되지 않아 사용자/프로젝트 마다 다른 로컬 산물(예: 스킬 심볼릭링크)에 적합.
func (s *Server) ensureExcluded(path, entry string) {
	excludePath := filepath.Join(path, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		log.Printf("ensureExcluded read 실패 (%s): %v", excludePath, err)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return
		}
	}
	_ = os.MkdirAll(filepath.Dir(excludePath), 0o755)
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	if err := os.WriteFile(excludePath, []byte(content), 0o644); err != nil {
		log.Printf("ensureExcluded write 실패 (%s): %v", excludePath, err)
	}
}

// ensureOriginURL — 기존 clone 의 origin URL 을 현재 launcher.json 설정과 정렬.
// 초기 clone 시점에 git.user/token 이 없어 익명 URL 로 만들어진 경우, 또는 PAT 가 갱신된
// 경우 자동으로 origin URL 을 교체한다. 같으면 no-op.
//
// 보안: 로그에 token 이 노출되지 않도록 cloneURL 결과는 출력하지 않는다.
func (s *Server) ensureOriginURL(path, code, user string) {
	// 자격이 전혀 없으면(사용자 PAT 조회 실패 + launcher.json 미설정) 익명 URL 을
	// 덮어쓸 이유가 없으므로 그대로 둔다.
	if gitUser, gitToken := s.userGitCreds(user); gitToken == "" || gitUser == "" {
		return
	}
	current, _ := gitCmd(path, "remote", "get-url", "origin")
	current = strings.TrimSpace(current)
	// repo 이름은 기존 origin URL 의 마지막 path segment 에서 파생 — 정렬 목적은 자격증명
	// (user/token) 갱신이지 repo 변경이 아니므로 백엔드 조회가 필요 없다. 이전에는 여기서
	// 매 호출 resolveRepoName(GoJIRA-API HTTP, timeout 10s)이 나가 사내망 /files 지연의
	// 주범이었다. URL 파싱 불가(빈 origin 등) 케이스만 HTTP 폴백.
	repoName := repoNameFromURL(current)
	if repoName == "" {
		repoName = s.resolveRepoName(code, user)
	}
	desired := s.cloneURL(repoName, user)
	if current == desired {
		return
	}
	if _, err := gitCmd(path, "remote", "set-url", "origin", desired); err != nil {
		log.Printf("ensureOriginURL set-url 실패 (%s): %v", path, err)
	} else {
		log.Printf("ensureOriginURL origin URL 갱신 (%s)", code)
	}
}

// repoNameFromURL — origin URL 의 마지막 path segment 에서 repo 이름을 뽑는다 (".git" 제거).
// 예) http://user:tok@host:3300/gojira/number-baseball.git → number-baseball
// 파싱 불가(빈 문자열, path 없음)면 "" — 호출 측이 resolveRepoName 으로 폴백.
func repoNameFromURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	u, err := url.Parse(remote)
	if err != nil {
		return ""
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	name := strings.TrimSuffix(segs[len(segs)-1], ".git")
	return name
}

// ensureGitIdentity — clone 의 user.name/user.email 이 비어있으면 주입한다 (commit author 식별).
// 우선순위: launcher.json git.authorName/Email → 요청 user(knoxId) 기반 fallback.
// 인증과는 무관 (인증은 clone URL embedded PAT). 이미 설정돼 있으면 손대지 않는다.
func (s *Server) ensureGitIdentity(path, user string) {
	name := s.cfg.Git.AuthorName
	if name == "" {
		name = user
	}
	email := s.cfg.Git.AuthorEmail
	if email == "" {
		email = user + "@gojira.local"
	}
	if cur, _ := gitCmd(path, "config", "--get", "user.name"); cur == "" {
		if _, err := gitCmd(path, "config", "user.name", name); err != nil {
			log.Printf("ensureGitIdentity user.name 실패 (%s): %v", path, err)
		}
	}
	if cur, _ := gitCmd(path, "config", "--get", "user.email"); cur == "" {
		if _, err := gitCmd(path, "config", "user.email", email); err != nil {
			log.Printf("ensureGitIdentity user.email 실패 (%s): %v", path, err)
		}
	}
}

// ensureAgentAssets — skillsSourceDir 의 에이전트 자산(.claude/.opencode/opencode.json)을
// 각 clone(target) 으로 연결한다 (이미 있으면 노옵). 실패는 비치명적 — 로그만.
// 목적: 런처가 spawn 하는 opencode/claude 가 cwd=clone 에서 스킬+설정을 그대로 인식하도록.
//
// OS별 전략 (사용자가 둔 동일 이름 파일이 있으면 어느 경우든 손대지 않음):
//   - Linux/macOS: os.Symlink (권한 불요)
//   - Windows(디렉터리): 1) symlink (Dev Mode 필요) 2) mklink /J junction(권한 불요/동일 볼륨) 3) 트리 복사
//   - Windows(단일 파일 opencode.json): symlink → 복사 (junction 은 디렉터리 전용)
func (s *Server) ensureAgentAssets(target string) {
	src := s.cfg.SkillsSourceDir
	if src == "" {
		return
	}
	for _, entry := range agentAssetEntries {
		srcPath := filepath.Join(src, entry)
		fi, err := os.Stat(srcPath)
		if err != nil {
			continue // 원본에 없으면 스킵 (예: 개발 빌드).
		}
		dst := filepath.Join(target, entry)
		if _, err := os.Lstat(dst); err == nil {
			continue // 이미 있음 — 사용자 파일이든 이전 링크든 건드리지 않는다.
		}
		linkOrCopy(srcPath, dst, fi.IsDir())
	}
	// Windows: opencode 의 Bash 툴이 기본 PowerShell 대신 Git Bash 를 쓰도록 clone 의
	// opencode.json 에 shell 설정을 주입한다. SHELL env 는 tool call 에 안 먹는 경우가
	// 있어 문서화된 `shell` 설정이 신뢰성이 높다(한글 UTF-8 깨짐의 근본 차단).
	ensureWindowsShellConfig(target, src)
}

// ensureWindowsShellConfig — Windows 에서 clone 의 opencode.json 에 Git Bash 셸 설정을
// 병합한다. 심볼릭 링크면 공유 원본 훼손 방지를 위해 실파일 복사로 교체한 뒤 수정한다.
// 사용자가 직접 지정한 shell 설정이 있으면 존중하고 건드리지 않는다. Git Bash 미발견 시 무시.
func ensureWindowsShellConfig(target, src string) {
	if runtime.GOOS != "windows" {
		return
	}
	bashPath := findGitBash()
	if bashPath == "" {
		log.Printf("ensureWindowsShellConfig: Git Bash 미발견 — opencode shell 설정 스킵 (스킬 bash 깨짐 위험)")
		return
	}
	dst := filepath.Join(target, "opencode.json")
	// 심볼릭 링크면 실파일로 교체(원본 opencode.json 오염 방지).
	if fi, err := os.Lstat(dst); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(dst)
		_ = copyFile(filepath.Join(src, "opencode.json"), dst, 0o644)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		return // clone 에 opencode.json 이 없으면 스킵(자산 추출 실패 등).
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		log.Printf("ensureWindowsShellConfig: opencode.json 파싱 실패 — 스킵: %v", err)
		return
	}
	// opencode 의 `shell` 키는 단순 문자열(경로 또는 짧은 이름)이다. 객체로 주면
	// config 스키마 검증이 실패해 opencode 가 기동조차 못 한다(config.get 등 실패).
	// - 이미 유효한 문자열이면 사용자 지정으로 보고 존중.
	// - 객체/비문자열(과거 잘못된 주입 포함)이면 문자열로 정규화해 self-heal.
	if sv, ok := m["shell"]; ok {
		if s, isStr := sv.(string); isStr && s != "" {
			return
		}
		log.Printf("ensureWindowsShellConfig: 잘못된 shell 설정(비문자열) 감지 → Git Bash 문자열로 정규화")
	}
	m["shell"] = bashPath
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		log.Printf("ensureWindowsShellConfig write 실패 (%s): %v", dst, err)
		return
	}
	log.Printf("ensureWindowsShellConfig: opencode.json 에 Git Bash 셸 주입 → %s", bashPath)
}

// linkOrCopy — src 를 dst 로 링크(불가 시 복사). isDir 에 따라 junction/트리복사 vs 단일파일 복사.
func linkOrCopy(src, dst string, isDir bool) {
	if runtime.GOOS != "windows" {
		if err := os.Symlink(src, dst); err != nil {
			log.Printf("ensureAgentAssets symlink 실패 (%s → %s): %v", src, dst, err)
		}
		return
	}
	// Windows 단계적 폴백.
	if err := os.Symlink(src, dst); err == nil {
		return
	}
	if isDir {
		if err := exec.Command("cmd", "/c", "mklink", "/J", dst, src).Run(); err == nil {
			return
		}
		if err := copyTree(src, dst); err != nil {
			log.Printf("ensureAgentAssets Windows fallback(copy dir) 실패 (%s → %s): %v — Dev Mode 권장", src, dst, err)
		}
		return
	}
	// 단일 파일 — junction 불가, 복사.
	if err := copyFile(src, dst, 0o644); err != nil {
		log.Printf("ensureAgentAssets Windows fallback(copy file) 실패 (%s → %s): %v", src, dst, err)
	}
}

// copyTree — src 트리를 dst 로 재귀 복사 (Windows symlink/junction 폴백 전용).
// 심볼릭 링크 자체는 따라가지 않고 일반 파일로 복사 — .claude/skills 내부 일반 파일/디렉토리만 다룬다.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// checkoutUserBranch — origin/<branch> 가 있으면 추적 체크아웃, 없으면 origin/main 기반 신규 생성.
// withFetch=true 일 때만 추가로: docs/{user} 가 origin/main 의 후행이고 미커밋 변경이 없으면 silently FF merge.
// (withFetch=false 는 read-only 호출 — 네트워크 hang 방지를 위해 pull/merge 도 생략.)
//
// 빈 clone (unborn HEAD) 자동 복구: origin/main 도 없으면 read-only 라도 1회 fetch 시도.
// (옛날 stale 빈 clone 이 남아있다가 후에 main 에 content 가 올라온 케이스 자동 회복.)
func (s *Server) checkoutUserBranch(path, branch string, withFetch bool) error {
	// 이미 해당 브랜치면 끝.
	if cur, err := gitCmd(path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && cur == branch {
		if withFetch {
			gitCmd(path, "pull", "--ff-only")
			s.tryFFFromMain(path)
		}
		return nil
	}
	// 원격에 존재하면 추적 체크아웃.
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/"+branch); err == nil {
		if _, err := gitCmd(path, "checkout", branch); err != nil {
			return err
		}
		if withFetch {
			gitCmd(path, "pull", "--ff-only")
			s.tryFFFromMain(path)
		}
		return nil
	}
	// 없으면 origin/main 기반 새 브랜치.
	base := "origin/main"
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/main"); err != nil {
		// origin/main 도 없음 — 빈 clone 의증. read-only 라도 1회 fetch 시도.
		log.Printf("checkoutUserBranch: origin/main 없음 — fetch 시도 (%s)", path)
		gitCmd(path, "fetch", "origin")
		if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/main"); err != nil {
			// fetch 후에도 main 없음: 빈 Gitea repo 이거나 (드물게), 인증 실패로 fetch 가
			// 아무 ref 를 못 받은 경우. 폐쇄망에서는 후자가 압도적으로 흔하므로 그쪽을 우선 안내.
			return fmt.Errorf("로컬 clone 에 origin/main 이 없음 — fetch 가 실패했거나 권한 부족. "+
				"launcher.json 의 git.user/git.token 을 확인하고, 이 폴더(%s) 를 지운 뒤 launcher 재시도. "+
				"(Gitea 측 main 브랜치는 정상일 수 있음 — UI 에서 직접 확인 권장.)", path)
		}
	}
	args := []string{"checkout", "-b", branch, base}
	_, err := gitCmd(path, args...)
	return err
}

// tryFFFromMain — docs/{user} 가 origin/main 의 후행이면 silently FF merge.
// 미커밋 dirty 거나, diverged, 또는 ahead 면 그대로 둔다 (사용자가 Pull 버튼으로 명시적 해결).
func (s *Server) tryFFFromMain(path string) {
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/main"); err != nil {
		return // main 없음
	}
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		return // dirty
	}
	if _, err := gitCmd(path, "merge", "--ff-only", "origin/main"); err != nil {
		// FF 불가 (diverged or ahead) — 정상 케이스. 로그만 남김.
		log.Printf("tryFFFromMain skipped (FF 불가) at %s: %v", path, err)
	}
}

// PushResult — /push 응답용.
type PushResult struct {
	Status     string `json:"status"` // "ok"
	Branch     string `json:"branch"`
	Pushed     bool   `json:"pushed"`  // 실제로 push 한 커밋이 있었는지
	Commits    int    `json:"commits"` // push 전 upstream 대비 ahead 수
	Dirty      bool   `json:"dirty"`   // 미커밋 변경 잔존 여부(경고)
	LastCommit string `json:"lastCommit,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Push — 로컬 clone 의 docs/<user> 브랜치를 origin 으로 push 한다.
//
// 로컬 커밋 자체는 사용자/CLI 의 몫(설계 §1: "저장 → 로컬 git commit → push").
// 여기서는 이미 커밋된 것을 올릴 뿐이며, 미커밋(dirty) 변경은 경고만 한다.
// 인증은 git 자격증명 헬퍼/clone URL 에 위임(PoC — §6 잔여).
func (s *Server) Push(code, user string) (*PushResult, error) {
	path, err := s.EnsureRepo(code, user)
	if err != nil {
		return nil, err
	}
	mu := getRepoLock(code)
	mu.Lock()
	defer mu.Unlock()

	branch := "docs/" + user
	res := &PushResult{Status: "ok", Branch: branch}
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		res.Dirty = true
	}

	// push 전 원격 변경 반영(rebase). 이슈(data/issues)·문서(business/architecture)는 경로가
	// 분리돼 같은 파일을 건드리지 않으므로 rebase 는 항상 무충돌(설계 §경로 분리).
	// dirty 면 rebase 가 실패하므로 건너뛰고 push 단계에서 경고만 한다(커밋은 사용자 몫).
	if !res.Dirty {
		if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/"+branch); err == nil {
			if _, err := gitCmd(path, "pull", "--rebase", "origin", branch); err != nil {
				return nil, fmt.Errorf("pull --rebase 실패(수동 해결 필요): %w", err)
			}
		}
	}

	if c, err := gitCmd(path, "log", "-1", "--format=%h %s"); err == nil {
		res.LastCommit = c
	}

	// upstream 대비 ahead 수 — 새 커밋이 없으면 push 생략.
	if counts, err := gitCmd(path, "rev-list", "--count", "@{upstream}..HEAD"); err == nil {
		res.Commits, _ = strconv.Atoi(strings.TrimSpace(counts))
	} else {
		// upstream 미설정(원격에 브랜치 없음) → 새 브랜치 push 필요. 커밋 수는 미상.
		res.Commits = -1
	}

	if res.Commits == 0 {
		res.Pushed = false
		res.Message = "push 할 새 커밋이 없습니다"
		if res.Dirty {
			res.Message += " (미커밋 변경 있음 — 먼저 commit 하세요)"
		}
		return res, nil
	}

	// 신규 브랜치면 upstream 설정, 아니면 일반 push.
	if _, err := gitCmd(path, "push", "-u", "origin", branch); err != nil {
		return nil, err
	}
	res.Pushed = true
	if res.Dirty {
		res.Message = "push 완료 (단, 미커밋 변경이 남아있습니다)"
	}
	return res, nil
}

// PullResult — /pull 응답용.
type PullResult struct {
	Status     string `json:"status"`              // "ok" | "noop"
	Branch     string `json:"branch"`              // 사용자 브랜치 (docs/<user>)
	Strategy   string `json:"strategy"`            // "ff" | "rebase"
	Merged     int    `json:"merged"`              // FF 로 끌어온 main 커밋 수
	Ahead      int    `json:"ahead"`               // pull 후 docs/<user> 가 main 보다 ahead 인 수
	LastCommit string `json:"lastCommit,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Pull — 로컬 docs/<user> 를 origin/main 으로 완전히 덮어쓴다 (destructive).
//   - git fetch origin → git reset --hard origin/main
//   - 사용자 커밋·미커밋 변경 모두 폐기. 사용자가 명시적으로 "main 으로 갱신" 의도일 때만 호출.
//   - 강한 경고는 프론트엔드에서 confirm 으로 처리 (UI 책임).
//   - 안전망: 직전 HEAD sha 를 응답에 포함해 사용자가 git reset --hard <prevSha> 로 복구 가능.
func (s *Server) Pull(code, user string) (*PullResult, error) {
	path, err := s.EnsureRepo(code, user)
	if err != nil {
		return nil, err
	}
	mu := getRepoLock(code)
	mu.Lock()
	defer mu.Unlock()

	branch := "docs/" + user
	res := &PullResult{Status: "ok", Branch: branch, Strategy: "reset-hard"}

	// 복구용 직전 HEAD 기록 (사용자가 실수로 눌렀을 때 git reset --hard <prevSha> 로 복원).
	prevHead, _ := gitCmd(path, "rev-parse", "HEAD")

	if _, err := gitCmd(path, "fetch", "origin"); err != nil {
		return nil, fmt.Errorf("fetch 실패: %w", err)
	}
	if _, err := gitCmd(path, "rev-parse", "--verify", "refs/remotes/origin/main"); err != nil {
		return nil, fmt.Errorf("origin/main 이 없습니다 (clone 직후 상태?) — Gitea 의 main 브랜치를 먼저 확인하세요")
	}

	// 이미 main 과 동일한 HEAD 면 noop.
	mainHead, _ := gitCmd(path, "rev-parse", "refs/remotes/origin/main")
	curHead, _ := gitCmd(path, "rev-parse", "HEAD")
	if mainHead != "" && mainHead == curHead {
		// working tree 가 깔끔하면 정말 노옵, dirty 면 그래도 reset 해서 깨끗하게.
		if porcelain, _ := gitCmd(path, "status", "--porcelain"); porcelain == "" {
			res.Status = "noop"
			res.Message = "이미 main 의 최신 내용과 동일합니다"
			res.LastCommit = curHead
			return res, nil
		}
	}

	// reset --hard origin/main — 로컬 커밋·미커밋 변경 모두 폐기.
	if _, err := gitCmd(path, "reset", "--hard", "origin/main"); err != nil {
		return nil, fmt.Errorf("reset --hard origin/main 실패: %w", err)
	}
	// untracked 파일은 reset --hard 가 안 지우므로 명시적으로 clean.
	gitCmd(path, "clean", "-fd")

	res.Merged = -1 // 적용 커밋 수는 의미 없음 (overwrite). frontend 는 strategy=reset-hard 로 분기.
	if c, err := gitCmd(path, "log", "-1", "--format=%h %s"); err == nil {
		res.LastCommit = c
	}
	if prevHead != "" {
		res.Message = fmt.Sprintf("로컬 docs/%s 를 main 으로 완전 덮어씌움 (직전 HEAD: %s — 실수면 cd %s && git reset --hard %s 로 복구)", user, prevHead[:min(8, len(prevHead))], path, prevHead[:min(8, len(prevHead))])
	} else {
		res.Message = fmt.Sprintf("로컬 docs/%s 를 main 으로 완전 덮어씌움", user)
	}
	return res, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RepoStatus — /status 응답용 (브랜치/ahead·behind/dirty/마지막 커밋).
type RepoStatus struct {
	State      string `json:"state"` // "clean" | "dirty" | "missing"
	Branch     string `json:"branch,omitempty"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Dirty      bool   `json:"dirty"`
	LastCommit string `json:"lastCommit,omitempty"`
}

func (s *Server) Status(code string) (*RepoStatus, error) {
	if err := validateName("code", code); err != nil {
		return nil, err
	}
	path := s.repoPath(code)
	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		return &RepoStatus{State: "missing"}, nil
	}

	st := &RepoStatus{State: "clean"}
	if br, err := gitCmd(path, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		st.Branch = br
	}
	if porcelain, err := gitCmd(path, "status", "--porcelain"); err == nil && porcelain != "" {
		st.Dirty = true
		st.State = "dirty"
	}
	// ahead/behind vs upstream (있을 때만).
	if counts, err := gitCmd(path, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
		if f := strings.Fields(counts); len(f) == 2 {
			st.Behind, _ = strconv.Atoi(f[0])
			st.Ahead, _ = strconv.Atoi(f[1])
		}
	}
	if c, err := gitCmd(path, "log", "-1", "--format=%h %s"); err == nil {
		st.LastCommit = c
	}
	return st, nil
}
